package soundtouchweb

import "embed"

// StaticFS is the embedded static-asset filesystem for the web UI.
// Callers typically take a sub-FS rooted at "static" before serving.
//
//go:embed static
var StaticFS embed.FS
