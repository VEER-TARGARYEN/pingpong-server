package network

import "github.com/VEER-TARGARYEN/pingpong-server/internal/game"

// This file is the single source of truth for the JSON wire protocol. The C#
// client must mirror these shapes exactly.

// ---------- client -> server ----------

// clientMsg is every message a client can send. The "t" field selects the kind:
//   {"t":"create"}                  -> make a new room, get a code back
//   {"t":"join","code":"ABCDEF"}    -> join an existing room by code
//   {"t":"input","dir":-1}          -> set this player's paddle direction
type clientMsg struct {
	T      string `json:"t"`
	Code   string `json:"code"`
	Dir    int    `json:"dir"`
	Points int    `json:"points"` // for "vote": this player's proposed match point
}

// ---------- server -> client ----------

// createdMsg: your room exists, here is the code to share. {"t":"created","code":"ABCDEF"}
type createdMsg struct {
	T    string `json:"t"`
	Code string `json:"code"`
}

// startMsg: the match is beginning and you are player "you" (1 or 2).
type startMsg struct {
	T   string `json:"t"`
	You int    `json:"you"`
}

// errorMsg: something went wrong (e.g. bad room code).
type errorMsg struct {
	T   string `json:"t"`
	Msg string `json:"msg"`
}

// endMsg: the match is over (e.g. the opponent disconnected).
type endMsg struct {
	T      string `json:"t"`
	Reason string `json:"reason"`
}

// beginMsg: both players have voted; here's the agreed target score and play begins.
type beginMsg struct {
	T      string `json:"t"`
	Target int    `json:"target"`
}

// overMsg: someone reached the target. Carries the winner and the final score.
type overMsg struct {
	T      string `json:"t"`
	Winner int    `json:"winner"`
	S1     int    `json:"s1"`
	S2     int    `json:"s2"`
}

// stateMsg is the 60 fps snapshot, tagged so the client can tell it apart from
// control messages. Embedding StateSnapshot anonymously FLATTENS its fields into
// this object, so the JSON is {"t":"s","bx":...,"by":...,"p1y":...,...}.
type stateMsg struct {
	T string `json:"t"`
	game.StateSnapshot
}
