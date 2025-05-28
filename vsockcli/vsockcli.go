package vsockcli

import (
	"context"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var (
	json      = jsoniter.ConfigCompatibleWithStandardLibrary
	globalCli VsockCli
)

const (
	defaultCliCount       = 4
	defaultMaxConnsPerCli = 16
	defaultTimeoutSec     = 3                        // default timeout for http client
	defaultOrigin         = "http://localhost:50001" // default origin for enclave client
)

type VsockCli interface {
	Get(context context.Context, fullPath string) ([]byte, error)
	Post(context context.Context, fullPath string, req any) ([]byte, error)
}

func MustInit(cfg *Config) {
	cli, err := newEnclaveClient(cfg)
	if err != nil {
		panic("failed to create enclave client: " + err.Error())
	}
	globalCli = cli
}

func Get() VsockCli {
	return globalCli
}

type Config struct {
	IsMocked       bool // is mocked for local development
	CID            uint32
	Port           uint32
	Origin         string
	CliCount       int // number of clients
	MaxConnsPerCli int // max connections per client to the enclave
	Timeout        time.Duration
}

func (cfg *Config) setupDefaults() *Config {
	clone := cfg
	if cfg == nil {
		clone = &Config{}
	}
	if clone.Origin == "" {
		cfg.Origin = defaultOrigin
	}
	if cfg.CliCount == 0 {
		cfg.CliCount = defaultCliCount
	}
	if cfg.MaxConnsPerCli == 0 {
		cfg.MaxConnsPerCli = defaultMaxConnsPerCli
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeoutSec * time.Second
	}
	return clone
}
