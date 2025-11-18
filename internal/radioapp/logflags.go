package radioapp

import playerpkg "github.com/edward-ap/miniradio/internal/player"

// SetTraceLogEnabled toggles verbose/file logging for libVLC initialisation.
// Call this before creating the App so Player.Init can see the flag.
func SetTraceLogEnabled(b bool) { playerpkg.SetTraceLoggingEnabled(b) }
