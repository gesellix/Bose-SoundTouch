package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleBMXServicesAvailability(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	req := httptest.NewRequest("GET", "/bmx/registry/v1/servicesAvailability", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	services, ok := resp["services"].([]interface{})
	if !ok {
		t.Fatal("Response missing 'services' field")
	}

	if len(services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(services))
	}

	foundTuneIn := false
	foundSiriusXM := false

	for _, s := range services {
		service := s.(map[string]interface{})
		name := service["service"].(string)
		switch name {
		case "TUNEIN":
			foundTuneIn = true
			if service["canAdd"] != true {
				t.Errorf("TUNEIN: expected canAdd true, got %v", service["canAdd"])
			}
			if service["canRemove"] != false {
				t.Errorf("TUNEIN: expected canRemove false, got %v", service["canRemove"])
			}
		case "SIRIUSXM_EVEREST":
			foundSiriusXM = true
			if service["canAdd"] != false {
				t.Errorf("SIRIUSXM_EVEREST: expected canAdd false, got %v", service["canAdd"])
			}
			if service["canRemove"] != true {
				t.Errorf("SIRIUSXM_EVEREST: expected canRemove true, got %v", service["canRemove"])
			}
		}
	}

	if !foundTuneIn {
		t.Error("TUNEIN not found in servicesAvailability")
	}
	if !foundSiriusXM {
		t.Error("SIRIUSXM_EVEREST not found in servicesAvailability")
	}
}
