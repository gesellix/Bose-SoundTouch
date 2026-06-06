package client

import (
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_Post_ErrorsResponse(t *testing.T) {
	// Mock speaker error response
	errorXML := `<?xml version="1.0" encoding="UTF-8" ?>
<errors deviceID="AABBCCDDEE0A">
    <error value="1029" name="UNKNOWN_ACTION_ERROR" severity="Unknown">This version of SCM does not support spotify create account functionality.</error>
</errors>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(errorXML))
	}))
	defer server.Close()

	c := createTestClient(server.URL)

	// Test post method
	err := c.post("/test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errs := &models.ErrorsResponse{}
	ok := errors.As(err, &errs)
	if !ok {
		t.Fatalf("expected models.ErrorsResponse, got %T: %v", err, err)
	}

	if errs.DeviceID != "AABBCCDDEE0A" {
		t.Errorf("expected DeviceID AABBCCDDEE0A, got %s", errs.DeviceID)
	}

	if len(errs.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs.Errors))
	}

	if errs.Errors[0].Value != 1029 {
		t.Errorf("expected error value 1029, got %d", errs.Errors[0].Value)
	}

	if errs.Errors[0].Name != "UNKNOWN_ACTION_ERROR" {
		t.Errorf("expected error name UNKNOWN_ACTION_ERROR, got %s", errs.Errors[0].Name)
	}

	expectedMsg := "This version of SCM does not support spotify create account functionality."
	if errs.Errors[0].Message != expectedMsg {
		t.Errorf("expected message '%s', got '%s'", expectedMsg, errs.Errors[0].Message)
	}

	if err.Error() != expectedMsg {
		t.Errorf("expected Error() to return '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestClient_PostWithResponse_ErrorsResponse(t *testing.T) {
	// Mock speaker error response
	errorXML := `<?xml version="1.0" encoding="UTF-8" ?>
<errors deviceID="AABBCCDDEE0A">
    <error value="1029" name="UNKNOWN_ACTION_ERROR" severity="Unknown">This version of SCM does not support spotify create account functionality.</error>
</errors>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(errorXML))
	}))
	defer server.Close()

	c := createTestClient(server.URL)

	// Test postWithResponse method
	var result struct {
		XMLName xml.Name `xml:"status"`
		Data    string   `xml:",chardata"`
	}
	err := c.postWithResponse("/test", nil, &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errs := &models.ErrorsResponse{}
	ok := errors.As(err, &errs)
	if !ok {
		t.Fatalf("expected models.ErrorsResponse, got %T: %v", err, err)
	}

	if errs.Errors[0].Value != 1029 {
		t.Errorf("expected error value 1029, got %d", errs.Errors[0].Value)
	}
}

func TestClient_Post_StandardAPIError(t *testing.T) {
	// Mock standard API error response
	errorXML := `<?xml version="1.0" encoding="UTF-8"?><error code="404">Not Found</error>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(errorXML))
	}))
	defer server.Close()

	c := createTestClient(server.URL)

	err := c.post("/test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr := &models.APIError{}
	ok := errors.As(err, &apiErr)
	if !ok {
		t.Fatalf("expected models.APIError, got %T: %v", err, err)
	}

	if apiErr.Code != 404 {
		t.Errorf("expected code 404, got %d", apiErr.Code)
	}

	if apiErr.Message != "Not Found" {
		t.Errorf("expected message 'Not Found', got '%s'", apiErr.Message)
	}
}
