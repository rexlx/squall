package main

import (
	"sync"

	"github.com/rexlx/squall/internal"
)

type Room struct {
	Stats       internal.AppStats  `json:"stats"`
	Messages    []internal.Message `json:"messages"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	MaxMessages int                `json:"max_messages"`
	Memory      *sync.RWMutex      `json:"-"`
}

func (rm *Room) GetRoomStats() internal.AppStats {
	rm.Memory.RLock()
	defer rm.Memory.RUnlock()
	return rm.Stats
}
