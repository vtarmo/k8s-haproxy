package haproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
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
	baseURL     *url.URL
	backendName string
	client      *http.Client
	username    string
	password    string
	token       string
}

// NewDataPlaneClient creates a new DataPlaneClient using the given base URL and backend name.
func NewDataPlaneClient(baseURL, username, password, token, backendName string) *DataPlaneClient {
	parsed, _ := url.Parse(baseURL)
	return &DataPlaneClient{
		baseURL:     parsed,
		backendName: backendName,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		username: username,
		password: password,
		token:    token,
	}
}

// BeginTransaction starts a new transaction in HAProxy Data Plane API.
func (c *DataPlaneClient) BeginTransaction(ctx context.Context) (string, error) {
	var resp transactionResponse
	if err := c.doRequest(ctx, http.MethodPost, "/v2/services/haproxy/transactions", nil, nil, &resp); err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("begin transaction: empty transaction id")
	}
	return resp.ID, nil
}

// CommitTransaction finalizes a transaction.
func (c *DataPlaneClient) CommitTransaction(ctx context.Context, transactionID string) error {
	if transactionID == "" {
		return fmt.Errorf("commit transaction: empty transaction id")
	}
	path := fmt.Sprintf("/v2/services/haproxy/transactions/%s", transactionID)
	return c.doRequest(ctx, http.MethodPut, path, nil, nil, nil)
}

// AbortTransaction rolls back a transaction.
func (c *DataPlaneClient) AbortTransaction(ctx context.Context, transactionID string) error {
	if transactionID == "" {
		return fmt.Errorf("abort transaction: empty transaction id")
	}
	path := fmt.Sprintf("/v2/services/haproxy/transactions/%s", transactionID)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil, nil)
}

// UpdateBackendsInTransaction updates backend servers within a transaction.
func (c *DataPlaneClient) UpdateBackendsInTransaction(ctx context.Context, transactionID string, backends []BackendServer) error {
	for _, b := range backends {
		payload := serverPayload{
			Name:    b.Name,
			Address: b.Address,
			Port:    b.Port,
			Weight:  b.Weight,
			Check:   b.Check,
		}
		values := url.Values{}
		values.Set("backend", c.backendName)
		values.Set("transaction_id", transactionID)
		resourcePath := path.Join("/v2/services/haproxy/configuration/servers", b.Name)
		if err := c.doRequest(ctx, http.MethodPut, resourcePath, values, payload, nil); err != nil {
			return fmt.Errorf("update server %s: %w", b.Name, err)
		}
	}
	return nil
}

// UpdateHealthChecksInTransaction updates health check configuration within a transaction.
func (c *DataPlaneClient) UpdateHealthChecksInTransaction(ctx context.Context, transactionID string, config HealthCheckConfig) error {
	backendPath := fmt.Sprintf("/v2/services/haproxy/configuration/backends/%s", c.backendName)
	payload := map[string]any{
		"name": c.backendName,
		// Minimal health check tuning; servers also have Check=true for per-server checks.
		"check_timeout": fmt.Sprintf("%dms", config.IntervalSeconds*1000),
	}
	values := url.Values{}
	values.Set("transaction_id", transactionID)
	return c.doRequest(ctx, http.MethodPut, backendPath, values, payload, nil)
}

type transactionResponse struct {
	ID string `json:"id"`
}

type serverPayload struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int32  `json:"port"`
	Weight  int    `json:"weight,omitempty"`
	Check   bool   `json:"check,omitempty"`
}

func (c *DataPlaneClient) doRequest(ctx context.Context, method, p string, query url.Values, body any, out any) error {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, p)
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var buf io.ReadWriter
	if body != nil {
		buf = &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
