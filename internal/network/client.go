package network

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Tuning constants for connection liveness (the heartbeat).
const (
	writeWait      = 10 * time.Second    // max time allowed to write one message
	pongWait       = 60 * time.Second    // we expect a pong within this window
	pingPeriod     = (pongWait * 9) / 10 // ping a bit faster than pongWait, so we never miss
	maxMessageSize = 512                 // inputs are tiny; reject anything bigger (anti-abuse)
)

// upgrader turns a normal HTTP connection into a WebSocket.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Dev-only: accept connections from any origin. We'll restrict this in Phase 5B.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client represents one connected player at the network layer.
type Client struct {
	conn   *websocket.Conn // the underlying socket
	send   chan []byte     // outbound queue: the room/hub writes here, writePump drains it
	hub    *Hub            // back-reference so readPump can forward messages
	player int             // 1 or 2, assigned by the Hub when matched
	code   string          // the room code this client created (if any)
}

// readPump is the ONLY goroutine that reads from this connection.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c // tell the Hub we're gone (cleanup / room teardown)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}

		var m clientMsg
		if err := json.Unmarshal(message, &m); err != nil {
			continue // ignore malformed messages rather than dropping the connection
		}

		// Route by message type. create/join go to matchmaking; input goes to
		// the Hub, which forwards it to the room that owns this client.
		switch m.T {
		case "create":
			c.hub.create <- c
		case "join":
			c.hub.join <- joinReq{client: c, code: m.Code}
		case "input":
			c.hub.inputs <- inputMsg{client: c, dir: m.Dir}
		case "vote":
			c.hub.votes <- voteMsg{client: c, points: m.Points}
		}
	}
}

// writePump is the ONLY goroutine that writes to this connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The send channel was closed (room tore us down). Say goodbye.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
