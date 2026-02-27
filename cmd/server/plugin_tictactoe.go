package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	tttTurnTimeLimitBase = 15 // 턴당 기본 제한 시간(초)
	tttTurnTimeLimitMin  = 2  // 터보 모드 최소 시간(초) — 턴마다 0.5초씩 차감
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// TicTacToeStateResponse는 매 턴마다 방 전체에 브로드캐스트되는 보드 상태입니다.
type TicTacToeStateResponse struct {
	Type   string        `json:"type"`
	RoomID string        `json:"roomId"`
	Data   TicTacToeData `json:"data"`
}

// TicTacToeData는 tictactoe_state 응답의 data 필드입니다.
type TicTacToeData struct {
	Board     [3][3]int      `json:"board"`     // 0=빈칸, 1=O, 2=X
	Turn      string         `json:"turn"`      // 현재 차례인 유저 ID
	Colors    map[string]int `json:"colors"`    // {"userID": 1(O) 또는 2(X)}
	Remaining float64        `json:"remaining"` // 터보 모드: 남은 시간(초)
}

// ── TicTacToeGame 플러그인 ────────────────────────────────────────────────────

// TicTacToeGame은 1:1 PVP 틱택토 게임 플러그인입니다.
//   - players[0] = O(1, 선공), players[1] = X(2, 후공)
//   - 2명 모두 입장 시 게임을 시작합니다.
//   - 가로·세로·대각선 3목 완성 시 승리, 9칸이 모두 차면 무승부입니다.
//   - 게임 종료 후 리매치 기능을 지원합니다.
type TicTacToeGame struct {
	room         *Room
	board        [3][3]int  // 0=빈칸, 1=O, 2=X
	players      [2]*Client // [0]=O(선공), [1]=X(후공)
	currentTurn  int        // 0 또는 1
	turnCount    int        // 터보 모드: 턴이 지날수록 제한 시간 감소
	gameStarted  bool
	stopTick     chan struct{}
	startReady   [2]bool
	rematchReady [2]bool
	mu           sync.Mutex
}

func NewTicTacToeGame(room *Room) *TicTacToeGame {
	return &TicTacToeGame{room: room}
}

func init() { RegisterPlugin("tictactoe", func(room *Room) GamePlugin { return NewTicTacToeGame(room) }) }

func (g *TicTacToeGame) Name() string { return "틱택토 (Tic-Tac-Toe)" }

// OnJoin은 플레이어가 방에 입장한 직후 호출됩니다.
func (g *TicTacToeGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch {
	case g.players[0] == nil:
		g.players[0] = client
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("⭕ [%s]님이 입장했습니다. 상대방을 기다리는 중...", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: 1,
		})
		g.room.broadcastAll(upd)

	case g.players[1] == nil && g.players[0] != client:
		g.players[1] = client
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: 2,
		})
		g.room.broadcastAll(upd)

	default:
		// 3번째 이후 입장자 → 관전자 (튕기지 않고 상태 전송)
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

		snap, _ := json.Marshal(TicTacToeStateResponse{
			Type:   "tictactoe_state",
			RoomID: g.room.ID,
			Data:   g.makeDataLocked(),
		})
		client.SafeSend(snap)
	}
}

