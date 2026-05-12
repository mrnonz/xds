package snapshot

import (
	"testing"

	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wongnai/xds/snapshot/namer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newSnapshotterForTest() *Snapshotter {
	return &Snapshotter{
		endpointResourceCache: map[string]endpointCacheItem{},
	}
}

func makeEndpoints(name, namespace, ip, portName string, port int32) *corev1.Endpoints { //nolint:staticcheck // legacy Kube API on purpose
	return &corev1.Endpoints{ //nolint:staticcheck
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, ResourceVersion: "1"},
		Subsets: []corev1.EndpointSubset{{ //nolint:staticcheck
			Addresses: []corev1.EndpointAddress{{IP: ip}}, //nolint:staticcheck
			Ports: []corev1.EndpointPort{{ //nolint:staticcheck
				Name: portName,
				Port: port,
			}},
		}},
	}
}

func indexCLAs(t *testing.T, resources []types.Resource) map[string]*endpointv3.ClusterLoadAssignment {
	t.Helper()
	out := map[string]*endpointv3.ClusterLoadAssignment{}
	for _, r := range resources {
		if cla, ok := r.(*endpointv3.ClusterLoadAssignment); ok {
			out[cla.ClusterName] = cla
		}
	}
	return out
}

func TestKubeEndpointToResources_LocalNamer(t *testing.T) {
	s := newSnapshotterForTest()
	ep := makeEndpoints("foo", "default", "10.0.0.1", "grpc", 50051)

	resources := s.kubeEndpointToResources(ep, namer.LocalNamer())

	clas := indexCLAs(t, resources)
	require.Contains(t, clas, "foo.default:grpc")

	cla := clas["foo.default:grpc"]
	require.Len(t, cla.Endpoints, 1)
	require.Len(t, cla.Endpoints[0].LbEndpoints, 1)
	addr := cla.Endpoints[0].LbEndpoints[0].GetEndpoint().Address.GetSocketAddress()
	assert.Equal(t, "10.0.0.1", addr.Address)
	assert.Equal(t, uint32(50051), addr.GetPortValue())
}

func TestKubeEndpointToResources_XDSTPNamer(t *testing.T) {
	s := newSnapshotterForTest()
	ep := makeEndpoints("foo", "default", "10.0.0.1", "grpc", 50051)

	resources := s.kubeEndpointToResources(ep, namer.XDSTPNamer("alpha"))

	clas := indexCLAs(t, resources)
	const xdstpClusterName = "xdstp://alpha/envoy.config.endpoint.v3.ClusterLoadAssignment/foo.default:grpc"
	require.Contains(t, clas, xdstpClusterName)

	cla := clas[xdstpClusterName]
	require.Len(t, cla.Endpoints, 1)
	require.Len(t, cla.Endpoints[0].LbEndpoints, 1)
	assert.Equal(t, "10.0.0.1", cla.Endpoints[0].LbEndpoints[0].GetEndpoint().Address.GetSocketAddress().Address)
}

func TestKubeEndpointToResources_DualEmission_DistinctEntries(t *testing.T) {
	s := newSnapshotterForTest()
	ep := makeEndpoints("foo", "default", "10.0.0.1", "grpc", 50051)

	local := s.kubeEndpointToResources(ep, namer.LocalNamer())
	xdstp := s.kubeEndpointToResources(ep, namer.XDSTPNamer("alpha"))

	// Both emissions must produce a CLA — the per-endpoint cache must not
	// collapse them into one despite sharing the same Kubernetes object.
	require.Len(t, local, 1)
	require.Len(t, xdstp, 1)

	localCLA := local[0].(*endpointv3.ClusterLoadAssignment)
	xdstpCLA := xdstp[0].(*endpointv3.ClusterLoadAssignment)

	assert.Equal(t, "foo.default:grpc", localCLA.ClusterName)
	assert.Equal(t, "xdstp://alpha/envoy.config.endpoint.v3.ClusterLoadAssignment/foo.default:grpc", xdstpCLA.ClusterName)

	// Endpoint payload (the actual addresses) must be identical between the
	// two emissions — they represent the same backend pods.
	assert.Equal(t,
		localCLA.Endpoints[0].LbEndpoints[0].GetEndpoint().Address.GetSocketAddress().Address,
		xdstpCLA.Endpoints[0].LbEndpoints[0].GetEndpoint().Address.GetSocketAddress().Address,
	)
}

func TestKubeEndpointToResources_CacheReuseOnSameNamerAndVersion(t *testing.T) {
	s := newSnapshotterForTest()
	ep := makeEndpoints("foo", "default", "10.0.0.1", "grpc", 50051)

	first := s.kubeEndpointToResources(ep, namer.LocalNamer())
	second := s.kubeEndpointToResources(ep, namer.LocalNamer())

	// Same object, same version, same namer → cache returns the same slice
	// header. (The cache exists to avoid re-marshalling identical state.)
	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	assert.Same(t, &first[0], &second[0], "cache should return same backing resources")
}
