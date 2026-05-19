package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	"github.com/go-chi/chi/v5"
)

func newHealthTestServer(t *testing.T) (*httptest.Server, *datastore.DataStore, string, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "handlers-health-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	account, device := "1000001", "DEVICEID01"
	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		Name:      "Speaker",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	_, server := setupRouter("http://localhost:8001", ds)

	r := chi.NewRouter()
	r.Get("/setup/health", server.HandleHealthChecks)
	r.Post("/setup/health/fix", server.HandleHealthFix)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts, ds, account, device
}

func TestHandleHealthChecks_ReportsMissingSourcesXML(t *testing.T) {
	ts, _, account, device := newHealthTestServer(t)

	res, err := http.Get(ts.URL + "/setup/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp struct {
		GeneratedAt string               `json:"generatedAt"`
		Checks      []health.CheckResult `json:"checks"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.GeneratedAt == "" {
		t.Errorf("expected generatedAt to be populated")
	}

	if len(resp.Checks) == 0 {
		t.Fatalf("expected at least one check")
	}

	var found *health.CheckResult
	for i := range resp.Checks {
		if resp.Checks[i].ID == health.CheckIDSourcesXMLPresent {
			found = &resp.Checks[i]
			break
		}
	}

	if found == nil {
		t.Fatalf("expected %q check in response", health.CheckIDSourcesXMLPresent)
	}

	if found.Severity != health.SeverityWarning {
		t.Errorf("expected warning, got %q", found.Severity)
	}

	if len(found.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(found.Findings))
	}

	finding := found.Findings[0]
	if finding.Target.Account != account || finding.Target.Device != device {
		t.Errorf("finding target = %+v, want account=%s device=%s", finding.Target, account, device)
	}
}

func TestHandleHealthFix_MaterialisesAndClearsFinding(t *testing.T) {
	ts, ds, account, device := newHealthTestServer(t)

	type fixReq struct {
		CheckID string        `json:"checkId"`
		FixID   string        `json:"fixId"`
		Target  health.Target `json:"target"`
	}

	body, err := json.Marshal(fixReq{
		CheckID: health.CheckIDSourcesXMLPresent,
		FixID:   health.FixIDCreateDefaultSources,
		Target:  health.Target{Account: account, Device: device},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/health/fix", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var fixResp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&fixResp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !fixResp.OK {
		t.Errorf("expected ok=true, got %+v", fixResp)
	}

	if !ds.HasConfiguredSources(account, device) {
		t.Fatalf("Sources.xml should exist after fix")
	}

	// Subsequent GET should now report SeverityOK for the check.
	res2, err := http.Get(ts.URL + "/setup/health")
	if err != nil {
		t.Fatalf("re-GET: %v", err)
	}
	defer res2.Body.Close()

	var resp struct {
		Checks []health.CheckResult `json:"checks"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for i := range resp.Checks {
		if resp.Checks[i].ID != health.CheckIDSourcesXMLPresent {
			continue
		}
		if resp.Checks[i].Severity != health.SeverityOK {
			t.Errorf("expected OK after fix, got %q", resp.Checks[i].Severity)
		}
		if len(resp.Checks[i].Findings) != 0 {
			t.Errorf("expected zero findings after fix, got %d", len(resp.Checks[i].Findings))
		}
	}
}

func TestHandleHealthFix_UnknownFixReturns404(t *testing.T) {
	ts, _, _, _ := newHealthTestServer(t)

	body, err := json.Marshal(struct {
		CheckID string `json:"checkId"`
		FixID   string `json:"fixId"`
	}{CheckID: "no_such_check", FixID: "no_such_fix"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/health/fix", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", res.StatusCode)
	}
}

func TestHandleHealthFix_BadRequest(t *testing.T) {
	ts, _, _, _ := newHealthTestServer(t)

	res, err := http.Post(ts.URL+"/setup/health/fix", "application/json", bytes.NewReader([]byte(`{"checkId":""}`)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", res.StatusCode)
	}
}
