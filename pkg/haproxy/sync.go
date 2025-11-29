package haproxy

import (
	"context"
	"fmt"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// Syncer drives HAProxy updates using the Data Plane API client.
type Syncer struct {
	client Client
}

// NewSyncer builds a new Syncer instance.
func NewSyncer(client Client) *Syncer {
	return &Syncer{client: client}
}

// SyncFromEndpointSlices converts EndpointSlices to HAProxy backends and pushes them through a transaction.
func (s *Syncer) SyncFromEndpointSlices(ctx context.Context, slices []*discoveryv1.EndpointSlice) error {
	backends := BuildBackendsFromEndpointSlices(slices)
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
func BuildBackendsFromEndpointSlices(slices []*discoveryv1.EndpointSlice) []BackendServer {
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

				for _, addr := range ep.Addresses {
					servers = append(servers, BackendServer{
						Name:    fmt.Sprintf("%s-%d", addr, *port.Port),
						Address: addr,
						Port:    *port.Port,
						Weight:  1,
						Check:   true,
					})
				}
			}
		}
	}

	return servers
}
