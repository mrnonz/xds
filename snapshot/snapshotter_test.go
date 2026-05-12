package snapshot

import (
	"context"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// waitForSnapshot polls the services cache until a non-empty snapshot is
// available, or the deadline expires. Returns the snapshot or fails the test.
func waitForSnapshot(t *testing.T, c cache.SnapshotCache, expectedNames ...string) cache.ResourceSnapshot {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := c.GetSnapshot("")
		if err == nil && snap != nil {
			items := snap.GetResources(resource.ListenerType)
			missing := false
			for _, name := range expectedNames {
				if _, ok := items[name]; !ok {
					missing = true
					break
				}
			}
			if !missing {
				return snap
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("snapshot did not contain all expected listeners %v within deadline", expectedNames)
	return nil
}

func TestSnapshotter_DualEmission_StoresBothLocalAndXDSTPResources(t *testing.T) {
	kubeSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "grpc", Port: 50051}},
		},
	}
	kubeEp := &corev1.Endpoints{ //nolint:staticcheck // legacy Kube API
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Subsets: []corev1.EndpointSubset{{ //nolint:staticcheck
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}, //nolint:staticcheck
			Ports: []corev1.EndpointPort{{ //nolint:staticcheck
				Name: "grpc",
				Port: 50051,
			}},
		}},
	}

	client := fake.NewClientset(kubeSvc, kubeEp)
	s := New(client, "alpha")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = s.Start(ctx) }()

	const localListener = "foo.default:50051"
	const xdstpListener = "xdstp://alpha/envoy.config.listener.v3.Listener/foo.default:50051"

	mux := s.MuxCache()
	require.NotNil(t, mux)

	// Pull the underlying services cache so we can poll for the snapshot.
	servicesCache := s.servicesCache
	snap := waitForSnapshot(t, servicesCache, localListener, xdstpListener)

	listeners := snap.GetResources(resource.ListenerType)
	assert.Contains(t, listeners, localListener)
	assert.Contains(t, listeners, xdstpListener)

	clusters := snap.GetResources(resource.ClusterType)
	assert.Contains(t, clusters, "foo.default:grpc")
	assert.Contains(t, clusters, "xdstp://alpha/envoy.config.cluster.v3.Cluster/foo.default:grpc")

	// Both clusters MUST have ServiceName set so gRPC's xDS client accepts
	// new-style names. Verify the value matches the matching CLA name.
	localCluster := clusters["foo.default:grpc"].(*clusterv3.Cluster)
	xdstpCluster := clusters["xdstp://alpha/envoy.config.cluster.v3.Cluster/foo.default:grpc"].(*clusterv3.Cluster)
	assert.Equal(t, "foo.default:grpc", localCluster.EdsClusterConfig.ServiceName)
	assert.Equal(t, "xdstp://alpha/envoy.config.endpoint.v3.ClusterLoadAssignment/foo.default:grpc", xdstpCluster.EdsClusterConfig.ServiceName)
}

func TestSnapshotter_LocalOnlyMode_StoresOnlyLocalResources(t *testing.T) {
	kubeSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "grpc", Port: 50051}},
		},
	}

	client := fake.NewClientset(kubeSvc)
	s := New(client, "")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = s.Start(ctx) }()

	const localListener = "foo.default:50051"
	snap := waitForSnapshot(t, s.servicesCache, localListener)

	listeners := snap.GetResources(resource.ListenerType)
	assert.Contains(t, listeners, localListener)
	for name := range listeners {
		assert.NotContains(t, name, "xdstp://", "local-only mode must not emit xdstp resources")
	}
}
