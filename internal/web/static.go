package web

import "embed"

//go:embed all:static
var staticFS embed.FS

// StaticFS returns the embedded static assets filesystem.
func StaticFS() embed.FS { return staticFS }
