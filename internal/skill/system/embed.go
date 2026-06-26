package system

import "embed"

const skillsRoot = "embed"

//go:embed embed
var skillsFS embed.FS
