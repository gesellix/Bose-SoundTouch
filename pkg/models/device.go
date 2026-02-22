package models

import (
	"encoding/xml"
	"time"
)

// DeviceInfo represents the response from GET /info endpoint
type DeviceInfo struct {
	XMLName          xml.Name      `xml:"info"`
	DeviceID         string        `xml:"deviceID,attr"`
	Name             string        `xml:"name"`
	Type             string        `xml:"type"`
	MargeAccountUUID string        `xml:"margeAccountUUID"`
	Components       []Component   `xml:"components>component"`
	MargeURL         string        `xml:"margeURL"`
	NetworkInfo      []NetworkInfo `xml:"networkInfo"`
	ModuleType       string        `xml:"moduleType"`
	Variant          string        `xml:"variant"`
	VariantMode      string        `xml:"variantMode"`
	CountryCode      string        `xml:"countryCode"`
	RegionCode       string        `xml:"regionCode"`
}

// Component represents a device component
type Component struct {
	ComponentCategory string `xml:"componentCategory"`
	SoftwareVersion   string `xml:"softwareVersion"`
	SerialNumber      string `xml:"serialNumber"`
}

// NetworkInfo represents network information for the device
type NetworkInfo struct {
	Type       string `xml:"type,attr"`
	MacAddress string `xml:"macAddress"`
	IPAddress  string `xml:"ipAddress"`
}

// XMLResponse is a generic wrapper for API responses
type XMLResponse struct {
	XMLName xml.Name
	Error   *APIError `xml:"error,omitempty"`
}

// APIError represents an error response from the API
type APIError struct {
	Code    int    `xml:"code,attr"`
	Message string `xml:",chardata"`
}

// Error implements the error interface
func (e *APIError) Error() string {
	return e.Message
}

// DiscoveredDevice represents a device found through network discovery
type DiscoveredDevice struct {
	Name            string    `json:"name"`
	Host            string    `json:"host"`
	Port            int       `json:"port"`
	ModelID         string    `json:"model_id"`
	SerialNo        string    `json:"serial_no"`
	LastSeen        time.Time `json:"last_seen"`
	DiscoveryMethod string    `json:"discovery_method"`

	// Standard URLs
	APIBaseURL string `json:"api_base_url"` // http://host:port/
	InfoURL    string `json:"info_url"`     // http://host:port/info

	// Protocol-specific details
	UPnPLocation string `json:"upnp_location,omitempty"` // UPnP device description XML URL
	UPnPUSN      string `json:"upnp_usn,omitempty"`      // UPnP Unique Service Name
	UPnPSerial   string `json:"upnp_serial,omitempty"`   // Serial number from UPnP (MAC address)
	MDNSHostname string `json:"mdns_hostname,omitempty"` // mDNS hostname (e.g., "device.local.")
	MDNSService  string `json:"mdns_service,omitempty"`  // mDNS service name
	ConfigName   string `json:"config_name,omitempty"`   // Original name from config

	// Additional metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GetStandardURLs returns the standard API URLs for this device
func (d *DiscoveredDevice) GetStandardURLs() map[string]string {
	return map[string]string{
		"base": d.APIBaseURL,
		"info": d.InfoURL,
	}
}

// GetProtocolSpecificData returns protocol-specific information
func (d *DiscoveredDevice) GetProtocolSpecificData() map[string]interface{} {
	data := make(map[string]interface{})

	if d.UPnPLocation != "" {
		data["upnp"] = map[string]string{
			"location": d.UPnPLocation,
			"usn":      d.UPnPUSN,
			"serial":   d.UPnPSerial,
		}
	}

	if d.MDNSHostname != "" {
		data["mdns"] = map[string]string{
			"hostname": d.MDNSHostname,
			"service":  d.MDNSService,
		}
	}

	if d.ConfigName != "" {
		data["config"] = map[string]string{
			"original_name": d.ConfigName,
		}
	}

	return data
}
