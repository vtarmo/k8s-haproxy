package haproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

const apiVersionPath = "/v3"

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
	version, err := c.fetchConfigurationVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch version: %w", err)
	}

	var resp transactionResponse
	values := url.Values{}
	values.Set("version", fmt.Sprintf("%d", version))

	if err := c.doRequest(ctx, http.MethodPost, apiVersionPath+"/services/haproxy/transactions", values, nil, &resp); err != nil {
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
	path := fmt.Sprintf(apiVersionPath+"/services/haproxy/transactions/%s", transactionID)
	return c.doRequest(ctx, http.MethodPut, path, nil, nil, nil)
}

// AbortTransaction rolls back a transaction.
func (c *DataPlaneClient) AbortTransaction(ctx context.Context, transactionID string) error {
	if transactionID == "" {
		return fmt.Errorf("abort transaction: empty transaction id")
	}
	path := fmt.Sprintf(apiVersionPath+"/services/haproxy/transactions/%s", transactionID)
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
			Check:   checkState(b.Check),
		}
		values := url.Values{}
		values.Set("transaction_id", transactionID)
		resourcePath := path.Join(apiVersionPath, "services/haproxy/configuration/backends", c.backendName, "servers", b.Name)
		if err := c.doRequest(ctx, http.MethodPut, resourcePath, values, payload, nil); err != nil {
			var apiErr *apiStatusError
			if errors.As(err, &apiErr) && apiErr.statusCode == http.StatusNotFound {
				createPath := path.Join(apiVersionPath, "services/haproxy/configuration/backends", c.backendName, "servers")
				if err := c.doRequest(ctx, http.MethodPost, createPath, values, payload, nil); err != nil {
					return fmt.Errorf("create server %s: %w", b.Name, err)
				}
				continue
			}
			return fmt.Errorf("update server %s: %w", b.Name, err)
		}
	}
	return nil
}

// UpdateHealthChecksInTransaction updates health check configuration within a transaction.
func (c *DataPlaneClient) UpdateHealthChecksInTransaction(ctx context.Context, transactionID string, config HealthCheckConfig) error {
	backendPath := fmt.Sprintf(apiVersionPath+"/services/haproxy/configuration/backends/%s", c.backendName)
	payload := map[string]any{
		"name": c.backendName,
		// Minimal health check tuning; servers also have Check=true for per-server checks.
		"check_timeout": config.IntervalSeconds * 1000,
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
	Check   string `json:"check,omitempty"`
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
		return &apiStatusError{statusCode: resp.StatusCode, body: string(data)}
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

func decodeVersion(body io.Reader) (int64, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 32<<10))
	if err != nil {
		return 0, fmt.Errorf("read version body: %w", err)
	}

	// Try as plain number.
	var num int64
	if err := json.Unmarshal(raw, &num); err == nil && num > 0 {
		return num, nil
	}

	// Try as object {"version": N}.
	var obj struct {
		Version int64 `json:"version"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Version > 0 {
		return obj.Version, nil
	}

	return 0, fmt.Errorf("unexpected version payload: %s", string(raw))
}

type apiStatusError struct {
	statusCode int
	body       string
}

func (e *apiStatusError) Error() string {
	return fmt.Sprintf("status %d: %s", e.statusCode, e.body)
}

func checkState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (c *DataPlaneClient) fetchConfigurationVersion(ctx context.Context) (int64, error) {
	u := fmt.Sprintf("%s/services/haproxy/configuration/version", apiVersionPath)

	reqURL := *c.baseURL
	reqURL.Path = path.Join(c.baseURL.Path, u)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	httpResp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4<<10))
		return 0, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(data))
	}

	version, err := decodeVersion(httpResp.Body)
	if err != nil {
		return 0, err
	}
	if version == 0 {
		return 0, fmt.Errorf("configuration version is zero")
	}
	return version, nil
}
