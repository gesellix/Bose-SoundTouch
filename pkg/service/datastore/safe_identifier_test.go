package datastore

import (
	"os"
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
		{"a@b", false},
		{"a#b", false},
		{"a$b", false},
		{"a%b", false},
		{"a^b", false},
		{"a&b", false},
		{"a*b", false},
		{"a(b", false},
		{"a)b", false},
	}

	for _, test := range tests {
		result := isSafeIdentifier(test.id)
		if result != test.expected {
			t.Errorf("isSafeIdentifier(%q) = %v; expected %v", test.id, result, test.expected)
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
