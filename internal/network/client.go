package network

import (
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
	// Dev-only: accept connections from any origin. We'll restrict this in Phase 5.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client represents one connected player at the network layer.
type Client struct {
	conn *websocket.Conn // the underlying socket
	send chan []byte     // outbound queue: the game loop writes here, writePump drains it
}

// ServeWS handles a client's request to open a WebSocket.
func ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response on failure.
		log.Printf("upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256), // buffered: absorb bursts without blocking the game loop
	}

	// Two goroutines per client: exactly one reader, exactly one writer.
	go client.writePump()
	go client.readPump()

	log.Printf("client connected: %s", conn.RemoteAddr())
}

// readPump is the ONLY goroutine that reads from this connection.
func (c *Client) readPump() {
	defer func() {
		c.conn.Close() // closing here unblocks writePump's next write, which then also exits
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		// Each pong from the client extends our deadline — proof it's alive.
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
			break // any error (incl. timeout / clean close) ends the loop -> defer closes conn
		}
		log.Printf("received from %s: %s", c.conn.RemoteAddr(), message)
		// Phase 4 hook: this is where an input message becomes a game command.
	}
}

// writePump is the ONLY goroutine that writes to this connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod) // fires every ~54s to send a ping
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The send channel was closed (e.g. hub kicked us). Say goodbye cleanly.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return // write failed -> connection is dead -> exit (defer closes it)
			}

		case <-ticker.C:
			// Heartbeat: send a ping. If the client is gone, this write fails and we bail.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
