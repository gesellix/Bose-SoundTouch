package client

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

const completeBalanceReadback = `<balance deviceID="DEVICEID01">` +
	`<balanceAvailable>true</balanceAvailable>` +
	`<balanceMin>-7</balanceMin>` +
	`<balanceMax>7</balanceMax>` +
	`<balanceDefault>0</balanceDefault>` +
	`<targetBalance>-3</targetBalance>` +
	`<actualBalance>-2</actualBalance>` +
	`</balance>`

func decodeBalanceReadback(t *testing.T, body string) (*models.Balance, error) {
	t.Helper()

	var readback balanceReadback
	if err := xml.Unmarshal([]byte(body), &readback); err != nil {
		t.Fatalf("xml.Unmarshal: %v", err)
	}

	return readback.balance()
}

func TestBalanceReadbackUnavailableAllowsOmittedCompanionsAndPreservesStaleValues(t *testing.T) {
	tests := []struct {
		name string
		body string
		want models.Balance
	}{
		{
			name: "companions omitted",
			body: `<balance deviceID="UNPAIRED"><balanceAvailable>false</balanceAvailable></balance>`,
			want: models.Balance{DeviceID: "UNPAIRED"},
		},
		{
			name: "incoherent stale companions retained",
			body: `<balance deviceID="UNPAIRED"><balanceAvailable>false</balanceAvailable>` +
				`<balanceMin>12</balanceMin><balanceMax>-12</balanceMax>` +
				`<balanceDefault>99</balanceDefault><targetBalance>123</targetBalance>` +
				`<actualBalance>-456</actualBalance></balance>`,
			want: models.Balance{
				DeviceID: "UNPAIRED",
				Min:      12,
				Max:      -12,
				Default:  99,
				Target:   123,
				Actual:   -456,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeBalanceReadback(t, test.body)
			if err != nil {
				t.Fatalf("balance(): %v", err)
			}

			if got.Available {
				t.Error("Available = true, want false")
			}
			if got.XMLName.Local != "balance" {
				t.Errorf("XMLName.Local = %q, want balance", got.XMLName.Local)
			}
			if got.DeviceID != test.want.DeviceID || got.Min != test.want.Min || got.Max != test.want.Max ||
				got.Default != test.want.Default || got.Target != test.want.Target || got.Actual != test.want.Actual {
				t.Errorf("balance = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestBalanceReadbackRequiresValidAvailability(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: `<balance/>`},
		{name: "empty", body: `<balance><balanceAvailable/></balance>`},
		{name: "malformed", body: `<balance><balanceAvailable>sometimes</balanceAvailable></balance>`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			balance, err := decodeBalanceReadback(t, test.body)
			if err == nil || balance != nil {
				t.Fatalf("balance = %+v, error = %v; want failure", balance, err)
			}
			if !strings.Contains(err.Error(), "balanceAvailable") {
				t.Errorf("error = %v, want balanceAvailable context", err)
			}
		})
	}
}

func TestAvailableBalanceReadbackRequiresEveryNumericField(t *testing.T) {
	fields := []string{"balanceMin", "balanceMax", "balanceDefault", "targetBalance", "actualBalance"}

	for _, field := range fields {
		open := "<" + field + ">"
		closeTag := "</" + field + ">"
		start := strings.Index(completeBalanceReadback, open)
		end := strings.Index(completeBalanceReadback, closeTag)
		if start < 0 || end < 0 {
			t.Fatalf("fixture does not contain %s", field)
		}
		end += len(closeTag)

		tests := []struct {
			name        string
			replacement string
		}{
			{name: "missing", replacement: ""},
			{name: "empty", replacement: open + closeTag},
			{name: "nonnumeric", replacement: open + "unknown" + closeTag},
		}

		for _, test := range tests {
			t.Run(field+"/"+test.name, func(t *testing.T) {
				body := completeBalanceReadback[:start] + test.replacement + completeBalanceReadback[end:]
				balance, err := decodeBalanceReadback(t, body)
				if err == nil || balance != nil {
					t.Fatalf("balance = %+v, error = %v; want failure", balance, err)
				}
				if !strings.Contains(err.Error(), field) {
					t.Errorf("error = %v, want %s context", err, field)
				}
			})
		}
	}
}

func TestAvailableBalanceReadbackRequiresCoherentAdvertisedBounds(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "minimum above maximum",
			body: strings.Replace(completeBalanceReadback,
				`<balanceMin>-7</balanceMin>`, `<balanceMin>8</balanceMin>`, 1),
		},
		{
			name: "default below minimum",
			body: strings.Replace(completeBalanceReadback,
				`<balanceDefault>0</balanceDefault>`, `<balanceDefault>-8</balanceDefault>`, 1),
		},
		{
			name: "default above maximum",
			body: strings.Replace(completeBalanceReadback,
				`<balanceDefault>0</balanceDefault>`, `<balanceDefault>8</balanceDefault>`, 1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			balance, err := decodeBalanceReadback(t, test.body)
			if err == nil || balance != nil {
				t.Fatalf("balance = %+v, error = %v; want failure", balance, err)
			}
		})
	}
}

func TestAvailableBalanceReadbackAcceptsExplicitZeroRange(t *testing.T) {
	body := `<balance><balanceAvailable>true</balanceAvailable>` +
		`<balanceMin>0</balanceMin><balanceMax>0</balanceMax>` +
		`<balanceDefault>0</balanceDefault><targetBalance>0</targetBalance>` +
		`<actualBalance>0</actualBalance></balance>`

	balance, err := decodeBalanceReadback(t, body)
	if err != nil {
		t.Fatalf("balance(): %v", err)
	}
	if !balance.Available || balance.Min != 0 || balance.Max != 0 || balance.Default != 0 ||
		balance.Target != 0 || balance.Actual != 0 {
		t.Errorf("balance = %+v, want complete available 0..0 readback", balance)
	}
}

func TestAvailableBalanceReadbackPreservesReportedLevelsOutsideAdvertisedRange(t *testing.T) {
	body := strings.Replace(completeBalanceReadback,
		`<targetBalance>-3</targetBalance><actualBalance>-2</actualBalance>`,
		`<targetBalance>99</targetBalance><actualBalance>99</actualBalance>`,
		1,
	)

	balance, err := decodeBalanceReadback(t, body)
	if err != nil {
		t.Fatalf("balance(): %v", err)
	}
	if balance.Target != 99 || balance.Actual != 99 {
		t.Errorf("balance = %+v, want reported target and actual 99 preserved", balance)
	}
}
