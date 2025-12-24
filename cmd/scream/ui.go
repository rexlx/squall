package main

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	mainApp fyne.App
	window  fyne.Window

	// Global UI State
	docTabs     *container.DocTabs
	openTabs    map[string]*container.TabItem
	roomBoxes   map[string]*fyne.Container   // Message area for each room
	roomScrolls map[string]*container.Scroll // Scroll container for each room
)

func init() {
	openTabs = make(map[string]*container.TabItem)
	roomBoxes = make(map[string]*fyne.Container)
	roomScrolls = make(map[string]*container.Scroll)
}

// ---------------------------------------------------------
// THEME DEFINITION
// ---------------------------------------------------------

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

func (v vfdTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (v vfdTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (v vfdTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }

// ---------------------------------------------------------
// LOGIN SCREEN
// ---------------------------------------------------------

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

	// SPACER FIX: Forces the login form to be at least 300px wide
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(300, 0))

	form := container.NewVBox(
		title,
		widget.NewSeparator(),
		emailEntry,
		passEntry,
		errorLabel,
		loginBtn,
		spacer,
	)

	return container.NewCenter(form)
}

// ---------------------------------------------------------
// MAIN APPLICATION SCREEN
// ---------------------------------------------------------

func MakeMainScreen() fyne.CanvasObject {
	// 1. Initialize the Tab Container
	docTabs = container.NewDocTabs()
	docTabs.OnClosed = func(item *container.TabItem) {
		roomName := item.Text

		// Close Stream
		Client.LeaveRoom(roomName)

		// Cleanup UI Maps
		delete(openTabs, roomName)
		delete(roomBoxes, roomName)
		delete(roomScrolls, roomName)
	}

	// 2. Sidebar: Saved Rooms & History
	// We use functions to refresh these lists dynamically.

	// --- SAVED ROOMS ---
	savedRoomsList := container.NewVBox()
	refreshSavedRooms := func() {
		savedRoomsList.Objects = nil
		if len(Client.User.Rooms) == 0 {
			savedRoomsList.Add(widget.NewLabel("No saved rooms."))
		}
		for _, r := range Client.User.Rooms {
			rName := r
			btn := widget.NewButton(rName, func() { loadRoom(rName) })
			btn.Alignment = widget.ButtonAlignLeading
			savedRoomsList.Add(btn)
		}
		savedRoomsList.Refresh()
	}
	refreshSavedRooms()

	// "Add to Saved" Logic
	addRoomEntry := widget.NewEntry()
	addRoomEntry.SetPlaceHolder("Room Name...")
	addRoomBtn := widget.NewButton("SAVE", func() {
		if addRoomEntry.Text != "" {
			// Inline "Add to Cache" logic to avoid Client dependency issues
			roomName := addRoomEntry.Text
			exists := false
			for _, r := range Client.User.Rooms {
				if r == roomName {
					exists = true
					break
				}
			}
			if !exists {
				Client.User.Rooms = append(Client.User.Rooms, roomName)
			}
			// Refresh UI
			addRoomEntry.SetText("")
			refreshSavedRooms()
		}
	})

	savedSection := container.NewVBox(
		container.NewBorder(nil, nil, nil, addRoomBtn, addRoomEntry),
		savedRoomsList,
	)

	// --- HISTORY ---
	historyList := container.NewVBox()
	refreshHistory := func() {
		historyList.Objects = nil
		// Iterate backwards to show newest first
		for i := len(Client.User.History) - 1; i >= 0; i-- {
			rName := Client.User.History[i]
			btn := widget.NewButton(rName, func() { loadRoom(rName) })
			btn.Alignment = widget.ButtonAlignLeading
			historyList.Add(btn)
		}
		if len(Client.User.History) == 0 {
			historyList.Add(widget.NewLabel("No history."))
		}
		historyList.Refresh()
	}
	refreshHistory()

	// --- SIDEBAR ACCORDION ---
	accordion := widget.NewAccordion(
		widget.NewAccordionItem("SAVED ROOMS", savedSection),
		widget.NewAccordionItem("HISTORY", historyList),
	)
	accordion.Items[0].Open = true // Open Saved by default

	// --- SIDEBAR HEADER (Join) ---
	newRoomEntry := widget.NewEntry()
	newRoomEntry.SetPlaceHolder("CHANNEL ID")
	joinBtn := widget.NewButton("JOIN", func() {
		if newRoomEntry.Text != "" {
			loadRoom(newRoomEntry.Text)
			newRoomEntry.SetText("")
		}
	})
	joinHeader := container.NewVBox(
		widget.NewLabelWithStyle("QUICK JOIN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		newRoomEntry,
		joinBtn,
		widget.NewSeparator(),
	)

	// --- SIDEBAR FOOTER (Load Keys) ---
	loadKeysBtn := widget.NewButton("LOAD KEY LIB", func() {
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			LoadKeys(reader)
		}, window)
		d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
		d.Show()
	})

	// Combine Sidebar
	sidebarContent := container.NewBorder(
		joinHeader,
		loadKeysBtn,
		nil, nil,
		container.NewVScroll(accordion),
	)

	// 3. Final Layout
	split := container.NewHSplit(sidebarContent, docTabs)
	split.SetOffset(0.25)

	return split
}