// OnLeave는 플레이어가 퇴장하기 직전에 호출됩니다.
func (g *TicTacToeGame) OnLeave(client *Client, remainingCount int) {
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
		return
	}

	if g.gameStarted {
		g.players[playerIdx] = nil
		g.stopTurnTimerLocked()
		g.currentTurn = 1 - playerIdx
		playerIds := make([]string, 0)
		if g.players[g.currentTurn] != nil {
			playerIds = append(playerIds, g.players[g.currentTurn].UserID)
		}
		pausedMsg, _ := json.Marshal(struct {
			Type      string   `json:"type"`
			RoomID    string   `json:"roomId"`
			PlayerIds []string `json:"playerIds"`
		}{"game_paused", g.room.ID, playerIds})
		g.room.broadcastAll(pausedMsg)
		log.Printf("[TICTACTOE] room:[%s] [%s] 퇴장 — 난입 대기", g.room.ID, client.UserID)
		return
	}

	if g.players[0] != nil && g.players[1] != nil {
		g.rematchReady = [2]bool{}
		g.startReady = [2]bool{}
	}
	g.resetLocked()
	dissolveMsg, _ := json.Marshal(ServerResponse{
		Type:    "error",
		Message: "플레이어가 퇴장하여 방이 해산됩니다.",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(dissolveMsg)
	log.Printf("[TICTACTOE] room:[%s] 방 해산: [%s] 퇴장", g.room.ID, client.UserID)
}

// HandleAction은 action: "game_action" 메시지를 코어로부터 위임받아 처리합니다.
func (g *TicTacToeGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
		R   int    `json:"r"`
		C   int    `json:"c"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "place":
		g.handlePlace(client, p.R, p.C)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 틱택토 명령: [%s]", p.Cmd),
		})
	}
}

// handlePlace는 돌 착수 요청을 처리합니다.
func (g *TicTacToeGame) handlePlace(client *Client, r, c int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}
	if r < 0 || r >= 3 || c < 0 || c >= 3 {
		client.SendJSON(ServerResponse{Type: "error", Message: "보드 범위를 벗어난 좌표입니다."})
		return
	}
	if g.board[r][c] != 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 표시가 된 자리입니다."})
		return
	}

	color := g.currentTurn + 1 // 1=O, 2=X
	g.board[r][c] = color
	symbol := map[int]string{1: "⭕", 2: "❌"}[color]
	log.Printf("[TICTACTOE] room:[%s] [%s](%s) place (%d,%d)",
		g.room.ID, client.UserID, symbol, r, c)

	if g.checkWin(color) {
		winner := g.players[g.currentTurn]
		loser := g.players[1-g.currentTurn]

		winner.RecordResult("tictactoe", "win")
		loser.RecordResult("tictactoe", "lose")

		data, _ := json.Marshal(GameResultResponse{
			Type:           "game_result",
			Message:        fmt.Sprintf("🏆 [%s](%s) 승리!", winner.UserID, symbol),
			RoomID:         g.room.ID,
			RematchEnabled: true,
		})
		g.room.broadcastAll(data)
		log.Printf("[TICTACTOE] room:[%s] 승리: [%s](%s)", g.room.ID, winner.UserID, symbol)
		g.stopTurnTimerLocked()
		g.endGameLocked()
		return
	}

	if g.checkDraw() {
		// 무한 데스매치: 보드 초기화 후 즉시 다음 라운드, endGame/리매치 대기 없음
		g.board = [3][3]int{}
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "🤝 무승부! 판을 비우고 즉시 다음 라운드를 시작합니다!",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.currentTurn = 1 - g.currentTurn
		g.turnCount++
		g.broadcastStateLocked()
		g.startTurnTimerLocked()
		log.Printf("[TICTACTOE] room:[%s] 무승부 — 데스매치 재시작", g.room.ID)
		return
	}

	g.currentTurn = 1 - g.currentTurn
	g.turnCount++
	g.broadcastStateLocked()
	g.startTurnTimerLocked()
}

// ── 타이머 ────────────────────────────────────────────────────────────────────

func (g *TicTacToeGame) tttTimeLimit() int {
	// 턴마다 0.5초씩 차감 (2턴당 1초)
	limit := tttTurnTimeLimitBase - (g.turnCount+1)/2
	if limit < tttTurnTimeLimitMin {
		limit = tttTurnTimeLimitMin
	}
	return limit
}

func (g *TicTacToeGame) startTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	currentPlayer := g.players[g.currentTurn]
	room := g.room
	limit := g.tttTimeLimit()

	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  currentPlayer.UserID,
		Remaining: limit,
	})
	g.room.broadcastAll(data)

	go func() {
		remaining := limit
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

func (g *TicTacToeGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *TicTacToeGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted || g.players[g.currentTurn] != timedOutPlayer {
		return
	}
	winner := g.players[1-g.currentTurn]
	loser := timedOutPlayer
	winner.RecordResult("tictactoe", "win")
	loser.RecordResult("tictactoe", "lose")
	msg := fmt.Sprintf("⏰ [%s]님 시간 초과! [%s]의 승리!", loser.UserID, winner.UserID)
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		RematchEnabled: true,
	})
	g.room.broadcastAll(data)
	log.Printf("[TICTACTOE] room:[%s] 시간초과: loser=[%s]", g.room.ID, loser.UserID)
	g.endGameLocked()
}

// ── 승부 판정 ─────────────────────────────────────────────────────────────────

// checkWin은 color가 3목을 달성했는지 검사합니다.
func (g *TicTacToeGame) checkWin(color int) bool {
	b := g.board
	// 가로
	for r := 0; r < 3; r++ {
		if b[r][0] == color && b[r][1] == color && b[r][2] == color {
			return true
		}
	}
	// 세로
	for c := 0; c < 3; c++ {
		if b[0][c] == color && b[1][c] == color && b[2][c] == color {
			return true
		}
	}
	// 대각선
	if b[0][0] == color && b[1][1] == color && b[2][2] == color {
		return true
	}
	if b[0][2] == color && b[1][1] == color && b[2][0] == color {
		return true
	}
	return false
}

// checkDraw는 빈칸 없이 모든 칸이 채워졌는지(무승부) 검사합니다.
func (g *TicTacToeGame) checkDraw() bool {
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			if g.board[r][c] == 0 {
				return false
			}
		}
	}
	return true
}

// ── 유틸리티 ──────────────────────────────────────────────────────────────────

func (g *TicTacToeGame) broadcastStateLocked() {
	data, _ := json.Marshal(TicTacToeStateResponse{
		Type:   "tictactoe_state",
		RoomID: g.room.ID,
		Data:   g.makeDataLocked(),
	})
	g.room.broadcastAll(data)
}

func (g *TicTacToeGame) makeDataLocked() TicTacToeData {
	limit := g.tttTimeLimit()
	return TicTacToeData{
		Board:     g.board,
		Turn:      g.players[g.currentTurn].UserID,
		Colors:    map[string]int{g.players[0].UserID: 1, g.players[1].UserID: 2},
		Remaining: float64(limit),
	}
}

// endGameLocked는 게임을 종료하지만 players를 유지합니다 (리매치용).
func (g *TicTacToeGame) endGameLocked() {
	g.board        = [3][3]int{}
	g.gameStarted  = false
	g.rematchReady = [2]bool{}
}

// resetLocked는 게임 상태와 players를 모두 초기화합니다.
func (g *TicTacToeGame) resetLocked() {
	g.endGameLocked()
	g.players     = [2]*Client{}
	g.currentTurn = 0
}

// handleReady는 게임 시작 전 준비 요청을 처리합니다.
func (g *TicTacToeGame) handleReady(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 이미 시작되었습니다."})
		return
	}
	idx := -1
	for i, p := range g.players {
		if p == client {
			idx = i
			break
		}
	}
	if idx < 0 || g.players[0] == nil || g.players[1] == nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	g.startReady[idx] = true
	readyCount := 0
	for _, r := range g.startReady {
		if r {
			readyCount++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: 2,
	})
	g.room.broadcastAll(upd)
	if readyCount < 2 {
		return
	}
	g.startReady = [2]bool{}
	g.gameStarted = true
	g.currentTurn = 0
	g.turnCount = 0
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"게임 시작! ⭕ O: [%s]  ❌ X: [%s]  — O가 선공입니다.",
			g.players[0].UserID, g.players[1].UserID,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.broadcastStateLocked()
	g.startTurnTimerLocked()
}

// handleRematch는 리매치 요청을 처리합니다.
func (g *TicTacToeGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}

	idx := -1
	for i, p := range g.players {
		if p == client {
			idx = i
			break
		}
	}
	if idx == -1 {
		client.SendJSON(ServerResponse{Type: "error", Message: "이 방의 플레이어가 아닙니다."})
		return
	}
	if g.players[0] == nil || g.players[1] == nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "상대방이 없습니다."})
		return
	}

	g.rematchReady[idx] = true
	readyCount := 0
	for _, r := range g.rematchReady {
		if r {
			readyCount++
		}
	}

	upd, _ := json.Marshal(RematchUpdateMessage{
		Type:       "rematch_update",
		RoomID:     g.room.ID,
		ReadyCount: readyCount,
		TotalCount: 2,
	})
	g.room.broadcastAll(upd)

	if readyCount < 2 {
		return
	}

	// 양쪽 레디 → O/X 교체 후 새 게임 시작 (리매치)
	g.players[0], g.players[1] = g.players[1], g.players[0]
	g.board = [3][3]int{}
	g.rematchReady = [2]bool{}
	g.turnCount = 0
	g.gameStarted = true
	g.currentTurn = 0

	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"🔄 리매치 시작! ⭕ O: [%s]  ❌ X: [%s]  — O가 선공입니다.",
			g.players[0].UserID, g.players[1].UserID,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.broadcastStateLocked()
	g.startTurnTimerLocked()
	log.Printf("[TICTACTOE] room:[%s] 리매치: O=[%s] X=[%s]",
		g.room.ID, g.players[0].UserID, g.players[1].UserID)
}

func (g *TicTacToeGame) handleTakeover(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 일시정지 상태가 아닙니다."})
		return
	}
	for i := range g.players {
		if g.players[i] == client {
			client.SendJSON(ServerResponse{Type: "error", Message: "이미 플레이어입니다."})
			return
		}
	}
	emptySlot := -1
	for i := range g.players {
		if g.players[i] == nil {
			emptySlot = i
			break
		}
	}
	if emptySlot < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "난입할 빈자리가 없습니다."})
		return
	}

	g.players[emptySlot] = client
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🪑 [%s]님이 난입했습니다!", client.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.broadcastStateLocked()
	g.startTurnTimerLocked()
	log.Printf("[TICTACTOE] room:[%s] [%s] 난입 (슬롯 %d)", g.room.ID, client.UserID, emptySlot)
}
