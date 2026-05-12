package config

import "flag"

// Config carries runtime knobs parsed from CLI flags. New flags should be
// added here and surfaced via ParseFlags so DI can consume a single value.
type Config struct {
	// StatsIntervalSeconds is the load-reporting service stats update interval.
	StatsIntervalSeconds int64

	// LocalCluster, when non-empty, names the xdstp authority this server is
	// authoritative for. Resources are then emitted twice: once under their
	// bare local names (for `xds:///foo` URLs) and once under
	// `xdstp://<LocalCluster>/<type>/foo` (for `xds://<LocalCluster>/foo`
	// URLs). Leave empty to disable xdstp emission and preserve local-only
	// behavior.
	LocalCluster string
}

// ParseFlags registers the server's CLI flags on flag.CommandLine and parses
// os.Args. Tests should construct Config directly instead of calling this.
func ParseFlags() Config {
	cfg := Config{}
	RegisterFlags(flag.CommandLine, &cfg)
	flag.Parse()
	return cfg
}

// RegisterFlags binds Config fields onto the given FlagSet without parsing.
// Splitting this out from ParseFlags lets tests register against a private
// FlagSet to avoid global-state interference.
func RegisterFlags(fs *flag.FlagSet, cfg *Config) {
	fs.Int64Var(&cfg.StatsIntervalSeconds, "statsinterval", 300, "stats update interval in seconds")
	fs.StringVar(&cfg.LocalCluster, "local-cluster", "", "xdstp authority this server is authoritative for; when set, resources are also emitted under xdstp://<name>/...")
}
