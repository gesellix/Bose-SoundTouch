package models

import "encoding/xml"

// Group represents a stereo pair of two ST10 SoundTouch speakers.
type Group struct {
	XMLName         xml.Name   `xml:"group"`
	ID              string     `xml:"id,attr,omitempty"`
	Name            string     `xml:"name"`
	MasterDeviceID  string     `xml:"masterDeviceId"`
	Roles           GroupRoles `xml:"roles"`
	SenderIPAddress string     `xml:"senderIPAddress,omitempty"`
	// Status is populated by the device on GET /group (e.g. "GROUP_OK")
	// and omitted from requests we send back.
	Status string `xml:"status,omitempty"`
}

// IsEmpty reports whether the device returned an empty <group/> element,
// which is the speaker's way of saying "no stereo pair configured".
func (g *Group) IsEmpty() bool {
	return g.ID == "" && g.MasterDeviceID == "" && len(g.Roles.Roles) == 0
}

// GroupRoles contains the role assignments for devices in a group.
type GroupRoles struct {
	Roles []GroupRole `xml:"groupRole"`
}

// GroupRole describes the role (LEFT or RIGHT) of a single device in a group.
type GroupRole struct {
	DeviceID  string `xml:"deviceId"`
	Role      string `xml:"role"`
	IPAddress string `xml:"ipAddress,omitempty"`
}
