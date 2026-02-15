package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMargeStockholmHandlers(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("HandleMargeAccountProfile GET", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/customer/account/12345")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "<accountID>12345</accountID>") {
			t.Errorf("Response missing account ID: %s", string(body))
		}
	})

	t.Run("HandleMargeUpdateAccountProfile POST", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/customer/account/12345", "application/xml", strings.NewReader("<profile/>"))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})

	t.Run("HandleMargeChangePassword POST", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/customer/account/12345/password", "application/xml", strings.NewReader("<password/>"))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})

	t.Run("HandleMargeGetEmailAddress GET", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/account/12345/emailaddress")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "user@example.com") {
			t.Errorf("Response missing email: %s", string(body))
		}
	})

	t.Run("HandleMargeGetDeviceSettings GET", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/device_setting/account/123/device/DEV1/device_settings")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "CLOCK_FORMAT") {
			t.Errorf("Response missing settings: %s", string(body))
		}
	})

	t.Run("HandleMargeUpdateDeviceSettings POST", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/marge/streaming/device_setting/account/123/device/DEV1/device_settings", "application/xml", strings.NewReader("<settings/>"))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})
}
