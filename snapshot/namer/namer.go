// Package namer holds the Namer interface and its implementations. It lives
// in its own package so both snapshot and snapshot/apigateway can depend on
// it without creating an import cycle (snapshot imports apigateway, so
// apigateway can't import snapshot).
package namer

// Namer translates a resource's logical id into the wire-level name it is
// emitted under. Implementations:
//
//   - LocalNamer returns the id verbatim — emits resources under the default
//     authority so `xds:///foo` keeps working.
//   - XDSTPNamer prefixes the id with `xdstp://<authority>/<type>/` so the
//     resource is addressable via gRPC A47 federation under that authority,
//     e.g. `xds://<authority>/foo`.
//
// Resource emission code threads a Namer through every name and inter-resource
// reference so that one pass produces a self-consistent set of resources whose
// internal references stay within a single naming namespace.
type Namer interface {
	NameListener(id string) string
	NameRouteConfig(id string) string
	NameCluster(id string) string
	NameEndpoint(id string) string

	// Scope returns a short tag identifying this Namer (e.g. "local" or
	// "xdstp:<authority>"). It is opaque — not a resource name — and is
	// meant to be composed by callers into cache keys that distinguish
	// entries produced by different Namers from the same logical id.
	Scope() string
}
