package main

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	pb "github.com/rexlx/squall/proto" // Import generated proto
)

var (
	mainApp fyne.App
	window  fyne.Window

	// UI Elements for Chat
	messagesBox *fyne.Container
	scrollBox   *container.Scroll
)

// MakeLoginScreen remains largely the same, assuming Client.Login signature didn't change.
func MakeLoginScreen(onSuccess func()) fyne.CanvasObject {
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("Username/Email")

	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Password")

	errorLabel := widget.NewLabel("")
	errorLabel.Hide()

	loginBtn := widget.NewButton("Login", func() {
		// Ensure Client.Login returns error on failure
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
	// --- Container for Dynamic Content (Right Side) ---
	// We use a Max container so we can easily swap between Chat and History views
	contentContainer := container.NewMax()

	// --- 1. Construct the Chat View ---

	messagesBox = container.NewVBox()
	messagesBox.Add(widget.NewLabel("Welcome. Join a room to start."))
	scrollBox = container.NewVScroll(messagesBox)

	msgInput := widget.NewMultiLineEntry()
	msgInput.SetPlaceHolder("Type your message here...")
	msgInput.Wrapping = fyne.TextWrapWord

	sendBtn := widget.NewButtonWithIcon("", theme.MailSendIcon(), func() {
		if msgInput.Text == "" {
			return
		}
		txt := msgInput.Text
		go func(t string) {
			if err := Client.SendMessage(t); err != nil {
				fmt.Println("Send Error:", err)
			}
		}(txt)
		msgInput.SetText("")
	})

	inputBar := container.NewBorder(nil, nil, nil, sendBtn, msgInput)

	chatView := container.NewBorder(
		container.NewPadded(widget.NewLabelWithStyle("Chat", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		container.NewPadded(inputBar),
		nil, nil,
		container.NewPadded(scrollBox),
	)

	// --- 2. Construct the History View Logic ---

	showHistory := func() {
		list := container.NewVBox()
		list.Add(widget.NewLabelWithStyle("Room History", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
		list.Add(widget.NewSeparator())

		if len(Client.User.History) == 0 {
			list.Add(widget.NewLabel("No history found."))
		}

		// Iterate through history and create a button for each room
		for _, roomName := range Client.User.History {
			// Capture the variable for the closure
			rName := roomName
			btn := widget.NewButton(rName, func() {
				// 1. Load the room logic (API call + connection)
				loadRoom(rName)

				// 2. Switch UI back to Chat View
				contentContainer.Objects = []fyne.CanvasObject{chatView}
				contentContainer.Refresh()
			})
			btn.Alignment = widget.ButtonAlignLeading
			list.Add(btn)
		}

		// Wrap the list in a scroll container in case history is long
		historyView := container.NewPadded(container.NewVScroll(list))

		// Swap the content
		contentContainer.Objects = []fyne.CanvasObject{historyView}
		contentContainer.Refresh()
	}

	// --- 3. Construct the Sidebar ---

	// Navigation Menu
	menuList := widget.NewList(
		func() int { return 4 },
		func() fyne.CanvasObject { return widget.NewLabel("Item") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			items := []string{"Chat", "Profile", "History", "Rooms"}
			o.(*widget.Label).SetText(items[i])
		},
	)

	// Handle Menu Selection
	menuList.OnSelected = func(id widget.ListItemID) {
		switch id {
		case 0: // Chat
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		case 2: // History
			showHistory()
		default:
			// For now, default back to chat or show a placeholder
			// specific handlers for Profile/Rooms can be added here later
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		}
	}

	// Room Joiner in Sidebar
	newRoomEntry := widget.NewEntry()
	newRoomEntry.SetPlaceHolder("New Room Name")

	joinBtn := widget.NewButton("Join", func() {
		if newRoomEntry.Text != "" {
			loadRoom(newRoomEntry.Text)
			newRoomEntry.SetText("")
			// Ensure we are looking at the chat view when joining
			contentContainer.Objects = []fyne.CanvasObject{chatView}
			contentContainer.Refresh()
		}
	})

	sidebar := container.NewBorder(
		widget.NewLabel("MENU"),
		container.NewVBox(newRoomEntry, joinBtn),
		nil, nil,
		menuList,
	)

	// --- 4. Final Layout ---

	// Initialize with Chat View
	contentContainer.Objects = []fyne.CanvasObject{chatView}

	// Split Container (Sidebar | Content)
	split := container.NewHSplit(sidebar, contentContainer)
	split.SetOffset(0.25) // Sidebar takes 25%

	return split
}

func loadRoom(name string) {
	messagesBox.Objects = nil // Clear old messages
	messagesBox.Refresh()

	// Client.JoinRoom now returns error, and sets Client.CurrentRoom (*pb.RoomResponse)
	err := Client.JoinRoom(name)
	if err != nil {
		messagesBox.Add(widget.NewLabel("Error joining room: " + err.Error()))
		return
	}

	// Assuming RoomResponse includes a History field (repeated ChatMessage)
	if Client.CurrentRoom.History != nil {
		for _, m := range Client.CurrentRoom.History {
			appendMessage(m)
		}
	}

	// Stream is started inside Client.JoinRoom or Client.StartStream now
}

// Update input type to *pb.ChatMessage
func appendMessage(m *pb.ChatMessage) {
	content := m.MessageContent

	// Decrypt if HotSauce (KeyName) is present
	if m.HotSauce != "" {
		decrypted, err := DecryptMessage(m.MessageContent, m.HotSauce, m.Iv)
		if err == nil {
			content = decrypted
		} else {
			content = "[Decryption Error]"
		}
	}

	// Convert Timestamp (int64) to readable string
	timeStr := time.Unix(m.Timestamp, 0).Format("15:04:05")

	// Styling
	header := canvas.NewText(m.Email+" "+timeStr, color.RGBA{0, 0, 128, 255})
	header.TextSize = 10

	body := widget.NewLabel(content)
	body.Wrapping = fyne.TextWrapWord

	msgContainer := container.NewVBox(header, body)

	messagesBox.Add(msgContainer)
	scrollBox.ScrollToBottom()
}

func ListenForMessages() {
	// Client.MsgChan is now chan *pb.ChatMessage
	for msg := range Client.MsgChan {
		appendMessage(msg)
	}
}
