package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
)

const (
	tetrisRows     = 20
	tetrisCols     = 15
	tetrisMaxPlayers = 4
)

// TetrisPiece는 현재 활성 블럭입니다.
type TetrisPiece struct {
	Type     int   `json:"type"`     // 0=I,1=O,2=T,3=S,4=Z,5=J,6=L
	Rotation int   `json:"rotation"` // 0~3
	X        int   `json:"x"`
	Y        int   `json:"y"`
}

// TetrisData는 tetris_state 응답의 data 필드입니다.
type TetrisData struct {
	Board        [][]int      `json:"board"`        // [row][col], 0=빈칸, 1~4=플레이어
	Players      [4]string    `json:"players"`      // [0]~[3]
	Scores       [4]int       `json:"scores"`      // 각 플레이어 점수
	CurrentTurn  string       `json:"currentTurn"`  // 현재 차례 유저 ID
	CurrentPiece *TetrisPiece `json:"currentPiece"` // 현재 플레이어의 활성 블럭
	LastClear    []int        `json:"lastClear"`    // 마지막으로 지워진 행들
	LastScorer   string       `json:"lastScorer"`   // 마지막 점수 획득자
}

// TetrisStateResponse는 테트리스 게임 상태 응답입니다.
type TetrisStateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   TetrisData `json:"data"`
}

