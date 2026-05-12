package namer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestXDSTPNamer_FormatsPerResourceType(t *testing.T) {
	n := XDSTPNamer("alpha")

	assert.Equal(t, "xdstp://alpha/envoy.config.listener.v3.Listener/foo.default:50051", n.NameListener("foo.default:50051"))
	assert.Equal(t, "xdstp://alpha/envoy.config.route.v3.RouteConfiguration/foo.default:50051", n.NameRouteConfig("foo.default:50051"))
	assert.Equal(t, "xdstp://alpha/envoy.config.cluster.v3.Cluster/foo.default:grpc", n.NameCluster("foo.default:grpc"))
	assert.Equal(t, "xdstp://alpha/envoy.config.endpoint.v3.ClusterLoadAssignment/foo.default:grpc", n.NameEndpoint("foo.default:grpc"))
}

func TestXDSTPNamer_ScopeDistinguishesAuthorities(t *testing.T) {
	a := XDSTPNamer("alpha")
	b := XDSTPNamer("beta")
	local := LocalNamer()

	assert.NotEqual(t, local.Scope(), a.Scope())
	assert.NotEqual(t, a.Scope(), b.Scope())
}
