package netbird

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthHeaderAndListZones(t *testing.T) {
	var gotAuth, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		_ = json.NewEncoder(w).Encode([]Zone{
			{ID: "z1", Domain: "example.com", Records: []Record{
				{ID: "r1", Name: "www.example.com", Type: "A", Content: "10.0.0.1", TTL: 300},
			}},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-pat")
	zones, err := c.ListZones(context.Background())
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if gotAuth != "Token test-pat" {
		t.Errorf("unexpected auth header: %q", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("unexpected accept header: %q", gotAccept)
	}
	if len(zones) != 1 || zones[0].ID != "z1" || len(zones[0].Records) != 1 {
		t.Errorf("unexpected zones: %+v", zones)
	}
}

func TestAPIErrorRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pat")
	_, err := c.ListZones(context.Background())
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusBadGateway {
		t.Errorf("unexpected status: %d", apiErr.StatusCode)
	}
	if !apiErr.Retryable() {
		t.Error("5xx should be retryable")
	}
}

func TestAPIErrorNotFoundNotRetryable(t *testing.T) {
	err := &APIError{StatusCode: http.StatusNotFound, Body: "nope"}
	if err.Retryable() {
		t.Error("404 should not be retryable")
	}
}

func TestCRUDRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var rec CreateRecord
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(Record{ID: "new", Name: rec.Name, Type: rec.Type, Content: rec.Content, TTL: rec.TTL})
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(Record{ID: "r1", Name: "www.example.com", Type: "A", Content: "10.0.0.2", TTL: 60})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c := NewClient(srv.URL, "pat")
	created, err := c.CreateRecord(ctx, "z1", CreateRecord{Name: "www.example.com", Type: "A", Content: "10.0.0.1", TTL: 300})
	if err != nil || created.ID != "new" {
		t.Fatalf("CreateRecord: %+v %v", created, err)
	}
	updated, err := c.UpdateRecord(ctx, "z1", "r1", UpdateRecord{Name: "www.example.com", Type: "A", Content: "10.0.0.2", TTL: 60})
	if err != nil || updated.Content != "10.0.0.2" {
		t.Fatalf("UpdateRecord: %+v %v", updated, err)
	}
	if err := c.DeleteRecord(ctx, "z1", "r1"); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	c := NewClient("", "pat")
	if c.baseURL != DefaultBaseURL {
		t.Errorf("expected default base URL, got %q", c.baseURL)
	}
}
