package handlers

import (
	"context"
	"log"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

type Server struct {
	ds           *datastore.DataStore
	sm           *setup.Manager
	serverURL    string
	proxyURL     string
	discovering  bool
	proxyRedact  bool
	proxyLogBody bool
}

func NewServer(ds *datastore.DataStore, sm *setup.Manager, serverURL string, proxyRedact bool, proxyLogBody bool) *Server {
	return &Server{
		ds:           ds,
		sm:           sm,
		serverURL:    serverURL,
		proxyURL:     serverURL,
		proxyRedact:  proxyRedact,
		proxyLogBody: proxyLogBody,
	}
}

func (s *Server) DiscoverDevices() {
	s.discovering = true
	defer func() { s.discovering = false }()

	log.Println("Scanning for Bose devices...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc := discovery.NewService(10 * time.Second)
	devices, err := svc.DiscoverDevices(ctx)
	if err != nil {
		log.Printf("Discovery error: %v", err)
		return
	}

	for _, d := range devices {
		log.Printf("Discovered Bose device: %s at %s (Serial: %s)", d.Name, d.Host, d.SerialNo)

		// 1. Check if we already have this device by serial number (best identifier)
		var existingID string // The directory name used for this device

		allDevices, _ := s.ds.ListAllDevices()
		for _, known := range allDevices {
			if d.SerialNo != "" && (known.DeviceID == d.SerialNo || known.DeviceSerialNumber == d.SerialNo) {
				existingID = known.DeviceID
				if existingID == "" {
					existingID = known.IPAddress
				}
				break
			}
		}

		// Use SerialNo if available, otherwise fallback to IP for the datastore directory name
		deviceID := d.SerialNo
		if deviceID == "" {
			// If serial is missing from discovery, try to fetch it from :8090/info
			log.Printf("Serial number missing for %s at %s, attempting live info fetch...", d.Name, d.Host)
			liveInfo, err := s.sm.GetLiveDeviceInfo(d.Host)
			if err == nil && liveInfo.SerialNumber != "" {
				d.SerialNo = liveInfo.SerialNumber
				log.Printf("Successfully retrieved serial number %s for %s via live info", d.SerialNo, d.Host)
			}
		}

		deviceID = d.SerialNo
		if deviceID == "" {
			deviceID = d.Host
		}

		// 2. If we found it by serial but it was stored under an IP-based directory,
		// we should ideally migrate it, but for now, we'll just ensure the Serial one is used.
		// If the IP changed for a known serial, SaveDeviceInfo will overwrite the old IP info
		// if deviceID == existingBySerial.DeviceID.

		info := &models.ServiceDeviceInfo{
			DeviceID:           d.SerialNo,
			Name:               d.Name,
			IPAddress:          d.Host,
			DeviceSerialNumber: d.SerialNo,
			ProductCode:        d.ModelID,
			FirmwareVersion:    "0.0.0", // Unknown from discovery
		}

		// If we had an IP-based entry and now have a Serial, clean up the IP-based entry
		if d.SerialNo != "" && existingID != "" && existingID != d.SerialNo {
			log.Printf("Device %s previously known as %s, migrating to serial-based ID %s", d.Name, existingID, d.SerialNo)
			s.ds.RemoveDevice("default", existingID)
		}

		if err := s.ds.SaveDeviceInfo("default", deviceID, info); err != nil {
			log.Printf("Failed to save device info: %v", err)
		}
	}
}
