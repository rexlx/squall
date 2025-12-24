package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	pb "github.com/rexlx/squall/proto"
)

var (
	mainApp fyne.App
	window  fyne.Window

	// UI Elements for Chat
	messagesBox   *fyne.Container
	scrollBox     *container.Scroll
	channelHeader *widget.Label
)

// --- VFD Theme Definition ---
type vfdTheme struct{}

var _ fyne.Theme = (*vfdTheme)(nil)

func (v vfdTheme) Color(n fyne.ThemeColorName, v2 fyne.ThemeVariant) color.Color {
	cyan := color.RGBA{0, 240, 255, 255}
	dimCyan := color.RGBA{0, 100, 110, 255}
	darkBlue := color.RGBA{3, 5, 8, 255}
	inputBg := color.RGBA{20, 30, 40, 255}

	switch n {
	case theme.ColorNameForeground:
		return cyan
	case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
		return darkBlue
	case theme.ColorNameInputBackground:
		return inputBg
	case theme.ColorNameButton:
		return inputBg
	case theme.ColorNameShadow:
		return cyan
	case theme.ColorNamePrimary:
		return cyan
	case theme.ColorNameScrollBar:
		return dimCyan
	case theme.ColorNamePlaceHolder:
		return dimCyan
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (v vfdTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (v vfdTheme) Font(s fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(s)
}

func (v vfdTheme) Size(n fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(n)
}

// --- UI Construction ---

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

	title := canvas.NewText("SCREAM-NG", color.RGBA{0, 255, 255, 255})
	title.TextSize = 24
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	form := container.NewVBox(
		title,
		widget.NewSeparator(),
		emailEntry,
		passEntry,
		errorLabel,
		loginBtn,
	)

	return container.NewCenter(form)
}

func MakeMainScreen() fyne.CanvasObject {
	contentContainer := container.NewMax()

	// ---------------------------------------------------------
	// 1. Chat View Configuration
	// ---------------------------------------------------------
	messagesBox = container.NewVBox()
	messagesBox.Add(widget.NewLabel("SYSTEM: VFD Terminal Initialized."))
	scrollBox = container.NewVScroll(messagesBox)

	// Use our custom widget
	msgInput := NewSubmitEntry()
	msgInput.SetPlaceHolder("TRANSMIT MESSAGE... (Enter to Send)")

	// Define the shared sending logic
	doSend := func(content string) {
		if content == "" {
			return
		}

		// Send in background to keep UI responsive
		go func(t string) {
			if err := Client.SendMessage(t); err != nil {
				fmt.Println("Transmission Error:", err)
			}
		}(content)

		// Clear the input field immediately
		msgInput.SetText("")
	}

	// Link triggers
	msgInput.OnSubmit = doSend

	// Manual Send Button
	sendBtn := widget.NewButtonWithIcon("", theme.MailSendIcon(), func() {
		doSend(msgInput.Text)
	})

	inputBar := container.NewBorder(nil, nil, nil, sendBtn, msgInput)
	channelHeader = widget.NewLabelWithStyle(">> AWAITING SIGNAL", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	chatView := container.NewBorder(
		container.NewPadded(channelHeader),
		container.NewPadded(inputBar),
		nil, nil,
		container.NewPadded(scrollBox),
	)

	// ---------------------------------------------------------
	// 2. Helper Views (History & Rooms)
	// ---------------------------------------------------------

	// History View
	showHistory := func() {
		list := container.NewVBox()
		list.Add(widget.NewLabelWithStyle(">> ACCESS HISTORY", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
		list.Add(widget.NewSeparator())

		if len(Client.User.History) == 0 {
			list.Add(widget.NewLabel("NO HISTORY."))
		}

		// Reverse iterate: Show newest (last in list) at top
		for i := len(Client.User.History) - 1; i >= 0; i-- {
			rName := Client.User.History[i]
			btn := widget.NewButton(rName, func() {
				loadRoom(rName)
				contentContainer.Objects = []fyne.CanvasObject{chatView}
				contentContainer.Refresh()
			})
			btn.Alignment = widget.ButtonAlignLeading
			list.Add(btn)
		}

		historyView := container.NewPadded(container.NewVScroll(list))
		contentContainer.Objects = []fyne.CanvasObject{historyView}
		contentContainer.Refresh()
	}

	showRooms := func() {
		list := container.NewVBox()
		list.Add(widget.NewLabelWithStyle(">> SAVED ROOMS", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
		list.Add(widget.NewSeparator())

		// UI for Adding a new Room (Saved)
		addRoomEntry := widget.NewEntry()
		addRoomEntry.SetPlaceHolder("New Room Name...")

		// Button Action: Load room and switch view immediately
		addRoomBtn := widget.NewButton("ADD / SAVE", func() {
			if addRoomEntry.Text != "" {
				loadRoom(addRoomEntry.Text)
				addRoomEntry.SetText("")

				// Switch back to chat view
				contentContainer.Objects = []fyne.CanvasObject{chatView}
				contentContainer.Refresh()
			}
		})

		addBox := container.NewBorder(nil, nil, nil, addRoomBtn, addRoomEntry)
		list.Add(addBox)
		list.Add(widget.NewSeparator())

		// Populate List
		if len(Client.User.Rooms) == 0 {
			list.Add(widget.NewLabel("NO SAVED ROOMS."))
		}

		for _, roomName := range Client.User.Rooms {
			rName := roomName
			btn := widget.NewButton(rName, func() {
				loadRoom(rName)
				contentContainer.Objects = []fyne.CanvasObject{chatView}
				contentContainer.Refresh()
			})
			btn.Alignment = widget.ButtonAlignLeading
			list.Add(btn)
		}

		roomsView := container.NewPadded(container.NewVScroll(list))
		contentContainer.Objects = []fyne.CanvasObject{roomsView}
		contentContainer.Refresh()
	}

	// ---------------------------------------------------------
	// 3. Sidebar Navigation
	// ---------------------------------------------------------
	menuList := widget.NewList(
		func() int { return 4 },
		func() fyne.CanvasObject { return widget.NewLabel("Item") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			items := []string{"COMM", "PROFILE", "HISTORY", "ROOMS"}
			o.(*widget.Label).SetText(items[i])
		},
	)

	menuList.OnSelected = func(id widget.ListItemID) {
		switch id {
		case 0: // Comm
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		case 1: // Profile (Placeholder)
			contentContainer.Objects = []fyne.CanvasObject{widget.NewLabel("PROFILE WIP")}
			contentContainer.Refresh()
		case 2: // History
			showHistory()
		case 3: // Rooms
			showRooms()
		default:
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		}
	}

	// Sidebar Footer (Join Room directly)
	newRoomEntry := widget.NewEntry()
	newRoomEntry.SetPlaceHolder("CHANNEL ID")

	joinBtn := widget.NewButton("JOIN", func() {
		if newRoomEntry.Text != "" {
			loadRoom(newRoomEntry.Text)
			newRoomEntry.SetText("")
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		}
	})

	// Key Loading Button
	loadKeysBtn := widget.NewButton("LOAD KEY LIB", func() {
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			if reader == nil {
				return
			}
			defer reader.Close()

			if reader.URI().Extension() != ".json" {
				dialog.ShowInformation("Invalid Format", "Please select a .json key library.", window)
				return
			}

			if err := LoadKeys(reader); err != nil {
				dialog.ShowError(fmt.Errorf("failed to load keys: %w", err), window)
			} else {
				dialog.ShowInformation("Success", "Key library loaded successfully.", window)
			}
		}, window)

		d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
		d.Show()
	})

	sidebarInner := container.NewBorder(
		container.NewVBox(newRoomEntry, joinBtn, widget.NewSeparator()),
		nil, nil, nil,
		container.NewPadded(menuList),
	)

	sidebar := container.NewBorder(
		widget.NewLabel("MENU"),
		container.NewVBox(widget.NewSeparator(), loadKeysBtn),
		nil, nil,
		sidebarInner,
	)

	// ---------------------------------------------------------
	// 4. Final Layout
	// ---------------------------------------------------------
	// Default to chat view
	contentContainer.Objects = []fyne.CanvasObject{chatView}

	split := container.NewHSplit(sidebar, contentContainer)
	split.SetOffset(0.25)

	return split
}

func loadRoom(name string) {
	messagesBox.Objects = nil
	messagesBox.Refresh()

	if channelHeader != nil {
		channelHeader.SetText(">> CHANNEL: " + strings.ToUpper(name))
	}

	err := Client.JoinRoom(name)
	if err != nil {
		messagesBox.Add(widget.NewLabel("Error joining channel: " + err.Error()))
		return
	}

	if Client.CurrentRoom.History != nil {
		for _, m := range Client.CurrentRoom.History {
			appendMessage(m)
		}
	}
}

func appendMessage(m *pb.ChatMessage) {
	content := m.MessageContent
	isEncrypted := false

	if m.HotSauce != "" {
		decrypted, err := DecryptMessage(m.MessageContent, m.HotSauce, m.Iv)
		if err == nil {
			content = decrypted
			isEncrypted = true
		} else {
			content = "[ENCRYPTION ERROR]"
		}
	}

	timeStr := time.Unix(m.Timestamp, 0).Format("15:04:05")

	headerTxt := fmt.Sprintf("[%s] <%s>", timeStr, m.Email)
	if isEncrypted {
		headerTxt += " [SECURE]"
	}
	header := canvas.NewText(headerTxt, color.RGBA{0, 200, 255, 200})
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
