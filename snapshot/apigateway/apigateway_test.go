package apigateway_test

import (
	"testing"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wongnai/xds/snapshot/apigateway"
	"github.com/wongnai/xds/snapshot/namer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeApigwService(name, ns, gateways, services string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				apigateway.NameAnnotation:    gateways,
				apigateway.ServiceAnnotation: services,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "grpc", Port: 50000}},
		},
	}
}

func splitByType(resources []types.Resource) (map[string]*listenerv3.Listener, map[string]*routev3.RouteConfiguration) {
	listeners := map[string]*listenerv3.Listener{}
	routes := map[string]*routev3.RouteConfiguration{}
	for _, r := range resources {
		switch v := r.(type) {
		case *listenerv3.Listener:
			listeners[v.Name] = v
		case *routev3.RouteConfiguration:
			routes[v.Name] = v
		}
	}
	return listeners, routes
}

func TestFromKubeServices_LocalNamer(t *testing.T) {
	svc := makeApigwService("backend", "default", "apigw1", "pkg.Service")
	resources, stats := apigateway.FromKubeServices([]*corev1.Service{svc}, namer.LocalNamer())

	listeners, routes := splitByType(resources)

	require.Contains(t, listeners, "apigw1")
	require.Contains(t, routes, "apigw1")

	assert.Equal(t, "backend.default:grpc", routes["apigw1"].VirtualHosts[0].Routes[0].GetRoute().GetCluster())
	assert.Equal(t, 1, stats["apigw1"])
}

func TestFromKubeServices_XDSTPNamer(t *testing.T) {
	svc := makeApigwService("backend", "default", "apigw1", "pkg.Service")
	resources, stats := apigateway.FromKubeServices([]*corev1.Service{svc}, namer.XDSTPNamer("alpha"))

	listeners, routes := splitByType(resources)

	const listenerName = "xdstp://alpha/envoy.config.listener.v3.Listener/apigw1"
	const routeName = "xdstp://alpha/envoy.config.route.v3.RouteConfiguration/apigw1"
	const clusterName = "xdstp://alpha/envoy.config.cluster.v3.Cluster/backend.default:grpc"

	require.Contains(t, listeners, listenerName)
	require.Contains(t, routes, routeName)

	assert.Equal(t, clusterName, routes[routeName].VirtualHosts[0].Routes[0].GetRoute().GetCluster())

	// Stats keys are the original gateway name regardless of namer — keeps
	// metric labels stable across name spaces.
	assert.Equal(t, 1, stats["apigw1"])
	assert.NotContains(t, stats, listenerName)
}
