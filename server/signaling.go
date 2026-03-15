package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

type Message struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	PeerID    string          `json:"peerId,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func (h *Hub) HandleCameraWS(w http.ResponseWriter, r *http.Request, roomID string) {
	password := r.URL.Query().Get("pass")
	if password == "" {
		http.Error(w, "Password required", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Camera upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:   uuid.New().String(),
		Role: RoleCamera,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	if err := h.CreateRoom(roomID, password, client); err != nil {
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "Room already exists"))
		conn.Close()
		return
	}

	go h.writePump(client, roomID)
	h.readPumpCamera(client, roomID)
}

func (h *Hub) HandleWatcherWS(w http.ResponseWriter, r *http.Request, roomID string) {
	password := r.URL.Query().Get("pass")
	if password == "" {
		http.Error(w, "Password required", http.StatusUnauthorized)
		return
	}

	room, exists := h.GetRoom(roomID)
	if !exists {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	if !room.VerifyPassword(password) {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Watcher upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:   uuid.New().String(),
		Role: RoleWatcher,
		Conn: conn,
		Send: make(chan []byte, 256),
	}

	room.AddWatcher(client)

	// Notify camera about new watcher
	if room.Camera != nil {
		msg := Message{Type: "watcher_joined", PeerID: client.ID}
		if data, err := json.Marshal(msg); err == nil {
			select {
			case room.Camera.Send <- data:
			default:
				log.Printf("Camera send buffer full")
			}
		}
	}

	go h.writePump(client, roomID)
	h.readPumpWatcher(client, roomID)
}

func (h *Hub) readPumpCamera(client *Client, roomID string) {
	defer func() {
		h.cleanupCamera(client, roomID)
	}()

	client.Conn.SetReadDeadline(time.Now().Add(pongWait))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	client.Conn.SetReadLimit(maxMessageSize)

	for {
		_, data, err := client.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		h.routeCameraMessage(client, roomID, &msg)
	}
}

func (h *Hub) readPumpWatcher(client *Client, roomID string) {
	defer func() {
		h.cleanupWatcher(client, roomID)
	}()

	client.Conn.SetReadDeadline(time.Now().Add(pongWait))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	client.Conn.SetReadLimit(maxMessageSize)

	for {
		_, data, err := client.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		h.routeWatcherMessage(client, roomID, &msg)
	}
}

func (h *Hub) routeCameraMessage(client *Client, roomID string, msg *Message) {
	room, exists := h.GetRoom(roomID)
	if !exists {
		return
	}

	switch msg.Type {
	case "offer", "candidate":
		if msg.PeerID == "" {
			return
		}
		watcher, exists := room.GetWatcher(msg.PeerID)
		if !exists {
			return
		}
		if data, err := json.Marshal(msg); err == nil {
			select {
			case watcher.Send <- data:
			default:
			}
		}
	}
}

func (h *Hub) routeWatcherMessage(client *Client, roomID string, msg *Message) {
	room, exists := h.GetRoom(roomID)
	if !exists || room.Camera == nil {
		return
	}

	switch msg.Type {
	case "answer", "candidate":
		msg.PeerID = client.ID
		if data, err := json.Marshal(msg); err == nil {
			select {
			case room.Camera.Send <- data:
			default:
			}
		}
	}
}

func (h *Hub) writePump(client *Client, roomID string) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) cleanupCamera(client *Client, roomID string) {
	room, exists := h.GetRoom(roomID)
	if exists {
		room.mu.Lock()
		for _, watcher := range room.Watchers {
			close(watcher.Send)
		}
		room.mu.Unlock()
	}
	h.DeleteRoom(roomID)
	close(client.Send)
	client.Conn.Close()
	log.Printf("Camera disconnected from room %s", roomID)
}

func (h *Hub) cleanupWatcher(client *Client, roomID string) {
	room, exists := h.GetRoom(roomID)
	if exists {
		room.RemoveWatcher(client.ID)
		if room.Camera != nil {
			msg := Message{Type: "watcher_left", PeerID: client.ID}
			if data, err := json.Marshal(msg); err == nil {
				select {
				case room.Camera.Send <- data:
				default:
				}
			}
		}
	}
	close(client.Send)
	client.Conn.Close()
	log.Printf("Watcher %s disconnected from room %s", client.ID, roomID)
}
