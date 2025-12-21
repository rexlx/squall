package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	// 1. Initialize the TLS Client immediately on startup
	if err := InitClient(); err != nil {
		// If we can't load certs, crash immediately with an error message
		log.Panic("Could not initialize TLS client: " + err.Error())
	}

	mainApp = app.New()
	window = mainApp.NewWindow("Scream-NG (Secure)")
	window.Resize(fyne.NewSize(1000, 800))

	// Start listener routine
	go ListenForMessages()

	// Show Login Screen initially
	window.SetContent(MakeLoginScreen(func() {
		// On success, switch to Main Screen
		window.SetContent(MakeMainScreen())
	}))

	window.ShowAndRun()
}
