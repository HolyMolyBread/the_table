package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
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
	Board  [3][3]int      `json:"board"`  // 0=빈칸, 1=O, 2=X
	Turn   string         `json:"turn"`   // 현재 차례인 유저 ID
	Colors map[string]int `json:"colors"` // {"userID": 1(O) 또는 2(X)}
}

// ── TicTacToeGame 플러그인 ────────────────────────────────────────────────────

// TicTacToeGame은 1:1 PVP 틱택토 게임 플러그인입니다.
//   - players[0] = O(1, 선공), players[1] = X(2, 후공)
//   - 2명 모두 입장 시 게임을 시작합니다.
//   - 가로·세로·대각선 3목 완성 시 승리, 9칸이 모두 차면 무승부입니다.
type TicTacToeGame struct {
	room        *Room
	board       [3][3]int  // 0=빈칸, 1=O, 2=X
	players     [2]*Client // [0]=O(선공), [1]=X(후공)
	currentTurn int        // 0 또는 1
	gameStarted bool
	mu          sync.Mutex
}

func NewTicTacToeGame(room *Room) *TicTacToeGame {
	return &TicTacToeGame{room: room}
}

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

	case g.players[1] == nil && g.players[0] != client:
		g.players[1] = client
		g.gameStarted = true
		g.currentTurn = 0 // players[0] = O 선공

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

	default:
		// 3번째 이후 입장자 → 관전자
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

		if g.gameStarted {
			snap, _ := json.Marshal(TicTacToeStateResponse{
				Type:   "tictactoe_state",
				RoomID: g.room.ID,
				Data:   g.makeDataLocked(),
			})
			client.SafeSend(snap)
		}
	}
}

// OnLeave는 플레이어가 퇴장하기 직전에 호출됩니다.
func (g *TicTacToeGame) OnLeave(client *Client) {
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
			winner.RecordResult("tictactoe", "win")
		}
		loser.RecordResult("tictactoe", "lose")

		msg := fmt.Sprintf("[%s]님이 퇴장했습니다. [%s]의 몰수승!", loser.UserID, winner.UserID)
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: msg,
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[TICTACTOE] room:[%s] 몰수승: winner=[%s] loser=[%s]",
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
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 아직 시작되지 않았습니다."})
		return
	}
	if g.players[g.currentTurn] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "상대방의 차례입니다."})
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
			Type:    "game_result",
			Message: fmt.Sprintf("🏆 [%s](%s) 승리!", winner.UserID, symbol),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[TICTACTOE] room:[%s] 승리: [%s](%s)", g.room.ID, winner.UserID, symbol)
		g.resetLocked()
		return
	}

	if g.checkDraw() {
		for _, p := range g.players {
			if p != nil {
				p.RecordResult("tictactoe", "draw")
			}
		}
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: "🤝 무승부입니다!",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[TICTACTOE] room:[%s] 무승부", g.room.ID)
		g.resetLocked()
		return
	}

	g.currentTurn = 1 - g.currentTurn
	g.broadcastStateLocked()
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
	return TicTacToeData{
		Board:  g.board,
		Turn:   g.players[g.currentTurn].UserID,
		Colors: map[string]int{g.players[0].UserID: 1, g.players[1].UserID: 2},
	}
}

// resetLocked는 게임 상태를 완전히 초기화합니다.
func (g *TicTacToeGame) resetLocked() {
	g.board       = [3][3]int{}
	g.gameStarted = false
	g.players     = [2]*Client{}
	g.currentTurn = 0
}
