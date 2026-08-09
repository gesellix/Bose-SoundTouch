package models

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"time"
)

// DeviceInfo represents the response from GET /info endpoint
type DeviceInfo struct {
	XMLName          xml.Name      `xml:"info" json:"-"`
	DeviceID         string        `xml:"deviceID,attr" json:"device_id"`
	Name             string        `xml:"name" json:"name"`
	Type             string        `xml:"type" json:"type"`
	MargeAccountUUID string        `xml:"margeAccountUUID" json:"marge_account_uuid,omitempty"`
	Components       []Component   `xml:"components>component" json:"components,omitempty"`
	MargeURL         string        `xml:"margeURL" json:"marge_url,omitempty"`
	NetworkInfo      []NetworkInfo `xml:"networkInfo" json:"network_info,omitempty"`
	ModuleType       string        `xml:"moduleType" json:"module_type,omitempty"`
	Variant          string        `xml:"variant" json:"variant,omitempty"`
	VariantMode      string        `xml:"variantMode" json:"variant_mode,omitempty"`
	CountryCode      string        `xml:"countryCode" json:"country_code,omitempty"`
	RegionCode       string        `xml:"regionCode" json:"region_code,omitempty"`
	IPAddress        string        `xml:"-" json:"ip_address,omitempty"`
}

// Component represents a device component
type Component struct {
	ComponentCategory string `xml:"componentCategory" json:"component_category"`
	SoftwareVersion   string `xml:"softwareVersion" json:"software_version"`
	SerialNumber      string `xml:"serialNumber" json:"serial_number"`
}

// NetworkInfo represents network information for the device
type NetworkInfo struct {
	Type       string `xml:"type,attr" json:"type"`
	MacAddress string `xml:"macAddress" json:"mac_address"`
	IPAddress  string `xml:"ipAddress" json:"ip_address"`
}

// SourcesUpdatedNotification represents the notification XML sent to the device
type SourcesUpdatedNotification struct {
	XMLName  xml.Name `xml:"updates"`
	DeviceID string   `xml:"deviceID,attr"`
	Sources  struct {
		XMLName xml.Name `xml:"sourcesUpdated"`
	} `xml:"sourcesUpdated"`
}

// NewSourcesUpdatedNotification creates a new sources updated notification
func NewSourcesUpdatedNotification(deviceID string) *SourcesUpdatedNotification {
	return &SourcesUpdatedNotification{
		DeviceID: deviceID,
	}
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

// ErrorsResponse represents a multi-error response from the API (common in some firmware versions)
type ErrorsResponse struct {
	XMLName  xml.Name      `xml:"errors"`
	DeviceID string        `xml:"deviceID,attr"`
	Errors   []DeviceError `xml:"error"`
}

// Error implements the error interface for ErrorsResponse
func (e *ErrorsResponse) Error() string {
	if len(e.Errors) > 0 {
		return e.Errors[0].Error()
	}

	return "unknown API error"
}

// DeviceError represents a single error in an ErrorsResponse
type DeviceError struct {
	Value   int    `xml:"value,attr"`
	Name    string `xml:"name,attr"`
	Message string `xml:",chardata"`
}

// Error implements the error interface for DeviceError. Some speakers
// return a Message that just restates Value as text (e.g. a bare "1047"
// for an error the firmware has no localized string for) — Name is the
// only informative part in that case, so it's always included unless
// Message already carries it.
func (e DeviceError) Error() string {
	if e.Name == "" {
		if e.Message == "" {
			return fmt.Sprintf("device error %d", e.Value)
		}

		return e.Message
	}

	if e.Message == "" || e.Message == e.Name || e.Message == strconv.Itoa(e.Value) {
		return fmt.Sprintf("%s (%d)", e.Name, e.Value)
	}

	return fmt.Sprintf("%s: %s", e.Name, e.Message)
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
	Manufacturer string `json:"manufacturer,omitempty"`  // Manufacturer from UPnP device description (used to reject non-Bose devices)
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
