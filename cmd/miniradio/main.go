package main

import (
	"flag"

	radioapp "github.com/edward-ap/miniradio/internal/radioapp"
)

func main() {
	trace := flag.Bool("traceLog", false, "enable verbose libVLC logging to vlc.log")
	flag.Parse()
	radioapp.SetTraceLogEnabled(*trace)

	app := radioapp.NewApp()
	app.Run()
}
