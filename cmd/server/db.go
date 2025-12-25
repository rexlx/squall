package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/rexlx/squall/internal"
)

// Database interface (unchanged)
type Database interface {
	GetMessage(roomid, messageid string) (internal.Message, error)
	StoreMessage(roomid string, message internal.Message) error
	GetUser(userid string) (User, error)
	StoreUser(user User) error
	GetRoom(roomid string) (Room, error)
	StoreRoom(room Room) error
	GetUserByEmail(email string) (User, error)
	PruneMessages(keep int) error
}

type PostgresDB struct {
	Conn *sql.DB
}

// NewPostgresDB creates a connection.
// connStr example: "postgres://user:password@localhost/dbname?sslmode=disable"
func NewPostgresDB(connStr string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresDB{Conn: db}, nil
}

func (db *PostgresDB) CreateTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password TEXT,
			name TEXT,
			role TEXT,  -- NEW COLUMN
			created TIMESTAMP,
			updated TIMESTAMP,
			rooms JSONB,
			history JSONB,
			stats JSONB,
			posts JSONB
);`,
		`CREATE TABLE IF NOT EXISTS rooms (
			id TEXT PRIMARY KEY,
			name TEXT,
			max_messages INT,
			stats JSONB
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			room_id TEXT NOT NULL,
			user_id TEXT,
			email TEXT,
			msg_content TEXT, 
			time_str TEXT, 
			reply_to TEXT,
			iv TEXT,
			hot_sauce TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		);`,
		// NEW: Index for performance
		`CREATE INDEX IF NOT EXISTS idx_messages_room_id ON messages(room_id);`,
	}

	for _, q := range queries {
		if _, err := db.Conn.Exec(q); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	return nil
}

// GetMessage retrieves a specific message.
// Note: Since internal.Message doesn't have an ID field, we query by the DB ID passed as messageid
// but we cannot return that ID in the struct.
func (db *PostgresDB) GetMessage(roomid, messageid string) (internal.Message, error) {
	query := `SELECT room_id, user_id, email, msg_content, time_str, reply_to, iv, hot_sauce 
	          FROM messages WHERE room_id = $1 AND id = $2`

	row := db.Conn.QueryRow(query, roomid, messageid)

	var m internal.Message
	err := row.Scan(&m.RoomID, &m.UserID, &m.Email, &m.Message, &m.Time, &m.ReplyTo, &m.InitialVector, &m.HotSauce)
	if err != nil {
		return internal.Message{}, err
	}
	return m, nil
}

// StoreMessage performs a fast INSERT only.
// Corrected to use specific fields from your internal.Message struct.
func (db *PostgresDB) StoreMessage(roomid string, m internal.Message) error {
	// 1. Insert the new message
	// Note: internal.Message.Time is a string, matching your DB schema (time_str)
	query := `INSERT INTO messages (room_id, user_id, email, msg_content, time_str, reply_to, iv, hot_sauce)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := db.Conn.Exec(query, roomid, m.UserID, m.Email, m.Message, m.Time, m.ReplyTo, m.InitialVector, m.HotSauce)
	return err
}

// PruneMessages deletes old messages from all rooms in the background.
// This replaces the inline DELETE subquery that was killing performance.
func (db *PostgresDB) PruneMessages(keep int) error {
	// 1. Get all unique room IDs
	rows, err := db.Conn.Query(`SELECT DISTINCT room_id FROM messages`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var rooms []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err == nil {
			rooms = append(rooms, r)
		}
	}

	// 2. Delete messages exceeding the limit for each room
	// We use the same logic as before, but decoupled from the send loop.
	query := `DELETE FROM messages 
	          WHERE room_id = $1 AND id NOT IN (
	              SELECT id FROM messages 
	              WHERE room_id = $1 
	              ORDER BY id DESC 
	              LIMIT $2
	          )`

	for _, room := range rooms {
		if _, err := db.Conn.Exec(query, room, keep); err != nil {
			log.Printf("Error pruning room %s: %v", room, err)
		}
	}
	return nil
}

