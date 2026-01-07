package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rexlx/squall/internal"
)

var (
	Whitelist   = make(map[string]bool)
	WhitelistMu sync.RWMutex
)

type Server struct {
	Queue     chan SaveRequest  `json:"-"`
	Rooms     map[string]*Room  `json:"rooms"`
	Address   string            `json:"address"`
	ID        string            `json:"id"`
	ValidKeys internal.KeyLib   `json:"valid_keys"`
	Key       string            `json:"key"`
	Stats     internal.AppStats `json:"stats"`
	StartTime time.Time         `json:"start_time"`
	Memory    *sync.RWMutex     `json:"-"`
	Logger    *log.Logger       `json:"-"`
	Gateway   *http.ServeMux    `json:"-"`
	DB        Database          `json:"-"`
}

type SaveRequest struct {
	RoomID  string
	Message internal.Message
}

func NewServer(address, key string, logger *log.Logger, db Database) *Server {
	start := time.Now()
	sQ := make(chan SaveRequest, 100)
	svr := &Server{
		Queue:     sQ,
		Rooms:     make(map[string]*Room),
		Address:   address,
		ID:        "server-001",
		ValidKeys: make(internal.KeyLib),
		Key:       key,
		Stats:     make(internal.AppStats),
		StartTime: start,
		Memory:    &sync.RWMutex{},
		Logger:    logger,
		Gateway:   http.NewServeMux(),
		DB:        db,
	}
	svr.ValidKeys["undefined"] = internal.Key{
		Value:       "undefined",
		Expires:     start.Add(24 * time.Hour),
		Issued:      start,
		RequestedBy: "system",
	}
	svr.Gateway.HandleFunc("/login", svr.LoginHandler)
	return svr
}

func (s *Server) StartSaveWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case req := <-s.Queue:
			if err := s.DB.StoreMessage(req.RoomID, req.Message); err != nil {
				s.Logger.Println("Error saving message to DB:", err)
			}
		case <-ticker.C:
			s.Logger.Println("Save Worker Heartbeat - Queue Length:", len(s.Queue))
		}
	}
}

func (s *Server) StartPruneWorker(interval time.Duration, keep int) {
	if interval <= 0 {
		s.Logger.Println("Pruning disabled (interval 0)")
		return
	}
	s.Logger.Printf("Prune worker started (Every %s, keep %d)", interval, keep)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		start := time.Now()
		s.Logger.Println("Starting Prune...")
		if err := s.DB.PruneMessages(keep); err != nil {
			s.Logger.Printf("Prune failed: %v", err)
		} else {
			s.Logger.Printf("Prune finished in %v", time.Since(start))
		}
	}
}

func (s *Server) StartRoomReaper(checkInterval time.Duration, staleThreshold time.Duration) {
	s.Logger.Printf("Room Reaper started (Check every %s, stale threshold %s)", checkInterval, staleThreshold)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		start := time.Now()
		s.Logger.Println("Room Reaper: Checking for stale rooms...")

		if err := s.DB.ReapStaleRooms(staleThreshold); err != nil {
			s.Logger.Printf("Room Reaper failed: %v", err)
		} else {
			s.Logger.Printf("Room Reaper finished in %v", time.Since(start))
		}
	}
}
