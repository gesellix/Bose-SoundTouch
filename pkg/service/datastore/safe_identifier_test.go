package datastore

import (
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestIsSafeIdentifier(t *testing.T) {
	tests := []struct {
		id       string
		expected bool
	}{
		{"abc", true},
		{"ABC", true},
		{"123", true},
		{"abc_123", true},
		{"abc-123", true},
		{"abc.123", true},
		{"00:11:22:33:44:55", true},
		// #634: third-party/manual pairing tools (e.g. the USB-stick
		// SSH-enable method) can report a non-numeric margeAccountUUID.
		{"stick@local", true},
		{strings.Repeat("a", maxSafeIdentifierLength), true},
		{"", false},
		{"/", false},
		{"\\", false},
		{"..", false},
		{"../etc/passwd", false},
		{"/etc/passwd", false},
		{"a/b", false},
		{"a\\b", false},
		{"a..b", false},
		{"a b", false},
		{"a!b", false},
		{"a#b", false},
		{"a$b", false},
		{"a%b", false},
		{"a^b", false},
		{"a&b", false},
		{"a*b", false},
		{"a(b", false},
		{"a)b", false},
		{"a<b", false},
		{"a>b", false},
		{`a"b`, false},
		{"a'b", false},
		{strings.Repeat("a", maxSafeIdentifierLength+1), false},
	}

	for _, test := range tests {
		result := IsSafeIdentifier(test.id)
		if result != test.expected {
			t.Errorf("IsSafeIdentifier(%q) = %v; expected %v", test.id, result, test.expected)
		}
	}
}

func TestSaveDeviceInfo_Validation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datastore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ds := NewDataStore(tmpDir)
	info := &models.ServiceDeviceInfo{DeviceID: "dev1"}

	tests := []struct {
		account string
		device  string
		wantErr bool
		errMsg  string
	}{
		{"acc1", "dev1", false, ""},
		{"", "dev1", true, "account ID cannot be empty"},
		{"acc1", "", true, "device ID/name cannot be empty"},
		{"acc/1", "dev1", true, "invalid account ID"},
		{"acc1", "dev/1", true, "invalid device ID"},
		{"acc..1", "dev1", true, "invalid account ID"},
		{"acc1", "dev..1", true, "invalid device ID"},
		// #634: a non-numeric margeAccountUUID is now accepted.
		{"stick@local", "dev1", false, ""},
	}

	for _, test := range tests {
		err := ds.SaveDeviceInfo(test.account, test.device, info)
		if (err != nil) != test.wantErr {
			t.Errorf("SaveDeviceInfo(%q, %q) error = %v, wantErr %v", test.account, test.device, err, test.wantErr)
			continue
		}
		if test.wantErr && err.Error() != test.errMsg {
			t.Errorf("SaveDeviceInfo(%q, %q) error message = %q, want %q", test.account, test.device, err.Error(), test.errMsg)
		}
	}
}

func TestSaveAccountInfo_Validation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datastore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ds := NewDataStore(tmpDir)

	tests := []struct {
		account string
		wantErr bool
		errMsg  string
	}{
		{"acc1", false, ""},
		// #634: a non-numeric margeAccountUUID reported via
		// POST /streaming/account (see HandleMargeCreateAccount) must
		// be validated the same way SaveDeviceInfo already validates
		// device-reported account IDs.
		{"stick@local", false, ""},
		{"acc/1", true, "invalid account ID"},
		{"acc..1", true, "invalid account ID"},
		{"a<b", true, "invalid account ID"},
	}

	for _, test := range tests {
		err := ds.SaveAccountInfo(test.account, &models.ServiceAccountInfo{AccountID: test.account})
		if (err != nil) != test.wantErr {
			t.Errorf("SaveAccountInfo(%q) error = %v, wantErr %v", test.account, err, test.wantErr)
			continue
		}
		if test.wantErr && err.Error() != test.errMsg {
			t.Errorf("SaveAccountInfo(%q) error message = %q, want %q", test.account, err.Error(), test.errMsg)
		}
	}
}