// ---------------------------------------------------------
// ROOM LOGIC
// ---------------------------------------------------------

func loadRoom(name string) {
	// 1. If tab exists, focus it
	if item, ok := openTabs[name]; ok {
		docTabs.Select(item)
		return
	}

	// 2. Start Stream in Background
	go func() {
		if err := Client.JoinRoom(name); err != nil {
			fmt.Println("Error joining room:", err)
		}
	}()

	// 3. Build UI Components
	messagesBox := container.NewVBox()
	messagesBox.Add(widget.NewLabel(fmt.Sprintf("SYSTEM: Connected to %s", name)))

	scroll := container.NewVScroll(messagesBox)

	// Input Field (using custom SubmitEntry)
	input := NewSubmitEntry()
	input.SetPlaceHolder(fmt.Sprintf("Message %s...", name))

	doSend := func(txt string) {
		if txt == "" {
			return
		}
		go func(t string) {
			Client.SendMessage(name, t)
		}(txt)
		input.SetText("")
	}

	input.OnSubmit = doSend
	sendBtn := widget.NewButtonWithIcon("", theme.MailSendIcon(), func() { doSend(input.Text) })

	inputBar := container.NewBorder(nil, nil, nil, sendBtn, input)

	tabLayout := container.NewBorder(
		nil,
		container.NewPadded(inputBar),
		nil, nil,
		container.NewPadded(scroll),
	)

	// 4. Create and Register Tab
	tabItem := container.NewTabItem(name, tabLayout)
	docTabs.Append(tabItem)
	docTabs.Select(tabItem)

	openTabs[name] = tabItem
	roomBoxes[name] = messagesBox
	roomScrolls[name] = scroll
}

// ---------------------------------------------------------
// MESSAGE HANDLING (Thread-Safe)
// ---------------------------------------------------------

func ListenForMessages() {
	for msg := range Client.MsgChan {
		// Capture variable for closure
		m := msg

		// FIX: Use fyne.Do() instead of RunOnMainThread
		fyne.Do(func() {
			roomName := m.RoomId

			// 1. Locate the UI Box for this room
			box, ok := roomBoxes[roomName]
			if !ok {
				// We received a message for a room we closed or haven't opened yet
				return
			}
			scroll := roomScrolls[roomName]

			// 2. Audible Chime Logic
			// Check if this room is NOT the currently focused tab
			if docTabs.Selected() != nil && docTabs.Selected().Text != roomName {
				// Don't chime for our own messages
				if m.Email != Client.User.Email {
					fmt.Print("\a") // System Bell
				}
			}

			// 3. Decrypt Content
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

			// 4. Construct UI Element
			timeStr := time.Unix(m.Timestamp, 0).Format("15:04:05")
			headerTxt := fmt.Sprintf("[%s] <%s>", timeStr, m.Email)
			if isEncrypted {
				headerTxt += " [SECURE]"
			}

			header := canvas.NewText(headerTxt, color.RGBA{0, 200, 255, 200})
			header.TextSize = 10

			body := widget.NewLabel(content)
			body.Wrapping = fyne.TextWrapWord

			cell := container.NewVBox(header, body)

			// 5. Append and Scroll
			box.Add(cell)
			scroll.ScrollToBottom()
		})
	}
}
