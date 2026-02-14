package handlers

import (
	"context"
	"log"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// Server handles HTTP requests for the SoundTouch service.
type Server struct {
	ds           *datastore.DataStore
	sm           *setup.Manager
	serverURL    string
	proxyURL     string
	discovering  bool
	proxyRedact  bool
	proxyLogBody bool
	recorder     *proxy.Recorder
}

// NewServer creates a new SoundTouch service server.
func NewServer(ds *datastore.DataStore, sm *setup.Manager, serverURL string, proxyRedact, proxyLogBody bool) *Server {
	return &Server{
		ds:           ds,
		sm:           sm,
		serverURL:    serverURL,
		proxyURL:     serverURL,
		proxyRedact:  proxyRedact,
		proxyLogBody: proxyLogBody,
	}
}

// SetRecorder sets the recorder for the server.
func (s *Server) SetRecorder(r *proxy.Recorder) {
	s.recorder = r
}

// DiscoverDevices starts a background device discovery process.
//
//nolint:contextcheck
func (s *Server) DiscoverDevices(ctx context.Context) {
	s.discovering = true

	defer func() { s.discovering = false }()

	log.Println("Scanning for Bose devices...")

	// Use background context if none provided or if it's likely a request context
	if ctx == nil {
		ctx = context.Background()
	}

	// Always wrap in a timeout to prevent hanging forever
	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	svc := discovery.NewService(10 * time.Second)

	devices, err := svc.DiscoverDevices(discoveryCtx)
	if err != nil {
		log.Printf("Discovery error: %v", err)
		return
	}

	for _, d := range devices {
		s.handleDiscoveredDevice(*d)
	}
}

func (s *Server) handleDiscoveredDevice(d models.DiscoveredDevice) {
	log.Printf("Discovered Bose device: %s at %s (Serial: %s)", d.Name, d.Host, d.SerialNo)

	// 1. Check if we already have this device by serial number (best identifier)
	existingID := s.findExistingDeviceID(d)

	// Use SerialNo if available, otherwise fallback to IP for the datastore directory name
	if d.SerialNo == "" {
		// If serial is missing from discovery, try to fetch it from :8090/info
		log.Printf("Serial number missing for %s at %s, attempting live info fetch...", d.Name, d.Host)

		liveInfo, err := s.sm.GetLiveDeviceInfo(d.Host)
		if err == nil && liveInfo.SerialNumber != "" {
			d.SerialNo = liveInfo.SerialNumber
			log.Printf("Successfully retrieved serial number %s for %s via live info", d.SerialNo, d.Host)
		}
	}

	deviceID := d.SerialNo
	if deviceID == "" {
		deviceID = d.Host
	}

	info := &models.ServiceDeviceInfo{
		DeviceID:           d.SerialNo,
		Name:               d.Name,
		IPAddress:          d.Host,
		DeviceSerialNumber: d.SerialNo,
		ProductCode:        d.ModelID,
		FirmwareVersion:    "0.0.0", // Unknown from discovery
		DiscoveryMethod:    d.DiscoveryMethod,
	}

	// If we had an IP-based entry and now have a Serial, clean up the IP-based entry
	if d.SerialNo != "" && existingID != "" && existingID != d.SerialNo {
		log.Printf("Device %s previously known as %s, migrating to serial-based ID %s", d.Name, existingID, d.SerialNo)
		_ = s.ds.RemoveDevice("default", existingID)
	}

	if err := s.ds.SaveDeviceInfo("default", deviceID, info); err != nil {
		log.Printf("Failed to save device info: %v", err)
	}
}

func (s *Server) findExistingDeviceID(d models.DiscoveredDevice) string {
	allDevices, _ := s.ds.ListAllDevices()
	for i := range allDevices {
		known := allDevices[i]
		if d.SerialNo != "" && (known.DeviceID == d.SerialNo || known.DeviceSerialNumber == d.SerialNo) {
			if known.DeviceID != "" {
				return known.DeviceID
			}

			return known.IPAddress
		}
	}

	return ""
}
