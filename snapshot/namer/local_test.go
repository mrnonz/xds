package namer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalNamer_ReturnsIdVerbatim(t *testing.T) {
	n := LocalNamer()
	assert.Equal(t, "foo.default:50051", n.NameListener("foo.default:50051"))
	assert.Equal(t, "foo.default:50051", n.NameRouteConfig("foo.default:50051"))
	assert.Equal(t, "foo.default:grpc", n.NameCluster("foo.default:grpc"))
	assert.Equal(t, "foo.default:grpc", n.NameEndpoint("foo.default:grpc"))
	assert.Equal(t, "local", n.Scope())
}
