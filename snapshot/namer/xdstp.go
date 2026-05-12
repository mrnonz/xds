package namer

import (
	"fmt"
	"strings"

	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

// XDSTPNamer returns a Namer that emits resource names under the given xdstp
// authority. authority must be non-empty; callers gate construction on the
// `--local-cluster` flag being set.
func XDSTPNamer(authority string) Namer { return xdstpNamer{authority: authority} }

type xdstpNamer struct {
	authority string
}

func (n xdstpNamer) NameListener(id string) string    { return n.format(resource.ListenerType, id) }
func (n xdstpNamer) NameRouteConfig(id string) string { return n.format(resource.RouteType, id) }
func (n xdstpNamer) NameCluster(id string) string     { return n.format(resource.ClusterType, id) }
func (n xdstpNamer) NameEndpoint(id string) string    { return n.format(resource.EndpointType, id) }

func (n xdstpNamer) Scope() string { return "xdstp:" + n.authority }

// format builds an xdstp resource name. The go-control-plane type constants
// include a `type.googleapis.com/` prefix; the xdstp spec uses just the proto
// FQN, so we trim it.
func (n xdstpNamer) format(typeURL, id string) string {
	return fmt.Sprintf("xdstp://%s/%s/%s", n.authority, strings.TrimPrefix(typeURL, resource.APITypePrefix), id)
}
