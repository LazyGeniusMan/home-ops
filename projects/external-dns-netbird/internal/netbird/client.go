// Package netbird implements a minimal NetBird Public API client for the
// DNS Custom Zone endpoints used by the ExternalDNS webhook provider.
//
// API docs: NetBird Public API, resources "DNS Zones"
// (GET/POST /api/dns/zones, GET/POST /api/dns/zones/{zoneId}/records,
// PUT/DELETE /api/dns/zones/{zoneId}/records/{recordId}).
// Auth: "Authorization: Token <PAT>".
package netbird

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the NetBird SaaS Public API endpoint.
	DefaultBaseURL = "https://api.netbird.io"

	defaultTimeout = 30 * time.Second
	maxBodyBytes  = 8 << 20 // 8 MiB safety cap on API response bodies
)

// Zone is a NetBird custom DNS zone.
type Zone struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Domain             string   `json:"domain"`
	Enabled            bool     `json:"enabled"`
	EnableSearchDomain bool     `json:"enable_search_domain"`
	DistributionGroups []string `json:"distribution_groups"`
	Records            []Record `json:"records,omitempty"`
}

// Record is a DNS record inside a NetBird custom DNS zone.
// Supported types per API docs: A, AAAA, CNAME.
type Record struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
}

// CreateRecord is the request body for POST /api/dns/zones/{zoneId}/records.
type CreateRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
}

// UpdateRecord is the request body for PUT /api/dns/zones/{zoneId}/records/{recordId}.
type UpdateRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
}

// Client is a minimal NetBird Public API client scoped to DNS zones.
type Client struct {
	baseURL    string
	httpClient *http.Client
	pat        string
}

// NewClient builds a Client. pat is the NetBird personal access token
// (service-user token recommended); it is sent as "Authorization: Token <pat>".
func NewClient(baseURL, pat string) *Client {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		pat:        pat,
	}
}

// ListZones returns all custom DNS zones (GET /api/dns/zones).
func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	var zones []Zone
	if err := c.do(ctx, http.MethodGet, "/api/dns/zones", nil, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

// GetZone returns a single zone (GET /api/dns/zones/{zoneId}).
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error) {
	var zone Zone
	if err := c.do(ctx, http.MethodGet, "/api/dns/zones/"+url.PathEscape(zoneID), nil, &zone); err != nil {
		return nil, err
	}
	return &zone, nil
}

// ListRecords returns all DNS records in a zone (GET /api/dns/zones/{zoneId}/records).
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	var records []Record
	if err := c.do(ctx, http.MethodGet, "/api/dns/zones/"+url.PathEscape(zoneID)+"/records", nil, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// CreateRecord creates a DNS record in a zone (POST /api/dns/zones/{zoneId}/records).
func (c *Client) CreateRecord(ctx context.Context, zoneID string, rec CreateRecord) (*Record, error) {
	var created Record
	if err := c.do(ctx, http.MethodPost, "/api/dns/zones/"+url.PathEscape(zoneID)+"/records", rec, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateRecord updates a DNS record (PUT /api/dns/zones/{zoneId}/records/{recordId}).
func (c *Client) UpdateRecord(ctx context.Context, zoneID, recordID string, rec UpdateRecord) (*Record, error) {
	var updated Record
	path := "/api/dns/zones/" + url.PathEscape(zoneID) + "/records/" + url.PathEscape(recordID)
	if err := c.do(ctx, http.MethodPut, path, rec, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteRecord deletes a DNS record (DELETE /api/dns/zones/{zoneId}/records/{recordId}).
func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	path := "/api/dns/zones/" + url.PathEscape(zoneID) + "/records/" + url.PathEscape(recordID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// APIError is a non-2xx response from the NetBird API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("netbird: request failed with status %d: %s", e.StatusCode, e.Body)
}

// Retryable reports whether the failure is transient (5xx or 429) and the
// caller should surface a soft error so ExternalDNS retries.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || (e.StatusCode >= 500 && e.StatusCode <= 599)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return fmt.Errorf("netbird: encode request body: %w", err)
		}
		rdr = buf
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("netbird: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+c.pat)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("netbird: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("netbird: read response body: %w", err)
	}
	if int64(len(respBody)) > maxBodyBytes {
		return fmt.Errorf("netbird: response body exceeds %d bytes", maxBodyBytes)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: truncate(string(respBody), 1024)}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("netbird: decode response: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
