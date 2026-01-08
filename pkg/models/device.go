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

// DiscoveredDevice represents a device found through UPnP discovery
type DiscoveredDevice struct {
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	ModelID  string    `json:"model_id"`
	SerialNo string    `json:"serial_no"`
	Location string    `json:"location"`
	LastSeen time.Time `json:"last_seen"`
}
