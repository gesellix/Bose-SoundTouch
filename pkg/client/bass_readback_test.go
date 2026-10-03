package client

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

const completeBassReadback = `<bass deviceID="DEVICEID01">` +
	`<targetbass>-3</targetbass>` +
	`<actualbass>-2</actualbass>` +
	`</bass>`

const completeBassCapabilitiesReadback = `<bassCapabilities deviceID="DEVICEID01">` +
	`<bassAvailable>true</bassAvailable>` +
	`<bassMin>-9</bassMin>` +
	`<bassMax>9</bassMax>` +
	`<bassDefault>0</bassDefault>` +
	`</bassCapabilities>`

func decodeBassReadback(t *testing.T, body string) (*models.Bass, error) {
	t.Helper()

	var readback bassReadback
	if err := xml.Unmarshal([]byte(body), &readback); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	return readback.bass()
}

func decodeBassCapabilitiesReadback(t *testing.T, body string) (*models.BassCapabilities, error) {
	t.Helper()

	var readback bassCapabilitiesReadback
	if err := xml.Unmarshal([]byte(body), &readback); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	return readback.bassCapabilities()
}

func TestBassReadbackRequiresEveryNumericField(t *testing.T) {
	for _, field := range []string{"targetbass", "actualbass"} {
		open := "<" + field + ">"
		closeTag := "</" + field + ">"
		start := strings.Index(completeBassReadback, open)
		end := strings.Index(completeBassReadback, closeTag)
		if start < 0 || end < 0 {
			t.Fatalf("fixture does not contain %s", field)
		}
		end += len(closeTag)

		for _, test := range []struct {
			name        string
			replacement string
		}{
			{name: "missing"},
			{name: "empty", replacement: open + closeTag},
			{name: "nonnumeric", replacement: open + "unknown" + closeTag},
			{name: "overflow", replacement: open + "999999999999999999999999999999999999" + closeTag},
		} {
			t.Run(field+"/"+test.name, func(t *testing.T) {
				body := completeBassReadback[:start] + test.replacement + completeBassReadback[end:]
				bass, err := decodeBassReadback(t, body)
				if err == nil || bass != nil {
					t.Fatalf("bass = %+v, error = %v; want failure", bass, err)
				}
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error = %v, want %s context", err, field)
				}
			})
		}
	}
}

func TestBassReadbackUsesExistingLevelValidation(t *testing.T) {
	t.Run("valid boundaries", func(t *testing.T) {
		body := `<bass><targetbass>-9</targetbass><actualbass>9</actualbass></bass>`
		bass, err := decodeBassReadback(t, body)
		if err != nil {
			t.Fatalf("bass(): %v", err)
		}
		if bass.TargetBass != -9 || bass.ActualBass != 9 {
			t.Errorf("bass = %+v, want target -9 and actual 9", bass)
		}
	})

	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "target below model range",
			body: `<bass><targetbass>-10</targetbass><actualbass>0</actualbass></bass>`,
		},
		{
			name: "actual above model range",
			body: `<bass><targetbass>0</targetbass><actualbass>10</actualbass></bass>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bass, err := decodeBassReadback(t, test.body)
			if err == nil || bass != nil {
				t.Fatalf("bass = %+v, error = %v; want model range failure", bass, err)
			}
		})
	}
}

func TestBassReadbackAcceptsExplicitZero(t *testing.T) {
	bass, err := decodeBassReadback(t, `<bass><targetbass>0</targetbass><actualbass>0</actualbass></bass>`)
	if err != nil {
		t.Fatalf("bass(): %v", err)
	}
	if bass.TargetBass != 0 || bass.ActualBass != 0 {
		t.Errorf("bass = %+v, want explicit zero levels", bass)
	}
}

func TestBassCapabilitiesReadbackRequiresValidAvailability(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "missing", body: `<bassCapabilities/>`},
		{name: "empty", body: `<bassCapabilities><bassAvailable/></bassCapabilities>`},
		{name: "whitespace", body: `<bassCapabilities><bassAvailable>  </bassAvailable></bassCapabilities>`},
		{name: "malformed", body: `<bassCapabilities><bassAvailable>sometimes</bassAvailable></bassCapabilities>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			capabilities, err := decodeBassCapabilitiesReadback(t, test.body)
			if err == nil || capabilities != nil {
				t.Fatalf("capabilities = %+v, error = %v; want failure", capabilities, err)
			}
			if !strings.Contains(err.Error(), "bassAvailable") {
				t.Errorf("error = %v, want bassAvailable context", err)
			}
		})
	}
}

func TestAvailableBassCapabilitiesRequiresEveryNumericField(t *testing.T) {
	for _, field := range []string{"bassMin", "bassMax", "bassDefault"} {
		open := "<" + field + ">"
		closeTag := "</" + field + ">"
		start := strings.Index(completeBassCapabilitiesReadback, open)
		end := strings.Index(completeBassCapabilitiesReadback, closeTag)
		if start < 0 || end < 0 {
			t.Fatalf("fixture does not contain %s", field)
		}
		end += len(closeTag)

		for _, test := range []struct {
			name        string
			replacement string
		}{
			{name: "missing"},
			{name: "empty", replacement: open + closeTag},
			{name: "nonnumeric", replacement: open + "unknown" + closeTag},
			{name: "overflow", replacement: open + "999999999999999999999999999999999999" + closeTag},
		} {
			t.Run(field+"/"+test.name, func(t *testing.T) {
				body := completeBassCapabilitiesReadback[:start] + test.replacement + completeBassCapabilitiesReadback[end:]
				capabilities, err := decodeBassCapabilitiesReadback(t, body)
				if err == nil || capabilities != nil {
					t.Fatalf("capabilities = %+v, error = %v; want failure", capabilities, err)
				}
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error = %v, want %s context", err, field)
				}
			})
		}
	}
}

