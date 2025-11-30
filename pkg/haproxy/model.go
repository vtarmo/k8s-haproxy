package haproxy

// BackendServer represents a single HAProxy backend server entry.
type BackendServer struct {
	Name    string
	Address string
	Port    int32
	Weight  int
	Check   bool
}

// HealthCheckConfig holds basic health check configuration for HAProxy backends.
type HealthCheckConfig struct {
	IntervalSeconds int
	RiseCount       int
	FallCount       int
	SendProxyV2     bool
}
