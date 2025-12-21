package internal

import "time"

type Stat struct {
	Time  time.Time `json:"time"`
	Value float64   `json:"value"`
}

type Post struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type AppStats map[string][]Stat
type KeyLib map[string]Key

type Message struct {
	RoomID        string `json:"room_id"`
	Time          string `json:"time"`
	ReplyTo       string `json:"reply_to"`
	Message       string `json:"message"`
	UserID        string `json:"user_id"`
	Email         string `json:"email"`
	InitialVector string `json:"iv"`
	HotSauce      string `json:"hot_sauce"`
}

type Key struct {
	Value       string    `json:"value"`
	Expires     time.Time `json:"expires"`
	Issued      time.Time `json:"issued"`
	RequestedBy string    `json:"requested_by"`
}
