package main

import (
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
)

type ClientRole int

const (
	RoleCamera ClientRole = iota
	RoleWatcher
)

type Client struct {
	ID   string
	Role ClientRole
	Conn *websocket.Conn
	Send chan []byte
}

type Room struct {
	ID           string
	PasswordHash string
	Camera       *Client
	Watchers     map[string]*Client
	mu           sync.RWMutex
}

type Hub struct {
	Rooms map[string]*Room
	mu    sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Rooms: make(map[string]*Room),
	}
}

func (h *Hub) CreateRoom(roomID, password string, camera *Client) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.Rooms[roomID]; exists {
		return ErrRoomExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	h.Rooms[roomID] = &Room{
		ID:           roomID,
		PasswordHash: string(hash),
		Camera:       camera,
		Watchers:     make(map[string]*Client),
	}

	return nil
}

func (h *Hub) GetRoom(roomID string) (*Room, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, exists := h.Rooms[roomID]
	return room, exists
}

func (h *Hub) DeleteRoom(roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.Rooms, roomID)
}

func (h *Hub) GetRoomList() []RoomInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	list := make([]RoomInfo, 0, len(h.Rooms))
	for _, room := range h.Rooms {
		room.mu.RLock()
		list = append(list, RoomInfo{
			ID:           room.ID,
			WatcherCount: len(room.Watchers),
		})
		room.mu.RUnlock()
	}
	return list
}

func (r *Room) VerifyPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(r.PasswordHash), []byte(password))
	return err == nil
}

func (r *Room) AddWatcher(watcher *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Watchers[watcher.ID] = watcher
}

func (r *Room) RemoveWatcher(watcherID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Watchers, watcherID)
}

func (r *Room) GetWatcher(watcherID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	watcher, exists := r.Watchers[watcherID]
	return watcher, exists
}

func (r *Room) GetWatcherCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.Watchers)
}

type RoomInfo struct {
	ID           string `json:"id"`
	WatcherCount int    `json:"watcherCount"`
}
