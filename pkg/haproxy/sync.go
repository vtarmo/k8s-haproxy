package haproxy

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

// Syncer drives HAProxy updates using the Data Plane API client.
type Syncer struct {
	client Client
	port   int32
}

// NewSyncer builds a new Syncer instance.
func NewSyncer(client Client) *Syncer {
	return &Syncer{client: client}
}

// NewSyncerWithPort builds a Syncer that forces a specific backend port if port > 0.
func NewSyncerWithPort(client Client, port int32) *Syncer {
	return &Syncer{client: client, port: port}
}

// Sync converts EndpointSlices or Endpoints to HAProxy backends and pushes them through a transaction.
func (s *Syncer) Sync(ctx context.Context, slices []*discoveryv1.EndpointSlice, endpoints []*corev1.Endpoints, nodeIPs map[string]string) error {
	overridePort := s.port
	backends := BuildBackendsFromEndpointSlices(slices, nodeIPs, overridePort)
	if len(backends) == 0 {
		backends = BuildBackendsFromEndpoints(endpoints, nodeIPs, overridePort)
	}

	healthChecks := HealthCheckConfig{IntervalSeconds: 5, RiseCount: 2, FallCount: 2}
	return s.SyncBackends(ctx, backends, healthChecks)
}

// SyncBackends updates HAProxy backends using a transaction pattern.
func (s *Syncer) SyncBackends(ctx context.Context, backends []BackendServer, health HealthCheckConfig) error {
	txID, err := s.client.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if err != nil {
			_ = s.client.AbortTransaction(ctx, txID)
		}
	}()

	if err = s.client.UpdateBackendsInTransaction(ctx, txID, backends); err != nil {
		return fmt.Errorf("updating backends: %w", err)
	}

	if err = s.client.UpdateHealthChecksInTransaction(ctx, txID, health); err != nil {
		return fmt.Errorf("updating health checks: %w", err)
	}

	if err = s.client.CommitTransaction(ctx, txID); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// BuildBackendsFromEndpointSlices maps EndpointSlices to HAProxy backend server definitions.
func BuildBackendsFromEndpointSlices(slices []*discoveryv1.EndpointSlice, nodeIPs map[string]string, overridePort int32) []BackendServer {
	var servers []BackendServer

	for _, slice := range slices {
		for _, port := range slice.Ports {
			if port.Port == nil {
				continue
			}

			for _, ep := range slice.Endpoints {
				if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
					continue
				}

				p := selectPort(port.Port, overridePort)
				for _, addr := range ep.Addresses {
					host := resolveAddress(addr, ep.NodeName, nodeIPs)
					servers = append(servers, BackendServer{
						Name:    fmt.Sprintf("%s-%d", host, p),
						Address: host,
						Port:    p,
						Weight:  1,
						Check:   true,
					})
				}
			}
		}
	}

	return servers
}

// BuildBackendsFromEndpoints maps Endpoints resources to HAProxy backend server definitions.
func BuildBackendsFromEndpoints(endpoints []*corev1.Endpoints, nodeIPs map[string]string, overridePort int32) []BackendServer {
	var servers []BackendServer

	for _, ep := range endpoints {
		for _, subset := range ep.Subsets {
			for _, port := range subset.Ports {
				p := selectPort(&port.Port, overridePort)
				for _, addr := range subset.Addresses {
					host := resolveAddress(addr.IP, addr.NodeName, nodeIPs)
					servers = append(servers, BackendServer{
						Name:    fmt.Sprintf("%s-%d", host, p),
						Address: host,
						Port:    p,
						Weight:  1,
						Check:   true,
					})
				}
			}
		}
	}

	return servers
}

func resolveAddress(original string, nodeName *string, nodeIPs map[string]string) string {
	if nodeName != nil {
		if ip, ok := nodeIPs[*nodeName]; ok && ip != "" {
			return ip
		}
	}
	return original
}

func selectPort(found *int32, override int32) int32 {
	if override > 0 {
		return override
	}
	if found != nil {
		return *found
	}
	return 0
}
