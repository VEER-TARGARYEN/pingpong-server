package game

// The virtual play field. The server simulates everything in these units.
// The C# client scales these to whatever window size it has (Phase 4),
// so the server never needs to know the client's resolution.
const (
	FieldWidth  = 800
	FieldHeight = 600

	PaddleWidth  = 12
	PaddleHeight = 100
	PaddleMargin = 30    // gap between each paddle's back and the side wall
	PaddleSpeed  = 480.0 // units per SECOND (not per tick) — see Update()

	BallRadius = 10
	BallSpeedX = 350.0 // initial horizontal speed (units/sec)
	BallSpeedY = 220.0 // initial vertical speed (units/sec)
)

// StateSnapshot is the entire payload we broadcast to clients every tick.
// JSON keys are deliberately short ("bx" not "ballX") because this struct is
// serialized and sent 60 times per second to every player — bytes add up.
type StateSnapshot struct {
	BallX  float64 `json:"bx"`
	BallY  float64 `json:"by"`
	P1Y    float64 `json:"p1y"`
	P2Y    float64 `json:"p2y"`
	Score1 int     `json:"s1"`
	Score2 int     `json:"s2"`
}

// Input is one player's intent at a moment in time: which way their paddle
// should currently be moving. Dir is -1 (up), 0 (stop), or +1 (down).
type Input struct {
	Player int // 1 or 2
	Dir    int
}