// tetromino shapes: [rotation][cell][row,col] offset from top-left
var tetrisShapes = [7][4][4][2]int{
	{{{-1, 0}, {0, 0}, {1, 0}, {2, 0}}, {{0, -1}, {0, 0}, {0, 1}, {0, 2}}, {{-1, 0}, {0, 0}, {1, 0}, {2, 0}}, {{0, -1}, {0, 0}, {0, 1}, {0, 2}}}, // I
	{{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}},             // O
	{{{0, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {0, -1}, {0, 1}, {1, 0}}, {{-1, 0}, {0, 0}, {1, 0}, {0, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},   // T
	{{{0, 0}, {1, 0}, {-1, 1}, {0, 1}}, {{0, 0}, {0, -1}, {1, 0}, {1, 1}}, {{0, 0}, {1, 0}, {-1, 1}, {0, 1}}, {{0, 0}, {0, -1}, {1, 0}, {1, 1}}},   // S
	{{{-1, 0}, {0, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, -1}, {1, -1}}, {{-1, 0}, {0, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, -1}, {1, -1}}},   // Z
	{{{-1, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, -1}, {0, 1}}, {{-1, 0}, {0, 0}, {1, 0}, {1, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},   // J
	{{{1, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {0, -1}, {0, 1}, {1, -1}}, {{-1, 0}, {0, 0}, {1, 0}, {-1, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},   // L
}

type TetrisGame struct {
	room        *Room
	players     [4]*Client
	board       [][]int
	scores      [4]int
	currentTurn int
	piece       *TetrisPiece
	gameStarted bool
	startReady  map[*Client]bool
	mu          sync.Mutex
}

func NewTetrisGame(room *Room) *TetrisGame {
	board := make([][]int, tetrisRows)
	for r := 0; r < tetrisRows; r++ {
		board[r] = make([]int, tetrisCols)
	}
	return &TetrisGame{
		room:       room,
		board:      board,
		startReady: make(map[*Client]bool),
	}
}

func init() { RegisterPlugin("tetris", func(room *Room) GamePlugin { return NewTetrisGame(room) }) }

func (g *TetrisGame) Name() string { return "다인용 테트리스 (Shared Tetris)" }

func (g *TetrisGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			g.players[i] = client
			g.sendStateToClientLocked(client)
			return
		}
	}

	slot := -1
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		notice, _ := json.Marshal(ServerResponse{
			Type: "game_notice", Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID), RoomID: g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.sendStateToClientLocked(client)
		return
	}

	g.players[slot] = client
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice", Message: fmt.Sprintf("테트리스 [%s]님이 입장했습니다. (%d/4)", client.UserID, slot+1), RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)

	readyCount, total := 0, 0
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
}

func (g *TetrisGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] == client {
			g.players[i] = nil
			delete(g.startReady, client)
			break
		}
	}
	if remainingCount == 0 {
		log.Printf("[tetris] 방 [%s] 비어서 초기화", g.room.ID)
		g.resetLocked()
	}
}

func (g *TetrisGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var base struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &base); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	switch base.Cmd {
	case "ready":
		g.handleReadyLocked(client)
	case "move":
		g.handleMoveLocked(client, payload)
	case "flick":
		g.handleFlickLocked(client)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 cmd: %s", base.Cmd)})
	}
}

func (g *TetrisGame) handleReadyLocked(client *Client) {
	if g.gameStarted {
		return
	}
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] == client {
			g.startReady[client] = true
			break
		}
	}

	readyCount, total := 0, 0
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total})
	g.room.broadcastAll(upd)

	if readyCount >= 2 && total >= 2 {
		g.startGameLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

func (g *TetrisGame) startGameLocked() {
	g.startReady = make(map[*Client]bool)
	g.gameStarted = true
	g.scores = [4]int{0, 0, 0, 0}
	for r := 0; r < tetrisRows; r++ {
		for c := 0; c < tetrisCols; c++ {
			g.board[r][c] = 0
		}
	}
	g.currentTurn = 0
	g.spawnPieceLocked()
	g.sendStateToAllLocked()
}

func (g *TetrisGame) resetLocked() {
	g.gameStarted = false
	g.piece = nil
	g.startReady = make(map[*Client]bool)
}

func (g *TetrisGame) spawnPieceLocked() bool {
	t := rand.Intn(7)
	rot := 0
	x := tetrisCols/2 - 1
	y := 0
	g.piece = &TetrisPiece{Type: t, Rotation: rot, X: x, Y: y}
	if !g.pieceFitsLocked() {
		g.piece = nil
		return false
	}
	return true
}

func (g *TetrisGame) pieceFitsLocked() bool {
	if g.piece == nil {
		return false
	}
	shape := tetrisShapes[g.piece.Type][g.piece.Rotation%4]
	for _, off := range shape {
		col := g.piece.X + off[0]
		row := g.piece.Y + off[1]
		if col < 0 || col >= tetrisCols || row >= tetrisRows {
			return false
		}
		if row >= 0 && g.board[row][col] != 0 {
			return false
		}
	}
	return true
}

func (g *TetrisGame) lockPieceLocked() {
	if g.piece == nil || g.players[g.currentTurn] == nil {
		return
	}
	color := g.currentTurn + 1
	shape := tetrisShapes[g.piece.Type][g.piece.Rotation%4]
	for _, off := range shape {
		col := g.piece.X + off[0]
		row := g.piece.Y + off[1]
		if row >= 0 && row < tetrisRows && col >= 0 && col < tetrisCols {
			g.board[row][col] = color
		}
	}

	cleared := []int{}
	for r := tetrisRows - 1; r >= 0; r-- {
		full := true
		for c := 0; c < tetrisCols; c++ {
			if g.board[r][c] == 0 {
				full = false
				break
			}
		}
		if full {
			cleared = append(cleared, r)
			for rr := r; rr > 0; rr-- {
				for c := 0; c < tetrisCols; c++ {
					g.board[rr][c] = g.board[rr-1][c]
				}
			}
			for c := 0; c < tetrisCols; c++ {
				g.board[0][c] = 0
			}
			r++
		}
	}

	pts := len(cleared) * 100
	if pts > 0 && g.players[g.currentTurn] != nil {
		g.scores[g.currentTurn] += pts
	}

	lastScorer := ""
	if g.players[g.currentTurn] != nil {
		lastScorer = g.players[g.currentTurn].UserID
	}
	g.piece = nil
	nextTurn := (g.currentTurn + 1) % tetrisMaxPlayers
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[nextTurn] != nil {
			break
		}
		nextTurn = (nextTurn + 1) % tetrisMaxPlayers
	}
	g.currentTurn = nextTurn

	if !g.spawnPieceLocked() {
		g.gameStarted = false
		msg, _ := json.Marshal(GameResultResponse{
			Type: "game_result", Message: "게임 오버! 보드가 가득 찼습니다.", RoomID: g.room.ID,
			Data: map[string]any{"scores": g.scores, "players": g.playersUserIDsLocked()},
			RematchEnabled: true,
		})
		g.room.broadcastAll(msg)
		g.resetLocked()
		return
	}

	data := g.makeDataLocked()
	data.LastClear = cleared
	data.LastScorer = lastScorer
	msg, _ := json.Marshal(TetrisStateResponse{Type: "tetris_state", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(msg)
}

func (g *TetrisGame) handleMoveLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted || g.piece == nil {
		return
	}
	if g.players[g.currentTurn] != client {
		return
	}

	var p struct {
		Dir string `json:"dir"` // left, right, down, rotate
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}

	switch p.Dir {
	case "left":
		g.piece.X--
		if !g.pieceFitsLocked() {
			g.piece.X++
		}
	case "right":
		g.piece.X++
		if !g.pieceFitsLocked() {
			g.piece.X--
		}
	case "down":
		g.piece.Y++
		if !g.pieceFitsLocked() {
			g.piece.Y--
			g.lockPieceLocked()
			return
		}
	case "rotate":
		g.piece.Rotation = (g.piece.Rotation + 1) % 4
		if !g.pieceFitsLocked() {
			g.piece.Rotation = (g.piece.Rotation + 3) % 4
		}
	default:
		return
	}
	g.sendStateToAllLocked()
}

func (g *TetrisGame) handleFlickLocked(client *Client) {
	if !g.gameStarted || g.piece == nil {
		return
	}
	if g.players[g.currentTurn] != client {
		return
	}
	for g.pieceFitsLocked() {
		g.piece.Y++
	}
	g.piece.Y--
	g.lockPieceLocked()
}

func (g *TetrisGame) makeDataLocked() TetrisData {
	boardCopy := make([][]int, tetrisRows)
	for r := 0; r < tetrisRows; r++ {
		boardCopy[r] = make([]int, tetrisCols)
		copy(boardCopy[r], g.board[r])
	}
	turnID := ""
	if g.players[g.currentTurn] != nil {
		turnID = g.players[g.currentTurn].UserID
	}
	var pieceCopy *TetrisPiece
	if g.piece != nil {
		pc := *g.piece
		pieceCopy = &pc
	}
	return TetrisData{
		Board:        boardCopy,
		Players:      g.playersUserIDsLocked(),
		Scores:       g.scores,
		CurrentTurn:  turnID,
		CurrentPiece: pieceCopy,
	}
}

func (g *TetrisGame) playersUserIDsLocked() [4]string {
	var out [4]string
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] != nil {
			out[i] = g.players[i].UserID
		}
	}
	return out
}

func (g *TetrisGame) sendStateToAllLocked() {
	msg, _ := json.Marshal(TetrisStateResponse{Type: "tetris_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	g.room.broadcastAll(msg)
}

func (g *TetrisGame) sendStateToClientLocked(client *Client) {
	msg, _ := json.Marshal(TetrisStateResponse{Type: "tetris_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	client.SafeSend(msg)
}
