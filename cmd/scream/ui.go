package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/color"
	"io"
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
// THEME DEFINITION (Restored VFD Styling)
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
	docTabs = container.NewDocTabs()
	docTabs.OnClosed = func(item *container.TabItem) {
		roomName := item.Text
		Client.LeaveRoom(roomName)
		delete(openTabs, roomName)
		delete(roomBoxes, roomName)
		delete(roomScrolls, roomName)
	}

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

	addRoomEntry := widget.NewEntry()
	addRoomEntry.SetPlaceHolder("Room Name...")
	addRoomBtn := widget.NewButton("SAVE", func() {
		if addRoomEntry.Text != "" {
			Client.AddRoomToCache(addRoomEntry.Text)
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

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("SAVED ROOMS", savedSection),
		widget.NewAccordionItem("HISTORY", historyList),
	)
	accordion.Items[0].Open = true

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

	sidebarContent := container.NewBorder(
		joinHeader,
		loadKeysBtn,
		nil, nil,
		container.NewVScroll(accordion),
	)

	split := container.NewHSplit(sidebarContent, docTabs)
	split.SetOffset(0.25)

	return split
}

// ---------------------------------------------------------
// ROOM LOGIC
// ---------------------------------------------------------

func loadRoom(name string) {
	if item, ok := openTabs[name]; ok {
		docTabs.Select(item)
		return
	}

	go func() {
		if err := Client.JoinRoom(name); err != nil {
			fmt.Println("Error joining room:", err)
		}
	}()

	messagesBox := container.NewVBox()
	messagesBox.Add(widget.NewLabel(fmt.Sprintf("SYSTEM: Connected to %s", name)))

	scroll := container.NewVScroll(messagesBox)
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

	fileBtn := widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()

			data, _ := io.ReadAll(reader)
			hash := sha256.Sum256(data)
			hashStr := hex.EncodeToString(hash[:])

			// Register for 30 min timeout
			Client.ActiveOffers.Store(hashStr, PendingFile{
				Data:      data,
				FileName:  reader.URI().Name(),
				Timestamp: time.Now(),
			})

			Client.SendFileControl(name, hashStr, reader.URI().Name(), "OFFER")
		}, window)
		d.Show()
	})

	inputBar := container.NewBorder(nil, nil, nil, container.NewHBox(fileBtn, sendBtn), input)

	tabLayout := container.NewBorder(
		nil,
		container.NewPadded(inputBar),
		nil, nil,
		container.NewPadded(scroll),
	)

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
		m := msg
		fyne.Do(func() {
			roomName := m.RoomId
			box, ok := roomBoxes[roomName]
			if !ok {
				return
			}
			scroll := roomScrolls[roomName]

			if docTabs.Selected() != nil && docTabs.Selected().Text != roomName {
				if m.Email != Client.User.Email {
					fmt.Print("\a") // System Bell
				}
			}

			switch m.Type {
			case pb.ChatMessage_FILE_CONTROL:
				handleFileControl(m)
			case pb.ChatMessage_TEXT:
				renderTextMessage(m, box, scroll)
			case pb.ChatMessage_FILE_CHUNK:
				// Data reassembly logic would go here for recipients
			}
		})
	}
}

func renderTextMessage(m *pb.ChatMessage, box *fyne.Container, scroll *container.Scroll) {
	content := m.GetMessageContent()
	isEncrypted := false

	if m.HotSauce != "" {
		decrypted, err := DecryptMessage(content, m.HotSauce, m.Iv)
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

	cell := container.NewVBox(header, body)
	box.Add(cell)
	scroll.ScrollToBottom()
}

func handleFileControl(m *pb.ChatMessage) {
	meta := m.GetFileMeta()
	if meta == nil {
		return
	}

	// Recipient Side: OFFER received
	if meta.Action == "OFFER" && m.Email != Client.User.Email {
		dialog.ShowConfirm("Incoming File",
			fmt.Sprintf("%s wants to send: %s\nAccept?", m.Email, meta.FileName),
			func(ok bool) {
				if ok {
					Client.SendFileControl(m.RoomId, meta.FileHash, meta.FileName, "ACCEPT")
				}
			}, window)
	}

	// Sender Side: ACCEPT received
	if meta.Action == "ACCEPT" && m.Email != Client.User.Email {
		if val, ok := Client.ActiveOffers.Load(meta.FileHash); ok {
			pending := val.(PendingFile)
			go func() {
				_ = Client.SendFileChunks(m.RoomId, pending.Data)
				Client.ActiveOffers.Delete(meta.FileHash) // Immediate Purge
			}()
		}
	}
}
