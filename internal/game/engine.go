package game

// Game is the authoritative simulation state. It is owned by exactly ONE
// goroutine (the room loop), which is why it needs no mutex: there is never
// more than one goroutine reading or writing these fields at a time.
type Game struct {
	ballX, ballY float64 // ball center
	velX, velY   float64 // ball velocity, units/sec

	p1Y, p2Y   float64 // top edge of each paddle
	dir1, dir2 int      // current movement direction of each paddle (-1/0/+1)

	score1, score2 int
}

// NewGame returns a centered, ready-to-serve game.
func NewGame() *Game {
	g := &Game{}
	g.p1Y = (FieldHeight - PaddleHeight) / 2
	g.p2Y = (FieldHeight - PaddleHeight) / 2
	g.reset(+1) // first serve heads right, toward player 2
	return g
}

// reset re-centers the ball after a point. dir = -1 sends it left (toward P1),
// +1 sends it right (toward P2).
func (g *Game) reset(dir int) {
	g.ballX = FieldWidth / 2
	g.ballY = FieldHeight / 2
	g.velX = BallSpeedX * float64(dir)
	g.velY = BallSpeedY
}

// ApplyInput records a player's current paddle direction. It does NOT move the
// paddle — movement happens in Update, so speed is governed by the clock, not
// by how fast inputs arrive.
func (g *Game) ApplyInput(in Input) {
	switch in.Player {
	case 1:
		g.dir1 = clampDir(in.Dir)
	case 2:
		g.dir2 = clampDir(in.Dir)
	}
}

// Update advances the simulation by dt seconds (1/60 for us). This is a fixed
// timestep: movement = velocity * dt, so the game runs at the same real-world
// speed regardless of the tick rate.
func (g *Game) Update(dt float64) {
	// 1. Move paddles, then clamp them inside the field.
	g.p1Y = clampPaddle(g.p1Y + float64(g.dir1)*PaddleSpeed*dt)
	g.p2Y = clampPaddle(g.p2Y + float64(g.dir2)*PaddleSpeed*dt)

	// 2. Move the ball.
	g.ballX += g.velX * dt
	g.ballY += g.velY * dt

	// 3. Bounce off the top and bottom walls. We snap the ball back to the wall
	//    so it can't tunnel out at high speed, then flip the vertical velocity.
	if g.ballY-BallRadius <= 0 {
		g.ballY = BallRadius
		g.velY = -g.velY
	} else if g.ballY+BallRadius >= FieldHeight {
		g.ballY = FieldHeight - BallRadius
		g.velY = -g.velY
	}

	// 4. Paddle collisions. The "face" is the inner edge each paddle hits with.
	p1Face := float64(PaddleMargin + PaddleWidth)
	p2Face := float64(FieldWidth - PaddleMargin - PaddleWidth)

	// Left paddle (player 1). Only check when the ball is moving left, so it
	// can't get stuck flipping velocity every tick while overlapping.
	if g.velX < 0 && g.ballX-BallRadius <= p1Face {
		if g.ballY >= g.p1Y && g.ballY <= g.p1Y+PaddleHeight {
			g.ballX = p1Face + BallRadius
			g.velX = -g.velX
			g.applySpin(g.p1Y)
		}
	}

	// Right paddle (player 2).
	if g.velX > 0 && g.ballX+BallRadius >= p2Face {
		if g.ballY >= g.p2Y && g.ballY <= g.p2Y+PaddleHeight {
			g.ballX = p2Face - BallRadius
			g.velX = -g.velX
			g.applySpin(g.p2Y)
		}
	}

	// 5. Scoring: the ball left the field past a side wall.
	if g.ballX < 0 {
		g.score2++
		g.reset(+1)
	} else if g.ballX > FieldWidth {
		g.score1++
		g.reset(-1)
	}
}

// applySpin gives the ball "english": where it strikes the paddle controls its
// new vertical angle. Center hit = straight; edges = steep. This is what makes
// Pong feel skillful instead of random.
func (g *Game) applySpin(paddleY float64) {
	// rel is -1 at the paddle's top, 0 at center, +1 at the bottom.
	rel := (g.ballY - (paddleY + PaddleHeight/2)) / (PaddleHeight / 2)
	g.velY = rel * BallSpeedY * 1.5
}

// Scores returns the current score for each player.
func (g *Game) Scores() (int, int) { return g.score1, g.score2 }

// Snapshot copies the current state into the network-facing DTO.
func (g *Game) Snapshot() StateSnapshot {
	return StateSnapshot{
		BallX:  g.ballX,
		BallY:  g.ballY,
		P1Y:    g.p1Y,
		P2Y:    g.p2Y,
		Score1: g.score1,
		Score2: g.score2,
	}
}

func clampPaddle(y float64) float64 {
	if y < 0 {
		return 0
	}
	if y > FieldHeight-PaddleHeight {
		return FieldHeight - PaddleHeight
	}
	return y
}

func clampDir(d int) int {
	switch {
	case d < 0:
		return -1
	case d > 0:
		return 1
	default:
		return 0
	}
}
