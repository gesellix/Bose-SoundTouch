package models

import "encoding/xml"

// Name represents the response from /name endpoint
type Name struct {
	XMLName xml.Name `xml:"name"`
	Value   string   `xml:",chardata"`
}

// GetName returns the device name
func (n *Name) GetName() string {
	return n.Value
}

// IsEmpty returns true if the name is empty
func (n *Name) IsEmpty() bool {
	return n.Value == ""
}

// String returns the device name as a string
func (n *Name) String() string {
	return n.Value
}
