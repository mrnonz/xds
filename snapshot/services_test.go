package snapshot

import (
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	managerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wongnai/xds/snapshot/namer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeService(name, namespace string, portName string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: portName, Port: port}},
		},
	}
}

// indexResources groups the output of kubeServicesToResources by concrete type
// for easy assertion. There's exactly one Listener/RouteConfig/Cluster per
// service+port in the input, so the maps key on resource Name.
func indexResources(t *testing.T, resources []types.Resource) (map[string]*listenerv3.Listener, map[string]*routev3.RouteConfiguration, map[string]*clusterv3.Cluster) {
	t.Helper()
	listeners := map[string]*listenerv3.Listener{}
	routes := map[string]*routev3.RouteConfiguration{}
	clusters := map[string]*clusterv3.Cluster{}
	for _, r := range resources {
		switch v := r.(type) {
		case *listenerv3.Listener:
			listeners[v.Name] = v
		case *routev3.RouteConfiguration:
			routes[v.Name] = v
		case *clusterv3.Cluster:
			clusters[v.Name] = v
		}
	}
	return listeners, routes, clusters
}

func extractInlineRouteConfig(t *testing.T, l *listenerv3.Listener) *routev3.RouteConfiguration {
	t.Helper()
	require.NotNil(t, l.ApiListener)
	mgr := &managerv3.HttpConnectionManager{}
	require.NoError(t, l.ApiListener.ApiListener.UnmarshalTo(mgr))
	rc, ok := mgr.RouteSpecifier.(*managerv3.HttpConnectionManager_RouteConfig)
	require.True(t, ok, "expected inline RouteConfig")
	return rc.RouteConfig
}

func TestKubeServicesToResources_LocalNamer(t *testing.T) {
	svc := makeService("foo", "default", "grpc", 50051)
	resources := kubeServicesToResources([]*corev1.Service{svc}, namer.LocalNamer())

	listeners, routes, clusters := indexResources(t, resources)

	require.Contains(t, listeners, "foo.default:50051")
	require.Contains(t, routes, "foo.default:50051")
	require.Contains(t, clusters, "foo.default:grpc")

	assert.Equal(t, "foo.default:grpc", routes["foo.default:50051"].VirtualHosts[0].Routes[0].GetRoute().GetCluster())

	// Local clusters get ServiceName == cluster name — same value gRPC would
	// default to, just made explicit so the wire format stays uniform with
	// xdstp emission.
	assert.Equal(t, "foo.default:grpc", clusters["foo.default:grpc"].EdsClusterConfig.ServiceName)

	inline := extractInlineRouteConfig(t, listeners["foo.default:50051"])
	assert.Equal(t, "foo.default:50051", inline.Name)
	assert.Equal(t, "foo.default:grpc", inline.VirtualHosts[0].Routes[0].GetRoute().GetCluster())
}

func TestKubeServicesToResources_XDSTPNamer(t *testing.T) {
	svc := makeService("foo", "default", "grpc", 50051)
	resources := kubeServicesToResources([]*corev1.Service{svc}, namer.XDSTPNamer("alpha"))

	listeners, routes, clusters := indexResources(t, resources)

	const listenerName = "xdstp://alpha/envoy.config.listener.v3.Listener/foo.default:50051"
	const routeName = "xdstp://alpha/envoy.config.route.v3.RouteConfiguration/foo.default:50051"
	const clusterName = "xdstp://alpha/envoy.config.cluster.v3.Cluster/foo.default:grpc"

	require.Contains(t, listeners, listenerName)
	require.Contains(t, routes, routeName)
	require.Contains(t, clusters, clusterName)

	// xdstp Clusters MUST set ServiceName explicitly — gRPC rejects new-style
	// cluster names without it. The value is the matching xdstp Endpoint name.
	const endpointName = "xdstp://alpha/envoy.config.endpoint.v3.ClusterLoadAssignment/foo.default:grpc"
	assert.Equal(t, endpointName, clusters[clusterName].EdsClusterConfig.ServiceName)

	// References inside the standalone RouteConfiguration must use xdstp Cluster name.
	assert.Equal(t, clusterName, routes[routeName].VirtualHosts[0].Routes[0].GetRoute().GetCluster())

	// References inside the listener's inline HCM RouteConfig must also stay xdstp.
	inline := extractInlineRouteConfig(t, listeners[listenerName])
	assert.Equal(t, routeName, inline.Name)
	assert.Equal(t, clusterName, inline.VirtualHosts[0].Routes[0].GetRoute().GetCluster())

	// VirtualHost.Domains stay as the original target forms — gRPC matches by
	// host header, not by xDS resource name.
	assert.ElementsMatch(t,
		[]string{"foo.default", "foo.default:grpc", "foo.default:50051", "foo"},
		routes[routeName].VirtualHosts[0].Domains,
	)
}

func TestBuildServiceResources_DualEmissionContainsBothNameSpaces(t *testing.T) {
	svc := makeService("foo", "default", "grpc", 50051)

	merged, _ := buildServiceResources(
		[]*corev1.Service{svc},
		[]namer.Namer{namer.LocalNamer(), namer.XDSTPNamer("alpha")},
	)

	listeners, routes, clusters := indexResources(t, merged)

	// Both local and xdstp variants must coexist.
	assert.Contains(t, listeners, "foo.default:50051")
	assert.Contains(t, listeners, "xdstp://alpha/envoy.config.listener.v3.Listener/foo.default:50051")
	assert.Contains(t, routes, "foo.default:50051")
	assert.Contains(t, routes, "xdstp://alpha/envoy.config.route.v3.RouteConfiguration/foo.default:50051")
	assert.Contains(t, clusters, "foo.default:grpc")
	assert.Contains(t, clusters, "xdstp://alpha/envoy.config.cluster.v3.Cluster/foo.default:grpc")
}
