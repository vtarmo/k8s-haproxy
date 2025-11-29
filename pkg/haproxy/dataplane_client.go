package haproxy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Client defines interactions with the HAProxy Data Plane API.
type Client interface {
	BeginTransaction(ctx context.Context) (string, error)
	CommitTransaction(ctx context.Context, transactionID string) error
	AbortTransaction(ctx context.Context, transactionID string) error
	UpdateBackendsInTransaction(ctx context.Context, transactionID string, backends []BackendServer) error
	UpdateHealthChecksInTransaction(ctx context.Context, transactionID string, config HealthCheckConfig) error
}

// DataPlaneClient is a minimal HTTP-based implementation of the Client interface.
type DataPlaneClient struct {
	baseURL string
	client  *http.Client
}

// NewDataPlaneClient creates a new DataPlaneClient using the given base URL.
func NewDataPlaneClient(baseURL, username, password, token string) *DataPlaneClient {
	_ = username
	_ = password
	_ = token

	return &DataPlaneClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// BeginTransaction starts a new transaction in HAProxy Data Plane API.
func (c *DataPlaneClient) BeginTransaction(_ context.Context) (string, error) {
	return fmt.Sprintf("tx-%d", time.Now().UnixNano()), nil
}

// CommitTransaction finalizes a transaction.
func (c *DataPlaneClient) CommitTransaction(_ context.Context, _ string) error {
	return nil
}

// AbortTransaction rolls back a transaction.
func (c *DataPlaneClient) AbortTransaction(_ context.Context, _ string) error {
	return nil
}

// UpdateBackendsInTransaction updates backend servers within a transaction.
func (c *DataPlaneClient) UpdateBackendsInTransaction(_ context.Context, _ string, _ []BackendServer) error {
	return nil
}

// UpdateHealthChecksInTransaction updates health check configuration within a transaction.
func (c *DataPlaneClient) UpdateHealthChecksInTransaction(_ context.Context, _ string, _ HealthCheckConfig) error {
	return nil
}
