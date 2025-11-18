package images

import _ "embed"

// Embed radio icons into the binary so runtime does not need the images folder.
//go:embed radio32.png
var Radio32 []byte

//go:embed radio128.png
var Radio128 []byte
