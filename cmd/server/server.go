package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rexlx/squall/internal"
)

type Server struct {
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

func NewServer(address, key string, logger *log.Logger, db Database) *Server {
	start := time.Now()
	svr := &Server{
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
