package websocket

import (
	"encoding/json"
	"github.com/juliazadorozhnaya/OnlineTextEditor/file"
	"time"
)

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	usernames  map[string]int
	f          *file.File
	startTime  time.Time
	fontSize   int
}

func NewHub(f *file.File) *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		usernames:  make(map[string]int),
		f:          f,
		startTime:  time.Now(), // Initialize start time
		fontSize:   16,         // Initialize font size
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.usernames[client.username]++
			h.UpdateUsers()
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.usernames[client.username]--
				if h.usernames[client.username] == 0 {
					delete(h.usernames, client.username)
				}
				h.UpdateUsers()
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
					h.usernames[client.username]--
					if h.usernames[client.username] == 0 {
						delete(h.usernames, client.username)
					}
					h.UpdateUsers()
				}
			}

			// Write the received message to the file
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if msgType, ok := msg["type"].(string); ok {
					if msgType == "text" {
						if text, ok := msg["text"].(string); ok {
							h.f.Write([]byte(text))
						}
					} else if msgType == "fontSize" {
						if fontSize, ok := msg["fontSize"].(float64); ok {
							h.fontSize = int(fontSize)
						}
					}
				}
			}
		}
	}
}

func (h *Hub) UpdateUsers() {
	users := []string{}
	for username := range h.usernames {
		users = append(users, username)
	}
	data := map[string]interface{}{
		"type":  "users",
		"users": users,
	}
	message, _ := json.Marshal(data)
	for client := range h.clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}
