package models

import "testing"

func TestDeviceError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      DeviceError
		expected string
	}{
		{
			name:     "message repeats the numeric value (real speaker case)",
			err:      DeviceError{Value: 1047, Name: "SOURCE_ALREADY_REMOVED", Message: "1047"},
			expected: "SOURCE_ALREADY_REMOVED (1047)",
		},
		{
			name:     "message is empty",
			err:      DeviceError{Value: 1047, Name: "SOURCE_ALREADY_REMOVED", Message: ""},
			expected: "SOURCE_ALREADY_REMOVED (1047)",
		},
		{
			name:     "message is meaningful and distinct from name",
			err:      DeviceError{Value: 1029, Name: "UNKNOWN_ACTION_ERROR", Message: "This version of SCM does not support spotify create account functionality."},
			expected: "UNKNOWN_ACTION_ERROR: This version of SCM does not support spotify create account functionality.",
		},
		{
			name:     "name is empty, message carries the detail",
			err:      DeviceError{Value: 500, Name: "", Message: "internal error"},
			expected: "internal error",
		},
		{
			name:     "both name and message are empty",
			err:      DeviceError{Value: 500, Name: "", Message: ""},
			expected: "device error 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestErrorsResponse_Error(t *testing.T) {
	t.Run("delegates to the first DeviceError", func(t *testing.T) {
		errs := &ErrorsResponse{
			Errors: []DeviceError{
				{Value: 1047, Name: "SOURCE_ALREADY_REMOVED", Message: "1047"},
			},
		}

		expected := "SOURCE_ALREADY_REMOVED (1047)"
		if got := errs.Error(); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("no errors", func(t *testing.T) {
		errs := &ErrorsResponse{}

		expected := "unknown API error"
		if got := errs.Error(); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})
}
