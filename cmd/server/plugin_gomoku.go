package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	gomokuSize    = 15 // 15×15 보드
	turnTimeLimit = 15 // 턴당 제한 시간(초)
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// BoardUpdateResponse는 돌이 놓일 때마다 방 전체에 브로드캐스트되는 보드 상태입니다.
type BoardUpdateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   BoardData `json:"data"`
}

// BoardData는 board_update / game_result 공통으로 사용하는 보드 상태 페이로드입니다.
type BoardData struct {
	Board    [gomokuSize][gomokuSize]int `json:"board"`
	Turn     string                     `json:"turn"`     // 현재 차례인 유저 ID
	Colors   map[string]int             `json:"colors"`   // {"player_A": 1, "player_B": 2}
	LastMove [2]int                     `json:"lastMove"` // [-1,-1] = 없음
}

// GomokuResultData는 game_result 응답의 data 필드입니다.
type GomokuResultData struct {
	Board    [gomokuSize][gomokuSize]int `json:"board"`
	Winner   string                     `json:"winner"`
	Colors   map[string]int             `json:"colors"`
	LastMove [2]int                     `json:"lastMove"`
}

// ── GomokuGame 플러그인 ───────────────────────────────────────────────────────

// GomokuGame은 1:1 PVP 오목 게임 플러그인입니다.
//   - players[0] = 흑(1), players[1] = 백(2)
//   - 2명 모두 입장 시 math/rand로 흑/백을 무작위 배정합니다.
//   - 렌주룰(쌍삼/쌍사/장목)은 흑(1)에게만 적용됩니다.
//   - 턴마다 15초 타이머가 동작하며, 초과 시 해당 유저가 패배합니다.
//   - 게임 종료 후 players를 유지하고 리매치(rematch) 기능을 지원합니다.
type GomokuGame struct {
	room         *Room
	board        [gomokuSize][gomokuSize]int
	players      [2]*Client // [0]=흑, [1]=백
	currentTurn  int        // 0 또는 1
	gameStarted  bool
	lastMove     [2]int    // 직전 착수 좌표, [-1,-1]=없음
	stopTick     chan struct{}
	startReady   [2]bool   // 게임 시작 전 준비 상태
	rematchReady [2]bool   // Phase 4.3: 각 플레이어의 리매치 레디 여부
	mu           sync.Mutex
}

func NewGomokuGame(room *Room) *GomokuGame {
	return &GomokuGame{room: room, lastMove: [2]int{-1, -1}}
}

func init() { RegisterPlugin("omok", func(room *Room) GamePlugin { return NewGomokuGame(room) }) }

func (g *GomokuGame) Name() string { return "1:1 PVP 오목 (Gomoku)" }

// OnJoin은 플레이어가 방에 입장한 직후 호출됩니다.
func (g *GomokuGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 재접속: 동일 UserID가 이미 슬롯에 있으면 새 클라이언트 포인터로 갱신 후 보드/타이머 전송
	for i := range g.players {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			g.players[i] = client
			turnUserID := ""
			if g.gameStarted && g.players[g.currentTurn] != nil {
				turnUserID = g.players[g.currentTurn].UserID
			}
			snap, _ := json.Marshal(BoardUpdateResponse{
				Type:   "board_update",
				RoomID: g.room.ID,
				Data: BoardData{
					Board:    g.board,
					Turn:     turnUserID,
					Colors:   g.makeColorMap(),
					LastMove: g.lastMove,
				},
			})
			client.SafeSend(snap)
			if g.gameStarted && g.players[g.currentTurn] != nil {
				g.broadcastTimerTickLocked(turnTimeLimit, g.players[g.currentTurn].UserID)
			}
			return
		}
	}

	switch {
	case g.players[0] == nil:
		g.players[0] = client
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("⚫ [%s]님이 입장했습니다. 상대방을 기다리는 중...", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: 1,
		})
		g.room.broadcastAll(upd)

	case g.players[1] == nil && (g.players[0] == nil || g.players[0].UserID != client.UserID):
		g.players[1] = client
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type:       "ready_update",
			RoomID:     g.room.ID,
			ReadyCount:  0,
			TotalCount: 2,
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

		turnUserID := ""
		if g.gameStarted && g.players[g.currentTurn] != nil {
			turnUserID = g.players[g.currentTurn].UserID
		}
		snap, _ := json.Marshal(BoardUpdateResponse{
			Type:   "board_update",
			RoomID: g.room.ID,
			Data: BoardData{
				Board:    g.board,
				Turn:     turnUserID,
				Colors:   g.makeColorMap(),
				LastMove: g.lastMove,
			},
		})
		client.SafeSend(snap)
	}
}

