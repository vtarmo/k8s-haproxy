package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds controller runtime configuration sourced from environment variables.
type Config struct {
	HAProxyBaseURL     string
	HAProxyUsername    string
	HAProxyPassword    string
	HAProxyToken       string
	HAProxyBackendName string
	IngressNamespace   string
	IngressServiceName string
	WorkerCount        int
	ResyncPeriod       time.Duration
	KubeconfigPath     string
}

// Load reads configuration from environment variables and applies defaults where needed.
func Load() (Config, error) {
	cfg := Config{
		IngressNamespace:   getEnv("INGRESS_NAMESPACE", "ingress-nginx"),
		IngressServiceName: getEnv("INGRESS_SERVICE_NAME", "ingress-nginx"),
		HAProxyBaseURL:     getEnv("HAPROXY_DATAPLANE_URL", "http://haproxy:5555"),
		HAProxyBackendName: getEnv("HAPROXY_BACKEND_NAME", ""),
		WorkerCount:        2,
		ResyncPeriod:       30 * time.Second,
		KubeconfigPath:     os.Getenv("KUBECONFIG"),
		HAProxyUsername:    os.Getenv("HAPROXY_DATAPLANE_USERNAME"),
		HAProxyPassword:    os.Getenv("HAPROXY_DATAPLANE_PASSWORD"),
		HAProxyToken:       os.Getenv("HAPROXY_DATAPLANE_TOKEN"),
	}

	if v := os.Getenv("WORKER_COUNT"); v != "" {
		count, err := strconv.Atoi(v)
		if err != nil || count < 1 {
			return Config{}, fmt.Errorf("invalid WORKER_COUNT value %q: %w", v, err)
		}
		cfg.WorkerCount = count
	}

	if v := os.Getenv("RESYNC_PERIOD"); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid RESYNC_PERIOD value %q: %w", v, err)
		}
		cfg.ResyncPeriod = dur
	}

	if cfg.HAProxyBackendName == "" {
		cfg.HAProxyBackendName = cfg.IngressServiceName
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}
