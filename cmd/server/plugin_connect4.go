package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	c4Rows           = 6  // 보드 행 수
	c4Cols           = 7  // 보드 열 수
	c4TurnTimeLimit  = 15 // 턴당 제한 시간(초)
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// Connect4StateResponse는 매 턴마다 방 전체에 브로드캐스트되는 보드 상태입니다.
type Connect4StateResponse struct {
	Type   string       `json:"type"`
	RoomID string       `json:"roomId"`
	Data   Connect4Data `json:"data"`
}

// Connect4Data는 connect4_state 응답의 data 필드입니다.
type Connect4Data struct {
	Board   [c4Rows][c4Cols]int `json:"board"`   // 0=빈칸, 1=빨강, 2=노랑
	Turn    string              `json:"turn"`    // 현재 차례인 유저 ID
	Colors  map[string]int      `json:"colors"`  // {"userID": 1(빨강) 또는 2(노랑)}
	LastCol int                 `json:"lastCol"` // 마지막으로 둔 열 (-1=없음)
	LastRow int                 `json:"lastRow"` // 마지막으로 둔 행 (-1=없음)
}

// ── Connect4Game 플러그인 ─────────────────────────────────────────────────────

// Connect4Game은 1:1 PVP 4목(Connect 4) 게임 플러그인입니다.
//   - players[0] = 빨강(1, 선공), players[1] = 노랑(2, 후공)
//   - 2명 모두 입장 시 게임을 시작합니다.
//   - 열(col)을 선택하면 중력에 의해 해당 열의 가장 아래 빈 행에 돌이 놓입니다.
//   - 가로·세로·대각선으로 4개를 먼저 이으면 승리, 보드가 꽉 차면 무승부입니다.
type Connect4Game struct {
	room        *Room
	board       [c4Rows][c4Cols]int // 0=빈칸, 1=빨강, 2=노랑
	players     [2]*Client          // [0]=빨강(선공), [1]=노랑(후공)
	currentTurn int                 // 0 또는 1
	gameStarted bool
	lastCol     int             // 마지막 착수 열 (-1=없음)
	lastRow     int             // 마지막 착수 행 (-1=없음)
	stopTick    chan struct{}
	mu          sync.Mutex
}

func NewConnect4Game(room *Room) *Connect4Game {
	return &Connect4Game{room: room, lastCol: -1, lastRow: -1}
}

func (g *Connect4Game) Name() string { return "4목 (Connect 4)" }

