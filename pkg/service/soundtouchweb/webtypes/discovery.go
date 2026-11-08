package webtypes

// RecordDiscoverySource remembers how a connection was registered. A later
// manual registration promotes an existing connection; discovery cannot undo
// that explicit choice. It reports whether the source changed.
func (c *DeviceConnection) RecordDiscoverySource(source string) bool {
	for {
		previous := c.discoverySource.Load()
		if previous != nil && (*previous == "manual" || source != "manual") {
			return false
		}

		if c.discoverySource.CompareAndSwap(previous, &source) {
			return true
		}
	}
}

// DiscoverySource reports the registration source, or empty for connections
// constructed without discovery metadata.
func (c *DeviceConnection) DiscoverySource() string {
	if source := c.discoverySource.Load(); source != nil {
		return *source
	}

	return ""
}
