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
