package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
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
	Board         [][]int        `json:"board"`         // [row][col], 0=빈칸, 1~4=플레이어
	Players       [4]string      `json:"players"`       // [0]~[3]
	Scores        [4]int         `json:"scores"`        // 각 플레이어 점수
	CurrentPieces [4]*TetrisPiece `json:"currentPieces"` // 각 플레이어의 활성 블럭 (nil=없음)
	LastClear     []int          `json:"lastClear"`     // 마지막으로 지워진 행들
	LastScorer    string         `json:"lastScorer"`    // 마지막 점수 획득자
}

// TetrisStateResponse는 테트리스 게임 상태 응답입니다.
type TetrisStateResponse struct {
	Type   string     `json:"type"`
	RoomID string     `json:"roomId"`
	Data   TetrisData `json:"data"`
}

// TetrisMoveResultResponse는 move 결과(성공/실패)를 클라이언트에 전달합니다.
type TetrisMoveResultResponse struct {
	Type    string `json:"type"`
	RoomID  string `json:"roomId"`
	Success bool   `json:"success"`
}

// tetromino shapes: [type][rotation][cell][col,row] offset from piece (X,Y). SRS 표준 회전.
var tetrisShapes = [7][4][4][2]int{
	// I: horizontal / vertical
	{{{-1, 0}, {0, 0}, {1, 0}, {2, 0}}, {{0, -1}, {0, 0}, {0, 1}, {0, 2}}, {{-1, 0}, {0, 0}, {1, 0}, {2, 0}}, {{0, -1}, {0, 0}, {0, 1}, {0, 2}}},
	// O: 2x2 (회전 동일)
	{{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, 1}, {1, 1}}},
	// T: ──┴── / ─┬─ / ──┬── / ─┴─
	{{{0, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {0, -1}, {0, 1}, {1, 0}}, {{-1, 0}, {0, 0}, {1, 0}, {0, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},
	// S: 〓 (가로) / 〓 (세로)
	{{{0, 0}, {1, 0}, {-1, 1}, {0, 1}}, {{0, 0}, {0, -1}, {1, 0}, {1, 1}}, {{0, 0}, {1, 0}, {-1, 1}, {0, 1}}, {{0, 0}, {0, -1}, {1, 0}, {1, 1}}},
	// Z: 〓 (가로) / 〓 (세로)
	{{{-1, 0}, {0, 0}, {0, 1}, {1, 1}}, {{0, 0}, {0, 1}, {1, 1}, {1, 2}}, {{-1, 0}, {0, 0}, {0, 1}, {1, 1}}, {{0, 0}, {0, 1}, {1, 1}, {1, 2}}},
	// J: ┌── / ─┬ / ──┐ / ┬─
	{{{-1, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {1, 0}, {0, -1}, {0, 1}}, {{-1, 0}, {0, 0}, {1, 0}, {1, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},
	// L: ──┐ / ┬─ / ┌── / ─┴ (SRS: stem bottom-right at rot1)
	{{{1, 0}, {-1, 1}, {0, 1}, {1, 1}}, {{0, 0}, {0, -1}, {0, 1}, {1, 1}}, {{-1, 0}, {0, 0}, {1, 0}, {-1, 1}}, {{0, 0}, {-1, 0}, {0, -1}, {0, 1}}},
}

type TetrisGame struct {
	room        *Room
	players     [4]*Client
	board       [][]int
	scores      [4]int
	pieces      [4]*TetrisPiece
	gameStarted bool
	startReady  map[*Client]bool
	stopTick    chan struct{}
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
			g.pieces[i] = nil
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
	for i := 0; i < tetrisMaxPlayers; i++ {
		g.pieces[i] = nil
	}
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] != nil {
			if !g.spawnPieceForSlotLocked(i) {
				g.gameOverLocked()
				return
			}
		}
	}
	g.startGravityTickerLocked()
	g.sendStateToAllLocked()
}

func (g *TetrisGame) stopGravityTickerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *TetrisGame) startGravityTickerLocked() {
	g.stopGravityTickerLocked()
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				g.mu.Lock()
				if !g.gameStarted {
					g.mu.Unlock()
					return
				}
				for i := 0; i < tetrisMaxPlayers; i++ {
					p := g.pieces[i]
					if p == nil || g.players[i] == nil {
						continue
					}
					p.Y++
					if !g.isValidMove(i, p) {
						p.Y--
						g.lockPieceLocked(i)
					}
				}
				g.sendStateToAllLocked()
				g.mu.Unlock()
			}
		}
	}()
}

func (g *TetrisGame) resetLocked() {
	g.gameStarted = false
	g.stopGravityTickerLocked()
	for i := 0; i < tetrisMaxPlayers; i++ {
		g.pieces[i] = nil
	}
	g.startReady = make(map[*Client]bool)
}

