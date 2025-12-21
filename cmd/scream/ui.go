package main

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	mainApp fyne.App
	window  fyne.Window

	// UI Elements for Chat
	messagesBox *fyne.Container
	scrollBox   *container.Scroll
)

func MakeLoginScreen(onSuccess func()) fyne.CanvasObject {
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("Username/Email")

	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Password")

	errorLabel := widget.NewLabel("")
	errorLabel.Hide()

	loginBtn := widget.NewButton("Login", func() {
		err := Client.Login(emailEntry.Text, passEntry.Text)
		if err != nil {
			errorLabel.SetText(err.Error())
			errorLabel.Show()
		} else {
			onSuccess()
		}
	})
	loginBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		widget.NewLabelWithStyle("scream-ng", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		emailEntry,
		passEntry,
		errorLabel,
		loginBtn,
	)

	return container.NewCenter(form)
}

func MakeMainScreen() fyne.CanvasObject {
	// --- Sidebar ---

	// Navigation items
	menuList := widget.NewList(
		func() int { return 4 },
		func() fyne.CanvasObject { return widget.NewLabel("Item") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			items := []string{"Home", "Profile", "History", "Rooms"}
			o.(*widget.Label).SetText(items[i])
		},
	)

	newRoomEntry := widget.NewEntry()
	newRoomEntry.SetPlaceHolder("New Room Name")

	joinBtn := widget.NewButton("Join", func() {
		if newRoomEntry.Text != "" {
			loadRoom(newRoomEntry.Text)
			newRoomEntry.SetText("")
		}
	})

	sidebar := container.NewBorder(
		widget.NewLabel("MENU"),
		container.NewVBox(newRoomEntry, joinBtn),
		nil, nil,
		menuList,
	)

	// --- Chat Area ---

	messagesBox = container.NewVBox()
	scrollBox = container.NewVScroll(messagesBox)

	msgInput := widget.NewMultiLineEntry()
	msgInput.SetPlaceHolder("Type your message here...")
	msgInput.Wrapping = fyne.TextWrapWord

	sendBtn := widget.NewButtonWithIcon("", theme.MailSendIcon(), func() {
		if msgInput.Text == "" {
			return
		}
		go func(txt string) {
			if err := Client.SendMessage(txt); err != nil {
				fmt.Println("Send Error:", err)
			}
		}(msgInput.Text)
		msgInput.SetText("")
	})

	inputBar := container.NewBorder(nil, nil, nil, sendBtn, msgInput)

	// Start with a welcome text
	messagesBox.Add(widget.NewLabel("Welcome. Join a room to start."))

	chatContent := container.NewBorder(
		container.NewPadded(widget.NewLabelWithStyle("Chat", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		container.NewPadded(inputBar),
		nil, nil,
		container.NewPadded(scrollBox),
	)

	// Split Container
	split := container.NewHSplit(sidebar, chatContent)
	split.SetOffset(0.25) // Sidebar takes 25%

	return split
}

func loadRoom(name string) {
	messagesBox.Objects = nil // Clear old messages
	messagesBox.Refresh()

	err := Client.JoinRoom(name)
	if err != nil {
		messagesBox.Add(widget.NewLabel("Error joining room: " + err.Error()))
		return
	}

	// Load existing messages
	for _, m := range Client.CurrentRoom.Messages {
		appendMessage(m)
	}

	// Connect WS
	Client.ConnectWS(Client.CurrentRoom.ID)
}

func appendMessage(m Message) {
	// Decrypt if necessary
	content := m.Message
	if m.HotSauce != "" {
		decrypted, err := DecryptMessage(m.Message, m.HotSauce, m.IV)
		if err == nil {
			content = decrypted
		} else {
			content = "[Decryption Error]"
		}
	}

	// Basic styling
	header := canvas.NewText(m.Email+" "+m.Time, color.RGBA{0, 0, 128, 255})
	header.TextSize = 10

	body := widget.NewLabel(content)
	body.Wrapping = fyne.TextWrapWord

	msgContainer := container.NewVBox(header, body)

	messagesBox.Add(msgContainer)
	scrollBox.ScrollToBottom()
}

func ListenForMessages() {
	for msg := range Client.MsgChan {
		appendMessage(msg)
	}
}
