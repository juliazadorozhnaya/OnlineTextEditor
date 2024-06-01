package websocket

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/juliazadorozhnaya/OnlineTextEditor/file"
	"log"
	"net/http"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func ServeWs(h *Hub, f *file.File, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	sessionID := r.URL.Query().Get("session_id") // Предполагаем, что session_id передаётся в запросе
	if sessionID == "" {
		sessionID = uuid.NewString() // Генерируем новый, если не был передан
	}

	username := r.Context().Value("username").(string)
	client := &Client{
		conn:      conn,
		send:      make(chan []byte, 256),
		hub:       h,
		f:         f,
		username:  username,
		sessionId: sessionID, // Используем sessionID
	}
	client.hub.register <- client

	// Отправка текущего текста и времени подключения новому клиенту
	text, err := f.Read()
	if err == nil {
		client.send <- []byte(fmt.Sprintf(`{"type": "text", "text": %q, "session_id": %q}`, text, sessionID))
	}
	startTimeMessage := fmt.Sprintf(`{"type": "startTime", "startTime": %d}`, h.startTime.Unix())
	client.send <- []byte(startTimeMessage)
	fontSizeMessage := fmt.Sprintf(`{"type": "fontSize", "fontSize": %d}`, h.fontSize)
	client.send <- []byte(fontSizeMessage)

	go client.Write()
	go client.read()
}
