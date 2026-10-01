package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetVolumeRequiresCompleteHTTPReadback(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		valid  bool
		target int
		actual int
	}{
		{name: "missing both", body: `<volume/>`},
		{name: "missing target", body: `<volume><actualvolume>0</actualvolume></volume>`},
		{name: "missing actual", body: `<volume><targetvolume>0</targetvolume></volume>`},
		{name: "empty target", body: `<volume><targetvolume/><actualvolume>0</actualvolume></volume>`},
		{name: "empty actual", body: `<volume><targetvolume>0</targetvolume><actualvolume> </actualvolume></volume>`},
		{name: "malformed target", body: `<volume><targetvolume>loud</targetvolume><actualvolume>0</actualvolume></volume>`},
		{name: "malformed actual", body: `<volume><targetvolume>0</targetvolume><actualvolume>0.0</actualvolume></volume>`},
		{name: "out of range target", body: `<volume><targetvolume>101</targetvolume><actualvolume>0</actualvolume></volume>`},
		{name: "out of range actual", body: `<volume><targetvolume>0</targetvolume><actualvolume>-1</actualvolume></volume>`},
		{name: "explicit zero", body: `<volume><targetvolume>0</targetvolume><actualvolume>0</actualvolume></volume>`, valid: true},
		{name: "ordinary values", body: `<volume><targetvolume>42</targetvolume><actualvolume>40</actualvolume></volume>`, valid: true, target: 42, actual: 40},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/volume" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			volume, err := NewClientFromHost(server.URL).GetVolume()
			if !test.valid {
				if err == nil || volume != nil {
					t.Fatalf("incomplete/invalid readback = %+v, error %v", volume, err)
				}
				return
			}
			if err != nil || volume == nil || volume.TargetVolume != test.target || volume.ActualVolume != test.actual {
				t.Fatalf("readback = %+v, error %v, want target %d actual %d", volume, err, test.target, test.actual)
			}
		})
	}
}
