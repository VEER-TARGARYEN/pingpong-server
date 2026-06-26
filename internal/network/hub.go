package network

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"

	"github.com/VEER-TARGARYEN/pingpong-server/internal/game"
)

type inputMsg struct {
	client *Client
	dir    int
}

type joinReq struct {
	client *Client
	code   string
}

type voteMsg struct {
	client *Client
	points int
}

// Hub owns ALL matchmaking state (pending rooms + which client is in which room).
// Everything here is touched only by the Run goroutine, so there are no mutexes.
type Hub struct {
	create     chan *Client
	join       chan joinReq
	quick      chan *Client
	inputs     chan inputMsg
	votes      chan voteMsg
	unregister chan *Client

	pending      map[string]*Client // room code -> the lone creator waiting for an opponent
	quickWaiting *Client            // a lone player in the random-match queue
	rooms        map[*Client]*Room  // client -> the active room it belongs to
}

func NewHub() *Hub {
	return &Hub{
		create:     make(chan *Client),
		join:       make(chan joinReq),
		quick:      make(chan *Client),
		inputs:     make(chan inputMsg, 64),
		votes:      make(chan voteMsg, 16),
		unregister: make(chan *Client),
		pending:    make(map[string]*Client),
		rooms:      make(map[*Client]*Room),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.create:
			code := h.newCode()
			h.pending[code] = c
			c.code = code
			pushJSON(c, createdMsg{T: "created", Code: code})
			log.Printf("room %s created — waiting for opponent", code)

		case req := <-h.join:
			creator, ok := h.pending[req.code]
			if !ok {
				pushJSON(req.client, errorMsg{T: "error", Msg: "room not found"})
				break
			}
			if creator == req.client {
				pushJSON(req.client, errorMsg{T: "error", Msg: "cannot join your own room"})
				break
			}
			delete(h.pending, req.code)
			h.startMatch(creator, req.client)
			log.Printf("room %s started", req.code)

		case c := <-h.quick:
			if h.quickWaiting == nil || h.quickWaiting == c {
				h.quickWaiting = c
				log.Printf("%s queued for quick match", c.conn.RemoteAddr())
			} else {
				p1 := h.quickWaiting
				h.quickWaiting = nil
				h.startMatch(p1, c)
				log.Printf("quick match started")
			}

		case msg := <-h.inputs:
			if room, ok := h.rooms[msg.client]; ok {
				room.inputs <- game.Input{Player: msg.client.player, Dir: msg.dir}
			}

		case v := <-h.votes:
			if room, ok := h.rooms[v.client]; ok {
				room.votes <- voteInput{Player: v.client.player, Points: v.points}
			}

		case c := <-h.unregister:
			h.cleanup(c)
		}
	}
}

// startMatch pairs two clients into a fresh room and kicks off its goroutine.
// Used by both code-join and quick-match.
func (h *Hub) startMatch(p1, p2 *Client) {
	p1.player, p2.player = 1, 2
	room := newRoom(p1, p2)
	h.rooms[p1] = room
	h.rooms[p2] = room
	pushJSON(p1, startMsg{T: "start", You: 1})
	pushJSON(p2, startMsg{T: "start", You: 2})
	go room.run()
}

// cleanup removes a departed client from wherever it was: a pending room, the
// quick-match queue, or an active match (then the opponent is told and the room
// is torn down).
func (h *Hub) cleanup(c *Client) {
	if c.code != "" && h.pending[c.code] == c {
		delete(h.pending, c.code)
		log.Printf("pending room %s abandoned", c.code)
	}
	if h.quickWaiting == c {
		h.quickWaiting = nil
	}

	room, ok := h.rooms[c]
	if !ok {
		return
	}
	opponent := room.p1
	if opponent == c {
		opponent = room.p2
	}
	pushJSON(opponent, endMsg{T: "end", Reason: "opponent_left"})

	delete(h.rooms, room.p1)
	delete(h.rooms, room.p2)
	close(room.done) // stops the room loop, which closes both send channels
}

// newCode returns a short, unique, human-friendly room code. The alphabet omits
// easily-confused characters (no O/0/I/1) so codes are easy to read and type.
func (h *Hub) newCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for {
		raw := make([]byte, 6)
		_, _ = rand.Read(raw)
		code := make([]byte, 6)
		for i, b := range raw {
			code[i] = alphabet[int(b)%len(alphabet)]
		}
		s := string(code)
		if _, taken := h.pending[s]; !taken {
			return s
		}
	}
}

// ServeWS upgrades the HTTP request to a WebSocket and starts the client's pumps.
// The client is idle until it sends a "create" or "join" message.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
		hub:  h,
	}

	go client.writePump()
	go client.readPump()

	log.Printf("client connected: %s", conn.RemoteAddr())
}

// pushJSON marshals v and enqueues it to the client without blocking the Hub. A
// full buffer means a dead/slow client; the heartbeat will evict it shortly.
func pushJSON(c *Client, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}
	select {
	case c.send <- data:
	default:
		log.Printf("dropped control message to %s (buffer full)", c.conn.RemoteAddr())
	}
}
