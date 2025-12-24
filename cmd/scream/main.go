package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	// 1. Initialize the TLS Client immediately on startup
	if err := InitClient(); err != nil {
		log.Panic("Could not initialize TLS client: " + err.Error())
	}

	mainApp = app.New()

	// Apply VFD Theme
	mainApp.Settings().SetTheme(&vfdTheme{})

	window = mainApp.NewWindow("Scream-NG (VFD Terminal)")
	window.Resize(fyne.NewSize(1000, 800))

	// Start listener routine
	go ListenForMessages()

	// Show Login Screen initially
	window.SetContent(MakeLoginScreen(func() {
		window.SetContent(MakeMainScreen())
	}))

	window.ShowAndRun()
}
