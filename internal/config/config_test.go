package config

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterFlags_Defaults(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := Config{}
	RegisterFlags(fs, &cfg)

	err := fs.Parse(nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(300), cfg.StatsIntervalSeconds)
	assert.Equal(t, "", cfg.LocalCluster)
}

func TestRegisterFlags_ValuesParsed(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := Config{}
	RegisterFlags(fs, &cfg)

	err := fs.Parse([]string{"--statsinterval=42", "--local-cluster=alpha"})
	assert.NoError(t, err)
	assert.Equal(t, int64(42), cfg.StatsIntervalSeconds)
	assert.Equal(t, "alpha", cfg.LocalCluster)
}
