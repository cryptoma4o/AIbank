package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return New(srv.URL), srv
}

func TestCreate_Success(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tenants" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var got CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got.ID != "demo" || got.BIK != "044525974" {
			t.Errorf("unexpected body: %+v", got)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Tenant{
			ID: "demo", Name: "Demo", BIK: "044525974", INN: "7700000000",
			Status: "trial", DeploymentMode: "saas",
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
	})
	defer srv.Close()

	got, err := c.Create(context.Background(), CreateRequest{
		ID: "demo", Name: "Demo", BIK: "044525974", INN: "7700000000", DeploymentMode: "saas",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "demo" || got.Status != "trial" {
		t.Errorf("unexpected response: %+v", got)
	}
}

func TestCreate_APIError(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "validation_failed", "message": "bik must be 9 chars"},
		})
	})
	defer srv.Close()

	_, err := c.Create(context.Background(), CreateRequest{ID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Code != "validation_failed" {
		t.Errorf("unexpected APIError: %+v", apiErr)
	}
}

func TestList_FilterAndDecode(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tenants" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []Tenant{
				{ID: "a", Status: "trial"},
				{ID: "b", Status: "active"},
				{ID: "c", Status: "active"},
			},
			"count": 3,
		})
	})
	defer srv.Close()

	all, err := c.List(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3, got %d", len(all))
	}
	active, err := c.List(context.Background(), "active", 0)
	if err != nil {
		t.Fatalf("List(active): %v", err)
	}
	if len(active) != 2 {
		t.Errorf("want 2 active, got %d", len(active))
	}
}

func TestGet_Success(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tenants/demo" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Tenant{ID: "demo", Status: "active"})
	})
	defer srv.Close()
	t1, err := c.Get(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if t1.ID != "demo" {
		t.Errorf("want demo, got %s", t1.ID)
	}
}

func TestUpdateConfig_Success(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/tenants/demo/config" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req UpdateConfigRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.SchemaVersion != "1.0" || len(req.RawConfig) == 0 {
			t.Errorf("bad body: %+v", req)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tenant_id":"demo"}`))
	})
	defer srv.Close()

	err := c.UpdateConfig(context.Background(), "demo", UpdateConfigRequest{
		RawConfig: []byte("hello"), SchemaVersion: "1.0",
	})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
}
