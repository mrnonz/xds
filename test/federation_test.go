package test_test

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/wongnai/xds/internal/config"
	"github.com/wongnai/xds/internal/di"
	"github.com/wongnai/xds/test"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/xds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
)

const federationAuthority = "alpha"

// federationXdsServerBind reuses the same loopback-alias bind pattern as the
// non-federation integration test (xdsServerBind = 127.2.0.1). We bind to a
// distinct address so both suites can run concurrently in the same package.
const federationXdsServerBind = "127.2.0.2:0"

// federationFakeServiceIP is the loopback alias the per-suite fake services
// bind on; assigned in TestXdsFederationIntegration to avoid colliding with
// the non-federation suite (which uses 127.2.1.x).
const federationFakeServiceIP = "127.2.2.0"

// federatedFakeService bundles a backend service plus the metadata needed to
// register it with the kube tracker. We register all services up-front in
// TestXdsFederationIntegration so the snapshotter's initial List() picks them
// up — fake.Clientset's Watch events for late Tracker().Add() calls can take
// >15s to deliver, which exceeds gRPC's xDS resource fetch timeout.
type federatedFakeService struct {
	name      string
	namespace string
	port      int32
	svc       *test.FakeService
}

// XdsFederationIntegrationTestSuite mirrors XdsIntegrationTestSuite but starts
// the xDS server with --local-cluster=alpha and points clients at the server
// via an `authorities.alpha` block in their bootstrap. The xds:// URLs use the
// federated form `xds://alpha/<target>`, which exercises the xdstp emission
// path end-to-end.
type XdsFederationIntegrationTestSuite struct {
	suite.Suite
	di.TestServer

	listener     net.Listener
	fakeServices map[string]*federatedFakeService
}

func (s *XdsFederationIntegrationTestSuite) SetupSuite() {
	listener, err := net.Listen("tcp", federationXdsServerBind)
	s.Require().NoError(err)
	s.listener = listener

	go func() {
		err := s.TestServer.GrpcServer.Serve(listener)
		if err != nil {
			s.T().Error(err)
		}
	}()
}

func (s *XdsFederationIntegrationTestSuite) TearDownTest() {
	for _, fs := range s.fakeServices {
		fs.svc.AssertExpectations(s.T())
	}
	klog.Flush()
}

func (s *XdsFederationIntegrationTestSuite) TearDownSuite() {
	s.TestServer.GrpcServer.Stop()
	s.listener.Close()
	for _, fs := range s.fakeServices {
		fs.svc.Stop()
	}
}

// getFederatedClient returns a gRPC client configured with an A47 federation
// bootstrap: the xDS server is registered as both the default `xds_servers`
// entry AND as the named `authorities.<federationAuthority>` entry. This
// matches the production deployment topology where each cluster runs its own
// xDS server, addressable both as the local default and as a named authority.
func (s *XdsFederationIntegrationTestSuite) getFederatedClient(target string) grpc_health_v1.HealthClient {
	bootstrap := fmt.Sprintf(`{
		"xds_servers": [{
			"server_uri": "%s",
			"channel_creds": [{"type": "insecure"}],
			"server_features": ["xds_v3"]
		}],
		"authorities": {
			"%s": {
				"xds_servers": [{
					"server_uri": "%s",
					"channel_creds": [{"type": "insecure"}],
					"server_features": ["xds_v3"]
				}]
			}
		},
		"node": {
			"id": "test",
			"locality": {
				"zone" : "test"
			}
		}
	}`, s.listener.Addr().String(), federationAuthority, s.listener.Addr().String())

	xdsBuilder, err := xds.NewXDSResolverWithConfigForTesting([]byte(bootstrap))
	s.Require().NoError(err)
	client, err := grpc.NewClient(target, grpc.WithResolvers(xdsBuilder), grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)

	return grpc_health_v1.NewHealthClient(client)
}

// checkEventually retries the gRPC Check call until it succeeds or the
// deadline expires. The first call in a fresh xDS resolver eats a few
// seconds of LDS/CDS/EDS round trips before resources propagate; using
// Eventually keeps the test from flaking on warm-up time.
func (s *XdsFederationIntegrationTestSuite) checkEventually(client grpc_health_v1.HealthClient, service string) {
	require.Eventually(s.T(), func() bool {
		_, err := client.Check(s.T().Context(), &grpc_health_v1.HealthCheckRequest{Service: service})
		return err == nil
	}, 20*time.Second, 200*time.Millisecond, "Check did not succeed within deadline")
}

