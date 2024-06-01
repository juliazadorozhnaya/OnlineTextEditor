package websocket

import (
	"github.com/gorilla/websocket"
	"github.com/juliazadorozhnaya/OnlineTextEditor/file"
	"log"
)

type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	hub       *Hub
	f         *file.File
	username  string
	sessionId string
}

func (c *Client) read() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		c.hub.broadcast <- msg
	}
}

func (c *Client) Write() {
	defer c.conn.Close()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}
