package main

// Configuration
const API_BASE = "https://localhost:8080"
const WS_BASE = "wss://localhost:8080"

// Data Models

type User struct {
	ID        string   `json:"id,omitempty"`
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	About     string   `json:"about"`
	History   []string `json:"history"`
	Rooms     []string `json:"rooms"`
	Posts     []Post   `json:"posts"`
}

type Post struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	UserID  string `json:"user_id"`
}

type Room struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Messages []Message `json:"messages"`
}

type Message struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Message  string `json:"message"` // Content
	Time     string `json:"time"`
	RoomID   string `json:"room_id"`
	ReplyTo  string `json:"reply_to"`
	HotSauce string `json:"hotsauce,omitempty"` // Key name if encrypted
	IV       string `json:"iv,omitempty"`       // Base64 IV if encrypted
	Command  string `json:"command,omitempty"`
}

// API Payloads

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User    User   `json:"user"`
	Message string `json:"message"`
	Error   bool   `json:"error"`
	// The token is usually returned in headers or body;
	// assuming body based on JS usage of `data.token` or similar implies access
	// Note: JS uses `this.tk` from `/hotsauce` for some things and `this.key` for Auth header.
	// We will assume the Login endpoint returns the token or sets it.
	// Based on JS: app.key = apiKey.
}

type EncryptedData struct {
	KeyName string `json:"key"`
	Data    string `json:"data"` // Base64 ciphertext
	IV      string `json:"iv"`   // Base64 IV
}