// isValidMove: 보드 경계, 굳은 블록, 다른 플레이어의 실시간 블록과 겹치지 않는지 검사
func (g *TetrisGame) isValidMove(slot int, piece *TetrisPiece) bool {
	if piece == nil || slot < 0 || slot >= tetrisMaxPlayers {
		return false
	}
	shape := tetrisShapes[piece.Type][piece.Rotation%4]
	for _, off := range shape {
		col := piece.X + off[0]
		row := piece.Y + off[1]
		if col < 0 || col >= tetrisCols || row >= tetrisRows {
			return false
		}
		if row >= 0 && g.board[row][col] != 0 {
			return false
		}
		// 다른 플레이어의 실시간 블록과 겹치는지
		for other := 0; other < tetrisMaxPlayers; other++ {
			if other == slot || g.pieces[other] == nil {
				continue
			}
			oshape := tetrisShapes[g.pieces[other].Type][g.pieces[other].Rotation%4]
			for _, ooff := range oshape {
				oc := g.pieces[other].X + ooff[0]
				or := g.pieces[other].Y + ooff[1]
				if col == oc && row == or {
					return false
				}
			}
		}
	}
	return true
}

func (g *TetrisGame) spawnPieceForSlotLocked(slot int) bool {
	t := rand.Intn(7)
	rot := 0
	x := 2 + (slot * 3)
	y := 0
	g.pieces[slot] = &TetrisPiece{Type: t, Rotation: rot, X: x, Y: y}
	if !g.isValidMove(slot, g.pieces[slot]) {
		g.pieces[slot] = nil
		return false
	}
	return true
}

func (g *TetrisGame) lockPieceLocked(slot int) {
	if g.pieces[slot] == nil || g.players[slot] == nil {
		return
	}
	piece := g.pieces[slot]
	color := slot + 1
	shape := tetrisShapes[piece.Type][piece.Rotation%4]
	for _, off := range shape {
		col := piece.X + off[0]
		row := piece.Y + off[1]
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
	if pts > 0 && g.players[slot] != nil {
		g.scores[slot] += pts
	}

	lastScorer := ""
	if g.players[slot] != nil {
		lastScorer = g.players[slot].UserID
	}
	g.pieces[slot] = nil

	if !g.spawnPieceForSlotLocked(slot) {
		g.gameOverLocked()
		return
	}

	data := g.makeDataLocked()
	data.LastClear = cleared
	data.LastScorer = lastScorer
	msg, _ := json.Marshal(TetrisStateResponse{Type: "tetris_state", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(msg)
}

func (g *TetrisGame) gameOverLocked() {
	g.gameStarted = false
	msg, _ := json.Marshal(GameResultResponse{
		Type: "game_result", Message: "게임 오버! 보드가 가득 찼습니다.", RoomID: g.room.ID,
		Data:           map[string]any{"scores": g.scores, "players": g.playersUserIDsLocked()},
		RematchEnabled: true,
	})
	g.room.broadcastAll(msg)
	g.resetLocked()
}

func (g *TetrisGame) clientSlot(client *Client) int {
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.players[i] == client {
			return i
		}
	}
	return -1
}

func (g *TetrisGame) sendMoveResult(client *Client, success bool) {
	msg, _ := json.Marshal(TetrisMoveResultResponse{
		Type: "tetris_move_result", RoomID: g.room.ID, Success: success,
	})
	client.SafeSend(msg)
}

func (g *TetrisGame) handleMoveLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted {
		return
	}
	slot := g.clientSlot(client)
	if slot < 0 || g.pieces[slot] == nil {
		return
	}

	var p struct {
		Dir string `json:"dir"` // left, right, down, rotate
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}

	piece := g.pieces[slot]

	switch p.Dir {
	case "left":
		piece.X--
		if !g.isValidMove(slot, piece) {
			piece.X++
			g.sendMoveResult(client, false)
			return
		}
	case "right":
		piece.X++
		if !g.isValidMove(slot, piece) {
			piece.X--
			g.sendMoveResult(client, false)
			return
		}
	case "down":
		piece.Y++
		if !g.isValidMove(slot, piece) {
			piece.Y--
			g.sendMoveResult(client, true) // lock 성공
			g.lockPieceLocked(slot)
			return
		}
		g.sendMoveResult(client, true)
	case "rotate":
		piece.Rotation = (piece.Rotation + 1) % 4
		if !g.isValidMove(slot, piece) {
			piece.Rotation = (piece.Rotation + 3) % 4
			g.sendMoveResult(client, false)
			return
		}
		g.sendMoveResult(client, true)
	default:
		return
	}

	if p.Dir != "down" {
		g.sendMoveResult(client, true)
	}
	g.sendStateToAllLocked()
}

func (g *TetrisGame) handleFlickLocked(client *Client) {
	if !g.gameStarted {
		return
	}
	slot := g.clientSlot(client)
	if slot < 0 || g.pieces[slot] == nil {
		return
	}
	for g.isValidMove(slot, g.pieces[slot]) {
		g.pieces[slot].Y++
	}
	g.pieces[slot].Y--
	g.sendMoveResult(client, true)
	g.lockPieceLocked(slot)
}

func (g *TetrisGame) makeDataLocked() TetrisData {
	boardCopy := make([][]int, tetrisRows)
	for r := 0; r < tetrisRows; r++ {
		boardCopy[r] = make([]int, tetrisCols)
		copy(boardCopy[r], g.board[r])
	}
	var piecesCopy [4]*TetrisPiece
	for i := 0; i < tetrisMaxPlayers; i++ {
		if g.pieces[i] != nil {
			pc := *g.pieces[i]
			piecesCopy[i] = &pc
		}
	}
	return TetrisData{
		Board:         boardCopy,
		Players:       g.playersUserIDsLocked(),
		Scores:        g.scores,
		CurrentPieces: piecesCopy,
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
