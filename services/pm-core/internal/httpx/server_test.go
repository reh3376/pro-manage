package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	rr := httptest.NewRecorder()
	Health("pm-core", "test")(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["service"] != "pm-core" || body["version"] != "test" {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestReadyReportsFailingCheck(t *testing.T) {
	checks := map[string]ReadyFunc{
		"ok":  func(context.Context) error { return nil },
		"bad": func(context.Context) error { return errors.New("down") },
	}
	rr := httptest.NewRecorder()
	Ready("pm-core", checks)(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rr.Code)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "not_ready" || body.Checks["ok"] != "ok" || body.Checks["bad"] != "fail: down" {
		t.Fatalf("unexpected body %+v", body)
	}
}

func TestReadyAllGood(t *testing.T) {
	rr := httptest.NewRecorder()
	Ready("pm-core", map[string]ReadyFunc{"x": func(context.Context) error { return nil }})(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}
