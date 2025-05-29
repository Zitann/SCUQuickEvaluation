package main

import (
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
)

func main() {
	go func() {
		// Create window
		w := &app.Window{}
		w.Option(app.Title("SCU 快速评教系统 - Go & Gio UI"))
		w.Option(app.Size(unit.Dp(600), unit.Dp(500)))

		// Create and run application
		appInstance := NewApp(w)
		if err := appInstance.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	// Start the main event loop
	app.Main()
}