// OnLeave는 플레이어가 퇴장한 직후 호출됩니다.
func (g *GomokuGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	playerIdx := -1
	for i, p := range g.players {
		if p != nil && p.UserID == client.UserID {
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
		log.Printf("[GOMOKU] room:[%s] [%s] 퇴장 — 난입 대기", g.room.ID, client.UserID)
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
	log.Printf("[GOMOKU] room:[%s] 방 해산: [%s] 퇴장", g.room.ID, client.UserID)
}

// HandleAction은 action: "game_action" 메시지를 코어로부터 위임받아 처리합니다.
func (g *GomokuGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
		X   int    `json:"x"`
		Y   int    `json:"y"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "place":
		g.handlePlace(client, p.X, p.Y)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 오목 명령: [%s]", p.Cmd),
		})
	}
}

// handlePlace는 돌 착수 요청을 처리합니다.
func (g *GomokuGame) handlePlace(client *Client, x, y int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}
	if x < 0 || x >= gomokuSize || y < 0 || y >= gomokuSize {
		client.SendJSON(ServerResponse{Type: "error", Message: "보드 범위를 벗어난 좌표입니다."})
		return
	}
	if g.board[x][y] != 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 돌이 놓여 있는 자리입니다."})
		return
	}

	color := g.currentTurn + 1 // 1=흑, 2=백

	// 흑(1)에게만 렌주룰 적용
	if color == 1 {
		if forbidden, reason := g.isRenjuForbidden(x, y); forbidden {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: fmt.Sprintf("렌주룰 금수(%s) 자리입니다. 다른 곳에 두세요.", reason),
			})
			return
		}
	}

	g.board[x][y] = color
	g.lastMove = [2]int{x, y}
	log.Printf("[GOMOKU] room:[%s] [%s](%s) place (%d,%d)",
		g.room.ID, client.UserID, map[int]string{1: "흑", 2: "백"}[color], x, y)

	if g.checkWin(x, y, color) {
		winner := g.players[g.currentTurn]
		loser := g.players[1-g.currentTurn]
		colorName := map[int]string{1: "흑", 2: "백"}[color]

		winner.RecordResult("omok", "win")
		loser.RecordResult("omok", "lose")

		colors := g.makeColorMap()
		data, _ := json.Marshal(GameResultResponse{
			Type:           "game_result",
			Message:        fmt.Sprintf("🏆 [%s](%s)의 5목 달성! 승리!", winner.UserID, colorName),
			RoomID:         g.room.ID,
			RematchEnabled: true, // Phase 4.3: 리매치 버튼 표시
			Data: GomokuResultData{
				Board:    g.board,
				Winner:   winner.UserID,
				Colors:   colors,
				LastMove: g.lastMove,
			},
		})
		g.room.broadcastAll(data)
		log.Printf("[GOMOKU] room:[%s] 승리: [%s](%s)", g.room.ID, winner.UserID, colorName)
		g.endGameLocked() // players는 유지, 보드만 초기화
		return
	}

	// 승부 미결 — 턴 전환 + 보드/타이머 브로드캐스트
	g.currentTurn = 1 - g.currentTurn
	g.broadcastBoardLocked()
	g.startTurnTimerLocked()
}

// ── 렌주룰 (흑 전용 금수 판정) ───────────────────────────────────────────────

// ── 렌주룰 (흑 전용 금수 판정) — 패턴 매칭 방식 ──────────────────────────────
//
// 각 방향에서 (x, y)를 중심으로 11칸 선분(문자열)을 추출한 뒤
// strings.Contains로 미리 정의된 패턴을 검사합니다.
//   '0' = 빈칸, '1' = 흑, '2' = 백 또는 보드 밖(벽)
//
// 판정 순서:
//  1. 정확한 5목 (countLine == 5) → 승리이므로 금수 아님
//  2. 장목 (countLine >= 6)       → 금수
//  3. 쌍사(4-4): 두 방향 이상에서 4목 위협 패턴 → 금수
//  4. 쌍삼(3-3): 두 방향 이상에서 활삼(活三) 패턴 → 금수

// IsRenjuForbiddenBoard는 보드 복사본에서 (x,y)에 흑을 두는 것이 렌주룰 위반인지 검사합니다.
// bot.go 등에서 보드 상태만으로 금수 여부를 판단할 때 사용합니다. 호출 전 board[x][y]==0 이어야 합니다.
func IsRenjuForbiddenBoard(board [gomokuSize][gomokuSize]int, x, y int) (forbidden bool, reason string) {
	board[x][y] = 1
	defer func() { board[x][y] = 0 }()

	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		if countLineBoard(board, x, y, d[0], d[1], 1) == 5 {
			return false, ""
		}
	}
	for _, d := range dirs {
		if countLineBoard(board, x, y, d[0], d[1], 1) >= 6 {
			return true, "장목(6목 이상)"
		}
	}
	fourCount := 0
	for _, d := range dirs {
		if hasFourPattern(extractLineBoard(board, x, y, d[0], d[1])) {
			fourCount++
		}
	}
	if fourCount >= 2 {
		return true, "쌍사(4-4)"
	}
	threeCount := 0
	for _, d := range dirs {
		if hasOpenThreePattern(extractLineBoard(board, x, y, d[0], d[1])) {
			threeCount++
		}
	}
	if threeCount >= 2 {
		return true, "쌍삼(3-3)"
	}
	return false, ""
}

func extractLineBoard(board [gomokuSize][gomokuSize]int, x, y, dx, dy int) string {
	buf := make([]byte, 11)
	for i := -5; i <= 5; i++ {
		nx, ny := x+dx*i, y+dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize {
			buf[i+5] = '2'
		} else {
			buf[i+5] = byte('0' + board[nx][ny])
		}
	}
	return string(buf)
}

func countLineBoard(board [gomokuSize][gomokuSize]int, x, y, dx, dy, color int) int {
	count := 1
	for i := 1; i < gomokuSize; i++ {
		nx, ny := x+dx*i, y+dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize || board[nx][ny] != color {
			break
		}
		count++
	}
	for i := 1; i < gomokuSize; i++ {
		nx, ny := x-dx*i, y-dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize || board[nx][ny] != color {
			break
		}
		count++
	}
	return count
}

func hasFourPattern(line string) bool {
	for _, p := range []string{
		"01111", "11110", "10111", "11011", "11101",
	} {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

func hasOpenThreePattern(line string) bool {
	for _, p := range []string{"01110", "010110", "011010"} {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

// isRenjuForbidden은 (x, y)에 흑을 두는 것이 렌주룰 위반인지 검사합니다.
// 호출 전 board[x][y] == 0 이어야 합니다.
func (g *GomokuGame) isRenjuForbidden(x, y int) (forbidden bool, reason string) {
	g.board[x][y] = 1
	defer func() { g.board[x][y] = 0 }()

	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}

	// 1) 정확한 5목 → 즉시 승리 (금수 해제)
	for _, d := range dirs {
		if g.countLine(x, y, d[0], d[1], 1) == 5 {
			return false, ""
		}
	}
	// 2) 장목(6목 이상)
	for _, d := range dirs {
		if g.countLine(x, y, d[0], d[1], 1) >= 6 {
			return true, "장목(6목 이상)"
		}
	}

	// 3) 쌍사(4-4) — 두 방향 이상에서 4목 위협 패턴 검출
	fourCount := 0
	for _, d := range dirs {
		if g.hasFourPattern(g.extractLine(x, y, d[0], d[1])) {
			fourCount++
		}
	}
	if fourCount >= 2 {
		return true, "쌍사(4-4)"
	}

	// 4) 쌍삼(3-3) — 두 방향 이상에서 활삼(活三) 패턴 검출
	threeCount := 0
	for _, d := range dirs {
		if g.hasOpenThreePattern(g.extractLine(x, y, d[0], d[1])) {
			threeCount++
		}
	}
	if threeCount >= 2 {
		return true, "쌍삼(3-3)"
	}

	return false, ""
}

// extractLine은 (x, y)를 중심으로 방향 (dx, dy)로 11자 선분 문자열을 반환합니다.
// 중심 인덱스는 5(0-indexed). 범위 밖 셀은 '2'(벽)로 처리합니다.
// board[x][y] == 1 이어야 합니다 (임시 착수 상태).
func (g *GomokuGame) extractLine(x, y, dx, dy int) string {
	buf := make([]byte, 11)
	for i := -5; i <= 5; i++ {
		nx, ny := x+dx*i, y+dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize {
			buf[i+5] = '2'
		} else {
			buf[i+5] = byte('0' + g.board[nx][ny])
		}
	}
	return string(buf)
}

// hasFourPattern은 11자 선분에서 "사(四)" 위협 패턴을 검사합니다.
//
// 사(四) 패턴: 5칸 안에 흑 4개 + 빈칸 1개 (완성하면 정확히 5목).
// 직선사 (_OOOO, OOOO_) 및 꺾인사 (O_OOO, OO_OO, OOO_O) 모두 포함.
func (g *GomokuGame) hasFourPattern(line string) bool {
	for _, p := range []string{
		"01111", "11110", // 직선사 (한쪽 열린)
		"10111", "11011", "11101", // 꺾인사
	} {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

// hasOpenThreePattern은 11자 선분에서 "활삼(活三)" 패턴을 검사합니다.
//
// 활삼: 한 수 더 두면 활사(活四, _OOOO_)가 되는 3목 배열.
//   "01110"  → _OOO_  (직선 활삼)
//   "010110" → _O_OO_ (꺾인 활삼 1)
//   "011010" → _OO_O_ (꺾인 활삼 2)
func (g *GomokuGame) hasOpenThreePattern(line string) bool {
	for _, p := range []string{
		"01110",   // _OOO_
		"010110",  // _O_OO_
		"011010",  // _OO_O_
	} {
		if strings.Contains(line, p) {
			return true
		}
	}
	return false
}

// countLine은 (x, y)에서 방향 (dx, dy)로 color 돌의 연속 개수를 셉니다.
// board[x][y] == color 이어야 합니다.
func (g *GomokuGame) countLine(x, y, dx, dy, color int) int {
	count := 1
	for i := 1; i < gomokuSize; i++ {
		nx, ny := x+dx*i, y+dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize || g.board[nx][ny] != color {
			break
		}
		count++
	}
	for i := 1; i < gomokuSize; i++ {
		nx, ny := x-dx*i, y-dy*i
		if nx < 0 || nx >= gomokuSize || ny < 0 || ny >= gomokuSize || g.board[nx][ny] != color {
			break
		}
		count++
	}
	return count
}

// checkWin은 (x, y)에 놓인 color 돌이 승리 조건을 만족하는지 검사합니다.
//   - 흑(1): 정확히 5목 (6목 이상은 렌주룰로 이미 차단됨)
//   - 백(2): 5목 이상 모두 승리
func (g *GomokuGame) checkWin(x, y, color int) bool {
	for _, d := range [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
		n := g.countLine(x, y, d[0], d[1], color)
		if color == 1 {
			if n == 5 {
				return true
			}
		} else {
			if n >= 5 {
				return true
			}
		}
	}
	return false
}

// ── 타이머 ────────────────────────────────────────────────────────────────────

// startTurnTimerLocked는 새 턴 타이머를 시작합니다. g.mu 보유 상태에서 호출해야 합니다.
// 초기 틱(15초)을 즉시 브로드캐스트한 후 1초 간격 고루틴을 시작합니다.
func (g *GomokuGame) startTurnTimerLocked() {
	g.stopTurnTimerLocked() // 기존 타이머 정리

	stopCh := make(chan struct{})
	g.stopTick = stopCh

	currentPlayer := g.players[g.currentTurn]
	room := g.room

	// 초기 틱 (15초) 즉시 전송
	g.broadcastTimerTickLocked(turnTimeLimit, currentPlayer.UserID)

	go func() {
		remaining := turnTimeLimit
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
		// 시간 초과 — 고루틴이 직접 패배 처리를 수행합니다.
		g.handleTimeOver(currentPlayer)
	}()
}

// stopTurnTimerLocked는 실행 중인 타이머 고루틴을 중단합니다. g.mu 보유 상태에서 호출해야 합니다.
func (g *GomokuGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *GomokuGame) broadcastTimerTickLocked(remaining int, turnUser string) {
	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  turnUser,
		Remaining: remaining,
	})
	g.room.broadcastAll(data)
}

// handleTimeOver는 시간 초과 패배를 처리합니다.
// 타이머 고루틴에서 g.mu를 보유하지 않은 상태에서 호출됩니다.
func (g *GomokuGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 게임이 이미 종료됐거나 차례가 바뀐 경우 무시 (중복 실행 방지)
	if !g.gameStarted || g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != timedOutPlayer.UserID {
		return
	}

	winner := g.players[1-g.currentTurn]
	loser := g.players[g.currentTurn]

	winner.RecordResult("omok", "win")
	loser.RecordResult("omok", "lose")

	msg := fmt.Sprintf("⏰ [%s]님 시간 초과! [%s]의 승리!",
		loser.UserID, winner.UserID)
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		RematchEnabled: true, // Phase 4.3
	})
	g.room.broadcastAll(data)
	log.Printf("[GOMOKU] room:[%s] 시간초과: loser=[%s]", g.room.ID, loser.UserID)

	g.endGameLocked() // players는 유지, 보드만 초기화
}

// ── 유틸리티 ──────────────────────────────────────────────────────────────────

func (g *GomokuGame) broadcastBoardLocked() {
	data, _ := json.Marshal(BoardUpdateResponse{
		Type:   "board_update",
		RoomID: g.room.ID,
		Data: BoardData{
			Board:    g.board,
			Turn:     g.players[g.currentTurn].UserID,
			Colors:   g.makeColorMap(),
			LastMove: g.lastMove,
		},
	})
	g.room.broadcastAll(data)
}

func (g *GomokuGame) makeColorMap() map[string]int {
	m := make(map[string]int)
	if g.players[0] != nil {
		m[g.players[0].UserID] = 1
	}
	if g.players[1] != nil {
		m[g.players[1].UserID] = 2
	}
	return m
}

// endGameLocked는 게임을 종료하지만 players를 유지합니다 (리매치용).
// g.mu 보유 상태에서 호출합니다.
func (g *GomokuGame) endGameLocked() {
	g.stopTurnTimerLocked()
	g.board = [gomokuSize][gomokuSize]int{}
	g.gameStarted = false
	g.lastMove = [2]int{-1, -1}
	g.rematchReady = [2]bool{}
	g.startReady = [2]bool{}
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: total,
	})
	g.room.broadcastAll(upd)
}

// resetLocked는 게임 상태와 players를 모두 초기화합니다 (퇴장/완전 초기화용).
// g.mu 보유 상태에서 호출합니다.
func (g *GomokuGame) resetLocked() {
	g.endGameLocked()
	g.players = [2]*Client{}
	g.currentTurn = 0
}

// handleReady는 게임 시작 전 준비 요청을 처리합니다.
// 전원 레디 시 게임을 시작합니다.
func (g *GomokuGame) handleReady(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 이미 시작되었습니다."})
		return
	}
	idx := -1
	for i, p := range g.players {
		if p != nil && p.UserID == client.UserID {
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
	// 전원 레디 → 게임 시작
	g.startReady = [2]bool{}
	if rand.Intn(2) == 1 {
		g.players[0], g.players[1] = g.players[1], g.players[0]
	}
	g.gameStarted = true
	g.currentTurn = 0
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"게임 시작! ⚫ 흑: [%s]  ⚪ 백: [%s]  — 흑이 선공입니다. (렌주룰 적용)",
			g.players[0].UserID, g.players[1].UserID,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.broadcastBoardLocked()
	g.startTurnTimerLocked()
}

// handleRematch는 리매치 요청을 처리합니다.
// 양쪽 모두 레디하면 흑/백을 교체하여 새 게임을 시작합니다.
func (g *GomokuGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}

	// 플레이어 슬롯 확인
	idx := -1
	for i, p := range g.players {
		if p != nil && p.UserID == client.UserID {
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

	// 현재 레디 수 집계
	readyCount := 0
	for _, r := range g.rematchReady {
		if r {
			readyCount++
		}
	}

	// 레디 상태 브로드캐스트
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

	// 양쪽 레디 → 흑/백 교체 후 새 게임 시작
	g.players[0], g.players[1] = g.players[1], g.players[0]
	g.board = [gomokuSize][gomokuSize]int{}
	g.lastMove = [2]int{-1, -1}
	g.rematchReady = [2]bool{}
	g.gameStarted = true
	g.currentTurn = 0

	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"🔄 리매치 시작! ⚫ 흑: [%s]  ⚪ 백: [%s]  — 흑이 선공입니다.",
			g.players[0].UserID, g.players[1].UserID,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.broadcastBoardLocked()
	g.startTurnTimerLocked()
	log.Printf("[GOMOKU] room:[%s] 리매치: 흑=[%s] 백=[%s]",
		g.room.ID, g.players[0].UserID, g.players[1].UserID)
}

func (g *GomokuGame) handleTakeover(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 일시정지 상태가 아닙니다."})
		return
	}
	for i := range g.players {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
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
	g.broadcastBoardLocked()
	g.startTurnTimerLocked()
	log.Printf("[GOMOKU] room:[%s] [%s] 난입 (슬롯 %d)", g.room.ID, client.UserID, emptySlot)
}