// OnJoin은 플레이어가 방에 입장한 직후 호출됩니다.
func (g *Connect4Game) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch {
	case g.players[0] == nil:
		g.players[0] = client
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("🔴 [%s]님이 입장했습니다. 상대방을 기다리는 중...", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

	case g.players[1] == nil && g.players[0] != client:
		g.players[1] = client
		g.gameStarted = true
		g.currentTurn = 0 // players[0] = 빨강 선공

		notice, _ := json.Marshal(ServerResponse{
			Type: "game_notice",
			Message: fmt.Sprintf(
				"게임 시작! 🔴 빨강: [%s]  🟡 노랑: [%s]  — 빨강이 선공입니다.",
				g.players[0].UserID, g.players[1].UserID,
			),
			RoomID: g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.broadcastStateLocked()
		g.startTurnTimerLocked()

	default:
		// 3번째 이후 입장자 → 관전자
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

		if g.gameStarted {
			snap, _ := json.Marshal(Connect4StateResponse{
				Type:   "connect4_state",
				RoomID: g.room.ID,
				Data:   g.makeDataLocked(),
			})
			client.SafeSend(snap)
		}
	}
}

// OnLeave는 플레이어가 퇴장하기 직전에 호출됩니다.
func (g *Connect4Game) OnLeave(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	playerIdx := -1
	for i, p := range g.players {
		if p == client {
			playerIdx = i
			break
		}
	}
	if playerIdx == -1 {
		return // 관전자 또는 미등록 (처리 불필요)
	}

	if g.gameStarted {
		winner := g.players[1-playerIdx]
		loser := client
		if winner != nil {
			winner.RecordResult("connect4", "win")
		}
		loser.RecordResult("connect4", "lose")

		msg := fmt.Sprintf("[%s]님이 퇴장했습니다. [%s]의 몰수승!", loser.UserID, winner.UserID)
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: msg,
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[CONNECT4] room:[%s] 몰수승: winner=[%s] loser=[%s]",
			g.room.ID, winner.UserID, loser.UserID)
	}

	g.resetLocked()

	// 방 전체에 해산 알림 (error 타입 → 클라이언트 자동 로비 이동)
	dissolveMsg, _ := json.Marshal(ServerResponse{
		Type:    "error",
		Message: "플레이어가 퇴장하여 방이 해산됩니다.",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(dissolveMsg)
	log.Printf("[CONNECT4] room:[%s] 방 해산: [%s] 퇴장", g.room.ID, client.UserID)
}

// HandleAction은 action: "game_action" 메시지를 코어로부터 위임받아 처리합니다.
func (g *Connect4Game) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
		Col int    `json:"col"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "place":
		g.handlePlace(client, p.Col)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 4목 명령: [%s]", p.Cmd),
		})
	}
}

// handlePlace는 열 선택 요청을 처리합니다. 중력에 의해 해당 열의 가장 아래 빈 행에 돌이 놓입니다.
func (g *Connect4Game) handlePlace(client *Client, col int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 아직 시작되지 않았습니다."})
		return
	}
	if g.players[g.currentTurn] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "상대방의 차례입니다."})
		return
	}
	if col < 0 || col >= c4Cols {
		client.SendJSON(ServerResponse{Type: "error", Message: "열 번호가 범위를 벗어났습니다."})
		return
	}

	// 중력 로직: 해당 열의 가장 아래쪽(인덱스가 큰) 빈 행을 탐색합니다.
	row := -1
	for r := c4Rows - 1; r >= 0; r-- {
		if g.board[r][col] == 0 {
			row = r
			break
		}
	}
	if row == -1 {
		// 열이 꽉 참 — 무시 (에러 전송만)
		client.SendJSON(ServerResponse{Type: "error", Message: "해당 열이 꽉 찼습니다."})
		return
	}

	color := g.currentTurn + 1 // 1=빨강, 2=노랑
	g.board[row][col] = color
	g.lastCol = col
	g.lastRow = row

	symbol := map[int]string{1: "🔴", 2: "🟡"}[color]
	log.Printf("[CONNECT4] room:[%s] [%s](%s) col=%d row=%d",
		g.room.ID, client.UserID, symbol, col, row)

	if g.checkWin(row, col, color) {
		winner := g.players[g.currentTurn]
		loser := g.players[1-g.currentTurn]

		winner.RecordResult("connect4", "win")
		loser.RecordResult("connect4", "lose")

		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: fmt.Sprintf("🏆 [%s](%s) 4목 달성! 승리!", winner.UserID, symbol),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[CONNECT4] room:[%s] 승리: [%s](%s)", g.room.ID, winner.UserID, symbol)
		g.stopTurnTimerLocked()
		g.resetLocked()
		return
	}

	if g.checkDraw() {
		for _, p := range g.players {
			if p != nil {
				p.RecordResult("connect4", "draw")
			}
		}
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: "🤝 무승부입니다!",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[CONNECT4] room:[%s] 무승부", g.room.ID)
		g.stopTurnTimerLocked()
		g.resetLocked()
		return
	}

	g.currentTurn = 1 - g.currentTurn
	g.broadcastStateLocked()
	g.startTurnTimerLocked()
}

// ── 타이머 ────────────────────────────────────────────────────────────────────

func (g *Connect4Game) startTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	currentPlayer := g.players[g.currentTurn]
	room := g.room

	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  currentPlayer.UserID,
		Remaining: c4TurnTimeLimit,
	})
	g.room.broadcastAll(data)

	go func() {
		remaining := c4TurnTimeLimit
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for remaining > 0 {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				remaining--
				data, _ := json.Marshal(TimerTickMessage{
					Type:      "timer_tick",
					RoomID:    room.ID,
					TurnUser:  currentPlayer.UserID,
					Remaining: remaining,
				})
				room.broadcastAll(data)
			}
		}
		g.handleTimeOver(currentPlayer)
	}()
}

func (g *Connect4Game) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *Connect4Game) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted || g.players[g.currentTurn] != timedOutPlayer {
		return
	}
	winner := g.players[1-g.currentTurn]
	loser := timedOutPlayer
	winner.RecordResult("connect4", "win")
	loser.RecordResult("connect4", "lose")
	msg := fmt.Sprintf("⏰ [%s]님 시간 초과! [%s]의 승리!", loser.UserID, winner.UserID)
	data, _ := json.Marshal(GameResultResponse{Type: "game_result", Message: msg, RoomID: g.room.ID})
	g.room.broadcastAll(data)
	log.Printf("[CONNECT4] room:[%s] 시간초과: loser=[%s]", g.room.ID, loser.UserID)
	g.resetLocked()
}

// ── 승부 판정 ─────────────────────────────────────────────────────────────────

// checkWin은 (row, col)에 놓인 color가 4목을 달성했는지 검사합니다.
// 가로(0,1), 세로(1,0), 우하향 대각선(1,1), 우상향 대각선(1,-1) 4방향을 검사합니다.
func (g *Connect4Game) checkWin(row, col, color int) bool {
	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		if g.countDir(row, col, d[0], d[1], color) >= 4 {
			return true
		}
	}
	return false
}

// countDir은 (row, col)을 중심으로 방향 (dr, dc)와 그 반대 방향으로 연속된 color 개수를 셉니다.
func (g *Connect4Game) countDir(row, col, dr, dc, color int) int {
	count := 1
	for i := 1; i < 4; i++ {
		r, c := row+dr*i, col+dc*i
		if r < 0 || r >= c4Rows || c < 0 || c >= c4Cols || g.board[r][c] != color {
			break
		}
		count++
	}
	for i := 1; i < 4; i++ {
		r, c := row-dr*i, col-dc*i
		if r < 0 || r >= c4Rows || c < 0 || c >= c4Cols || g.board[r][c] != color {
			break
		}
		count++
	}
	return count
}

// checkDraw는 모든 열의 최상단 행이 채워졌는지(보드 꽉 참) 검사합니다.
func (g *Connect4Game) checkDraw() bool {
	for c := 0; c < c4Cols; c++ {
		if g.board[0][c] == 0 {
			return false
		}
	}
	return true
}

// ── 유틸리티 ──────────────────────────────────────────────────────────────────

func (g *Connect4Game) broadcastStateLocked() {
	data, _ := json.Marshal(Connect4StateResponse{
		Type:   "connect4_state",
		RoomID: g.room.ID,
		Data:   g.makeDataLocked(),
	})
	g.room.broadcastAll(data)
}

func (g *Connect4Game) makeDataLocked() Connect4Data {
	return Connect4Data{
		Board:   g.board,
		Turn:    g.players[g.currentTurn].UserID,
		Colors:  map[string]int{g.players[0].UserID: 1, g.players[1].UserID: 2},
		LastCol: g.lastCol,
		LastRow: g.lastRow,
	}
}

// resetLocked는 게임 상태를 완전히 초기화합니다.
func (g *Connect4Game) resetLocked() {
	g.board       = [c4Rows][c4Cols]int{}
	g.gameStarted = false
	g.players     = [2]*Client{}
	g.currentTurn = 0
	g.lastCol     = -1
	g.lastRow     = -1
}
