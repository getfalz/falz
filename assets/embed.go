package assets

import "embed"

// TODO there should not be any assets on the project.
// the user should be able to set this values
//
//go:embed *.css *.js
var EmbedFiles embed.FS
