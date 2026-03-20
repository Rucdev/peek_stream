package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	ErrRoomExists = errors.New("room already exists")
)

type ICEServer struct {
	URLs       interface{} `json:"urls"`
	Username   string      `json:"username,omitempty"`
	Credential string      `json:"credential,omitempty"`
}

type ICEConfig struct {
	ICEServers []ICEServer `json:"iceServers"`
}

func main() {
	hub := NewHub()

	http.HandleFunc("/api/rooms", func(w http.ResponseWriter, r *http.Request) {
		handleRoomList(w, r, hub)
	})

	http.HandleFunc("/api/ice-config", func(w http.ResponseWriter, r *http.Request) {
		handleICEConfig(w, r)
	})

	http.HandleFunc("/ws/camera/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimPrefix(r.URL.Path, "/ws/camera/")
		if roomID == "" {
			http.Error(w, "Room ID required", http.StatusBadRequest)
			return
		}
		hub.HandleCameraWS(w, r, roomID)
	})

	http.HandleFunc("/ws/watch/", func(w http.ResponseWriter, r *http.Request) {
		roomID := strings.TrimPrefix(r.URL.Path, "/ws/watch/")
		if roomID == "" {
			http.Error(w, "Room ID required", http.StatusBadRequest)
			return
		}
		hub.HandleWatcherWS(w, r, roomID)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleRoomList(w http.ResponseWriter, r *http.Request, hub *Hub) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rooms := hub.GetRoomList()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

func handleICEConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := ICEConfig{
		ICEServers: []ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
