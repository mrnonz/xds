package namer

// LocalNamer returns the default-authority Namer — every method returns the
// id unchanged. Use this when emitting resources under their bare names for
// `xds:///foo` URLs (the cluster's own clients).
func LocalNamer() Namer { return localNamer{} }

type localNamer struct{}

func (localNamer) NameListener(id string) string    { return id }
func (localNamer) NameRouteConfig(id string) string { return id }
func (localNamer) NameCluster(id string) string     { return id }
func (localNamer) NameEndpoint(id string) string    { return id }
func (localNamer) Scope() string                    { return "local" }