func (db *PostgresDB) GetUser(userid string) (User, error) {
	query := `SELECT id, email, password, name, role, created, updated, rooms, history, stats, posts FROM users WHERE id = $1`
	row := db.Conn.QueryRow(query, userid)

	var u User
	var roomsJSON, historyJSON, statsJSON, postsJSON []byte

	err := row.Scan(&u.ID, &u.Email, &u.Password, &u.Name, &u.Role, &u.Created, &u.Updated, &roomsJSON, &historyJSON, &statsJSON, &postsJSON)
	if err != nil {
		return User{}, err
	}

	// Unmarshal JSON fields back into structs
	_ = json.Unmarshal(roomsJSON, &u.Rooms)
	_ = json.Unmarshal(historyJSON, &u.History)
	_ = json.Unmarshal(statsJSON, &u.Stats)
	_ = json.Unmarshal(postsJSON, &u.Posts)

	return u, nil
}

func (db *PostgresDB) StoreUser(u User) error {
	// Marshal complex fields to JSON
	roomsJSON, _ := json.Marshal(u.Rooms)
	historyJSON, _ := json.Marshal(u.History)
	statsJSON, _ := json.Marshal(u.Stats)
	postsJSON, _ := json.Marshal(u.Posts)

	query := `INSERT INTO users (id, email, password, name, role, created, updated, rooms, history, stats, posts)
          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
          ON CONFLICT (id) DO UPDATE SET
          email = EXCLUDED.email,
          password = EXCLUDED.password,
          name = EXCLUDED.name,
          role = EXCLUDED.role,
          updated = EXCLUDED.updated,
          rooms = EXCLUDED.rooms,
          history = EXCLUDED.history,
          stats = EXCLUDED.stats,
          posts = EXCLUDED.posts;`

	_, err := db.Conn.Exec(query, u.ID, u.Email, u.Password, u.Name, u.Role, u.Created, time.Now(), roomsJSON, historyJSON, statsJSON, postsJSON)
	return err
}

func (db *PostgresDB) GetRoom(roomid string) (Room, error) {
	// 1. Get Room Metadata
	query := `SELECT id, name, max_messages, stats FROM rooms WHERE id = $1`
	row := db.Conn.QueryRow(query, roomid)

	var r Room
	var statsJSON []byte

	err := row.Scan(&r.ID, &r.Name, &r.MaxMessages, &statsJSON)
	if err != nil {
		return Room{}, err
	}
	_ = json.Unmarshal(statsJSON, &r.Stats)

	// 2. Hydrate Messages (Fetch last N messages)
	// Note: We populate the Messages slice so the Room struct is complete for the application
	msgQuery := `SELECT room_id, user_id, email, msg_content, time_str, reply_to, iv, hot_sauce 
	             FROM messages WHERE room_id = $1 ORDER BY id DESC LIMIT 50`

	rows, err := db.Conn.Query(msgQuery, roomid)
	if err != nil {
		// We don't fail getting the room if messages fail, just log it or return empty
		log.Printf("Warning: failed to fetch messages for room %s: %v", roomid, err)
	} else {
		defer rows.Close()
		var msgs []internal.Message
		for rows.Next() {
			var m internal.Message
			if err := rows.Scan(&m.RoomID, &m.UserID, &m.Email, &m.Message, &m.Time, &m.ReplyTo, &m.InitialVector, &m.HotSauce); err == nil {
				// Prepend to maintain chronological order if using DESC?
				// Or simply append and let the client handle sort.
				// Usually chat needs oldest->newest.
				msgs = append([]internal.Message{m}, msgs...)
			}
		}
		r.Messages = msgs
	}

	return r, nil
}

func (db *PostgresDB) StoreRoom(r Room) error {
	statsJSON, _ := json.Marshal(r.Stats)

	// We do NOT store r.Messages here because they are stored individually via StoreMessage.
	// We only store the room metadata.
	query := `INSERT INTO rooms (id, name, max_messages, stats)
	          VALUES ($1, $2, $3, $4)
	          ON CONFLICT (id) DO UPDATE SET
	          name = EXCLUDED.name,
	          max_messages = EXCLUDED.max_messages,
	          stats = EXCLUDED.stats;`

	_, err := db.Conn.Exec(query, r.ID, r.Name, r.MaxMessages, statsJSON)
	return err
}

func (db *PostgresDB) GetUserByEmail(email string) (User, error) {
	// Select the ID first, or the whole row
	query := `SELECT id FROM users WHERE email = $1`
	row := db.Conn.QueryRow(query, email)

	var id string
	if err := row.Scan(&id); err != nil {
		return User{}, err
	}

	// Reuse existing GetUser
	return db.GetUser(id)
}
