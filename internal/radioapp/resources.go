package radioapp

import (
	"fyne.io/fyne/v2"
	"github.com/edward-ap/miniradio/images"
)

// Embedded application icons (PNG), bundled into the binary via images package.
// We pick a sensible default (32px) for the runtime window/app icon.
var (
	RadioIcon32  fyne.Resource
	RadioIcon128 fyne.Resource

	// AppIcon is the default icon used for the app and window.
	AppIcon fyne.Resource
)

func init() {
	// Build static resources from embedded PNGs
	if len(images.Radio32) > 0 {
		RadioIcon32 = fyne.NewStaticResource("radio32.png", images.Radio32)
	}
	if len(images.Radio128) > 0 {
		RadioIcon128 = fyne.NewStaticResource("radio128.png", images.Radio128)
	}

	// Choose 32px as a good default for runtime window/taskbar icon on Windows.
	if RadioIcon32 != nil {
		AppIcon = RadioIcon32
	} else if RadioIcon128 != nil {
		AppIcon = RadioIcon128
	}
}