func TestAvailableBassCapabilitiesRequiresCoherentAdvertisedBounds(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "minimum above maximum",
			body: strings.Replace(completeBassCapabilitiesReadback,
				`<bassMin>-9</bassMin>`, `<bassMin>10</bassMin>`, 1),
		},
		{
			name: "default below minimum",
			body: strings.Replace(completeBassCapabilitiesReadback,
				`<bassDefault>0</bassDefault>`, `<bassDefault>-10</bassDefault>`, 1),
		},
		{
			name: "default above maximum",
			body: strings.Replace(completeBassCapabilitiesReadback,
				`<bassDefault>0</bassDefault>`, `<bassDefault>10</bassDefault>`, 1),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capabilities, err := decodeBassCapabilitiesReadback(t, test.body)
			if err == nil || capabilities != nil {
				t.Fatalf("capabilities = %+v, error = %v; want failure", capabilities, err)
			}
		})
	}
}

func TestAvailableBassCapabilitiesAcceptsExplicitZeroRange(t *testing.T) {
	body := `<bassCapabilities><bassAvailable>true</bassAvailable>` +
		`<bassMin>0</bassMin><bassMax>0</bassMax><bassDefault>0</bassDefault>` +
		`</bassCapabilities>`

	capabilities, err := decodeBassCapabilitiesReadback(t, body)
	if err != nil {
		t.Fatalf("bassCapabilities(): %v", err)
	}
	if !capabilities.BassAvailable || capabilities.BassMin != 0 ||
		capabilities.BassMax != 0 || capabilities.BassDefault != 0 {
		t.Errorf("capabilities = %+v, want complete available 0..0 readback", capabilities)
	}
}

func TestUnavailableBassCapabilitiesAllowsOmittedCompanionsAndPreservesStaleValues(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want models.BassCapabilities
	}{
		{
			name: "companions omitted",
			body: `<bassCapabilities deviceID="UNSUPPORTED">` +
				`<bassAvailable>false</bassAvailable></bassCapabilities>`,
			want: models.BassCapabilities{DeviceID: "UNSUPPORTED"},
		},
		{
			name: "incoherent stale companions retained",
			body: `<bassCapabilities deviceID="UNSUPPORTED">` +
				`<bassAvailable>false</bassAvailable>` +
				`<bassMin>12</bassMin><bassMax>-12</bassMax><bassDefault>99</bassDefault>` +
				`</bassCapabilities>`,
			want: models.BassCapabilities{
				DeviceID:    "UNSUPPORTED",
				BassMin:     12,
				BassMax:     -12,
				BassDefault: 99,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capabilities, err := decodeBassCapabilitiesReadback(t, test.body)
			if err != nil {
				t.Fatalf("bassCapabilities(): %v", err)
			}
			if capabilities.BassAvailable {
				t.Error("BassAvailable = true, want false")
			}
			if capabilities.DeviceID != test.want.DeviceID || capabilities.BassMin != test.want.BassMin ||
				capabilities.BassMax != test.want.BassMax || capabilities.BassDefault != test.want.BassDefault {
				t.Errorf("capabilities = %+v, want %+v", capabilities, test.want)
			}
		})
	}
}

func TestUnavailableBassCapabilitiesRejectsMalformedPresentCompanions(t *testing.T) {
	for _, field := range []string{"bassMin", "bassMax", "bassDefault"} {
		for _, value := range []string{"", "unknown", "999999999999999999999999999999999999"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				body := `<bassCapabilities><bassAvailable>false</bassAvailable><` + field + `>` + value +
					`</` + field + `></bassCapabilities>`
				capabilities, err := decodeBassCapabilitiesReadback(t, body)
				if err == nil || capabilities != nil {
					t.Fatalf("capabilities = %+v, error = %v; want failure", capabilities, err)
				}
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error = %v, want %s context", err, field)
				}
			})
		}
	}
}

func TestClientBassReadbacksConvertOnlyValidatedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/bass":
			_, _ = w.Write([]byte(completeBassReadback))
		case "/bassCapabilities":
			_, _ = w.Write([]byte(completeBassCapabilitiesReadback))
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()

	client := NewClientFromHost(server.URL)

	bass, err := client.GetBass()
	if err != nil {
		t.Fatalf("GetBass: %v", err)
	}
	if bass.XMLName.Local != "bass" || bass.DeviceID != "DEVICEID01" ||
		bass.TargetBass != -3 || bass.ActualBass != -2 {
		t.Errorf("GetBass = %+v, want validated converted response", bass)
	}

	capabilities, err := client.GetBassCapabilities()
	if err != nil {
		t.Fatalf("GetBassCapabilities: %v", err)
	}
	if capabilities.XMLName.Local != "bassCapabilities" || capabilities.DeviceID != "DEVICEID01" ||
		!capabilities.BassAvailable || capabilities.BassMin != -9 || capabilities.BassMax != 9 ||
		capabilities.BassDefault != 0 {
		t.Errorf("GetBassCapabilities = %+v, want validated converted response", capabilities)
	}
}

func TestClientBassReadbacksReturnNilOnTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientFromHost(server.URL)

	bass, err := client.GetBass()
	if err == nil || bass != nil {
		t.Fatalf("GetBass = %+v, %v; want nil result and transport error", bass, err)
	}

	capabilities, err := client.GetBassCapabilities()
	if err == nil || capabilities != nil {
		t.Fatalf("GetBassCapabilities = %+v, %v; want nil result and transport error", capabilities, err)
	}
}
