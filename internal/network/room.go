package network

import (
	"encoding/json"
	"log"
	"time"

	"github.com/VEER-TARGARYEN/pingpong-server/internal/game"
)

const (
	tickRate     = 60
	tickInterval = time.Second / tickRate // ~16.67ms per frame
)

// roomState is the room's lifecycle within a match.
type roomState int

const (
	negotiating roomState = iota // waiting for both players' match-point votes
	playing                      // physics running, racing to target
	over                         // someone won; waiting for a rematch (new votes)
)

// voteInput is one player's proposed match point, routed in from the Hub.
type voteInput struct {
	Player int
	Points int
}

// Room is a single match between two clients. Its run() goroutine owns the game
// state and the match lifecycle, so no locks are needed.
type Room struct {
	p1, p2 *Client
	game   *game.Game
	inputs chan game.Input
	votes  chan voteInput
	done   chan struct{}
}

func newRoom(p1, p2 *Client) *Room {
	return &Room{
		p1:     p1,
		p2:     p2,
		game:   game.NewGame(),
		inputs: make(chan game.Input, 16),
		votes:  make(chan voteInput, 8),
		done:   make(chan struct{}),
	}
}

func (r *Room) run() {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	dt := tickInterval.Seconds()

	state := negotiating
	var vote1, vote2, target int

	log.Printf("room started: %s vs %s", r.p1.conn.RemoteAddr(), r.p2.conn.RemoteAddr())

	for {
		select {
		case in := <-r.inputs:
			if state == playing {
				r.game.ApplyInput(in)
			}

		case v := <-r.votes:
			if state == playing {
				break // can't change the target mid-game
			}
			if state == over {
				// A vote after game-over means "rematch": start a fresh round.
				state = negotiating
				vote1, vote2 = 0, 0
			}
			if v.Player == 1 {
				vote1 = clampPoints(v.Points)
			} else {
				vote2 = clampPoints(v.Points)
			}
			if vote1 > 0 && vote2 > 0 {
				// The match point is the AVERAGE of both proposals (rounded).
				target = (vote1 + vote2 + 1) / 2
				r.game = game.NewGame()
				state = playing
				r.broadcastJSON(beginMsg{T: "begin", Target: target})
				log.Printf("match begins: votes %d/%d -> first to %d", vote1, vote2, target)
			}

		case <-ticker.C:
			if state != playing {
				break
			}
			r.game.Update(dt)
			r.broadcastState()
			if s1, s2 := r.game.Scores(); s1 >= target || s2 >= target {
				winner := 1
				if s2 >= target {
					winner = 2
				}
				state = over
				r.broadcastJSON(overMsg{T: "over", Winner: winner, S1: s1, S2: s2})
				log.Printf("game over: winner %d (%d-%d)", winner, s1, s2)
			}

		case <-r.done:
			log.Printf("room closing")
			close(r.p1.send)
			close(r.p2.send)
			return
		}
	}
}

func (r *Room) broadcastState() {
	data, err := json.Marshal(stateMsg{T: "s", StateSnapshot: r.game.Snapshot()})
	if err != nil {
		return
	}
	r.push(r.p1, data)
	r.push(r.p2, data)
}

func (r *Room) broadcastJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	r.push(r.p1, data)
	r.push(r.p2, data)
}

// push enqueues WITHOUT blocking; a full buffer (slow/dead client) just drops
// the frame so the other player's game keeps running.
func (r *Room) push(c *Client, data []byte) {
	select {
	case c.send <- data:
	default:
	}
}

// clampPoints keeps a proposed match point in a sane range.
func clampPoints(p int) int {
	if p < 1 {
		return 1
	}
	if p > 21 {
		return 21
	}
	return p
}
