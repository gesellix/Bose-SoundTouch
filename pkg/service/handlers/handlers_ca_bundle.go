package handlers

import (
	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/service/health"
)

// restoreAndInjectCAFix is the FixFunc registered for
// (CheckIDSpeakerCABundle, FixIDRestoreAndInjectCA). It handles the case
// where original factory CA certificates have gone missing from the live
// bundle by:
//
//  1. Copying .original back over the live ca-bundle.crt via SSH.
//  2. Re-injecting the AfterTouch CA via TrustCACert so the speaker
//     continues to trust AfterTouch's TLS certificate.
func (s *Server) restoreAndInjectCAFix(target health.Target) (string, error) {
	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	deviceIP, err := s.resolveDeviceIDToIP(target.Device)
	if err != nil {
		return "", fmt.Errorf("locate device %s: %w", target.Device, err)
	}

	if err := s.sm.RestoreCABundleFromOriginal(deviceIP); err != nil {
		return "", fmt.Errorf("restore CA bundle on device %s: %w", target.Device, err)
	}

	if _, err := s.sm.TrustCACert(deviceIP); err != nil {
		return "", fmt.Errorf("re-inject AfterTouch CA on device %s after restore: %w", target.Device, err)
	}

	return fmt.Sprintf(
		"Device %s: original CA bundle restored from factory backup and AfterTouch CA re-injected.",
		target.Device,
	), nil
}

// injectCACertFix is the FixFunc registered for
// (CheckIDSpeakerCABundle, FixIDInjectCACert). It handles the case where
// the AfterTouch CA is absent from the live bundle (e.g. removed manually
// or not yet injected).
func (s *Server) injectCACertFix(target health.Target) (string, error) {
	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	deviceIP, err := s.resolveDeviceIDToIP(target.Device)
	if err != nil {
		return "", fmt.Errorf("locate device %s: %w", target.Device, err)
	}

	if _, err := s.sm.TrustCACert(deviceIP); err != nil {
		return "", fmt.Errorf("inject AfterTouch CA on device %s: %w", target.Device, err)
	}

	return fmt.Sprintf(
		"Device %s: AfterTouch CA certificate installed in speaker's bundle.",
		target.Device,
	), nil
}
