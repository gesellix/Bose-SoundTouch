// Package speaker holds protocol-level constants for the Bose SoundTouch
// speaker's local API surface: the well-known HTTP port, the request paths
// exposed by every device, and the on-device file locations the migration
// flow needs to know about.
//
// This package is intentionally a leaf with no internal dependencies, so
// any layer can import it (client library, service, CLI, tests) without
// introducing a cycle or a cross-topic edge. Anything speaker-shaped that
// would otherwise be duplicated between packages belongs here.
package speaker

// HTTPPort is the well-known port the SoundTouch device exposes its local
// API on (e.g. /info, /presets, /group).
const HTTPPort = 8090

// Well-known HTTP paths the SoundTouch device serves on HTTPPort.
const (
	DeviceInfoPath = "/info"
	PresetsPath    = "/presets"
	RecentsPath    = "/recents"
)

// On-device filesystem paths that the migration/sync flow needs to read or
// write over SSH. These live in the device's persistence area and are not
// part of the HTTP surface.
const (
	SourcesFileLocation      = "/mnt/nv/BoseApp-Persistence/1/Sources.xml"
	GroupServiceFileLocation = "/mnt/nv/BoseApp-Persistence/1/GroupService.xml"
)