// TestXdstpAuthorityResolves verifies the federated URL form
// `xds://alpha/...` resolves through the xdstp resource emission path.
func (s *XdsFederationIntegrationTestSuite) TestXdstpAuthorityResolves() {
	fs := s.fakeServices["fed-app"]
	fs.svc.On("Check", mock.Anything, "fed-test").Return(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil)

	client := s.getFederatedClient(fmt.Sprintf("xds://%s/%s.%s:%d", federationAuthority, fs.name, fs.namespace, fs.port))
	s.checkEventually(client, "fed-test")
}

// TestLegacyURLStillResolvesUnderFederation verifies that enabling xdstp
// emission does not break the legacy `xds:///foo` URL form. The same xDS
// server should serve both name spaces from the same backend.
func (s *XdsFederationIntegrationTestSuite) TestLegacyURLStillResolvesUnderFederation() {
	fs := s.fakeServices["legacy-app"]
	fs.svc.On("Check", mock.Anything, "legacy-test").Return(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil)

	client := s.getFederatedClient(fmt.Sprintf("xds:///%s.%s:%d", fs.name, fs.namespace, fs.port))
	s.checkEventually(client, "legacy-test")
}

// startFederatedFakeService spins up a backend on the loopback alias, with an
// OS-assigned port, and returns its handle. The kube objects are NOT registered
// here — the caller owns the kube tracker so it can pre-populate everything
// before the snapshotter starts.
func startFederatedFakeService(t *testing.T, name, namespace string, servicePort int32) *federatedFakeService {
	svc, err := test.NewFakeService(fmt.Sprintf("%s:0", federationFakeServiceIP))
	require.NoError(t, err)
	svc.Test(t)
	return &federatedFakeService{
		name:      name,
		namespace: namespace,
		port:      servicePort,
		svc:       svc,
	}
}

// kubeManifests turns a federatedFakeService into the kube Service+Endpoints
// pair that the snapshotter needs to translate it into xDS resources.
func (fs *federatedFakeService) kubeManifests() (*corev1.Service, *corev1.Endpoints) { //nolint:staticcheck // legacy Kube API on purpose
	kubeSvc := (&test.K8SService{
		Name:      fs.name,
		Namespace: fs.namespace,
		Ports: []corev1.ServicePort{{
			Name:     "grpc",
			Port:     fs.port,
			Protocol: corev1.ProtocolTCP,
		}},
	}).AsK8S()

	kubeEp := (&test.K8SEndpoint{
		Name:      fs.name,
		Namespace: fs.namespace,
		IP:        []string{fs.svc.Host()},
		Ports: []corev1.EndpointPort{{ //nolint:staticcheck
			Name: "grpc",
			Port: fs.svc.Port(),
		}},
	}).AsK8S()

	return kubeSvc, kubeEp
}

func TestXdsFederationIntegration(t *testing.T) {
	// Spin up the backend services first so we know their OS-assigned ports.
	legacyApp := startFederatedFakeService(t, "legacy-app", "default", 2)
	fedApp := startFederatedFakeService(t, "fed-app", "default", 1)

	// Pre-populate the kube tracker BEFORE InitializeTestServer so the
	// snapshotter's initial List captures everything. With fake.Clientset,
	// late Tracker().Add() calls can take >15s to surface via Watch, which
	// exceeds gRPC's xDS resource fetch deadline.
	var objects []runtime.Object
	for _, fs := range []*federatedFakeService{legacyApp, fedApp} {
		svc, ep := fs.kubeManifests()
		objects = append(objects, svc, ep)
	}
	kube := fake.NewClientset(objects...)

	testServer, stop, err := di.InitializeTestServer(t.Context(), kube, config.Config{
		StatsIntervalSeconds: 1,
		LocalCluster:         federationAuthority,
	})
	require.NoError(t, err)
	defer stop()

	suite.Run(t, &XdsFederationIntegrationTestSuite{
		TestServer: testServer,
		fakeServices: map[string]*federatedFakeService{
			"legacy-app": legacyApp,
			"fed-app":    fedApp,
		},
	})
}
