package models

import "encoding/xml"

// ListMediaServersResponse is the XML response from the speaker's
// /listMediaServers endpoint. Real speakers can return an empty self-closing
// element when no servers are visible, so MediaServers may be nil or empty.
type ListMediaServersResponse struct {
	XMLName      xml.Name          `xml:"ListMediaServersResponse"`
	MediaServers []MediaServerInfo `xml:"media_server"`
}

// MediaServerInfo describes a single DLNA media server as reported by the
// speaker's own UPnP/DLNA discovery layer.
type MediaServerInfo struct {
	// ID is the UDN (uuid:...) of the server.
	ID string `xml:"id,attr"`
	// MAC is the server's MAC address when reported by the speaker.
	MAC string `xml:"mac,attr,omitempty"`
	// IP is the LAN IP address the speaker resolved for the server.
	IP string `xml:"ip,attr,omitempty"`
	// Manufacturer is the vendor string from the UPnP description.
	Manufacturer string `xml:"manufacturer,attr,omitempty"`
	// ModelName is the model string from the UPnP description.
	ModelName string `xml:"model_name,attr,omitempty"`
	// FriendlyName is the human-readable server name.
	FriendlyName string `xml:"friendly_name,attr,omitempty"`
	// ModelDescription is the optional long model description.
	ModelDescription string `xml:"model_description,attr,omitempty"`
	// Location is the URL of the device's UPnP root description document.
	Location string `xml:"location,attr,omitempty"`
}
