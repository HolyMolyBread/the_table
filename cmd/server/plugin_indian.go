package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	indianStartHearts = 10 // 게임 시작 시 각 플레이어의 하트 수
	indianTurnTimeLimit = 30 // 턴당 제한 시간(초) — 초과 시 포기 처리
)

// ── 카드 비교 ─────────────────────────────────────────────────────────────────

// indianCardRank은 인디언 포커 승패 판정용 카드 점수를 반환합니다.
// 숫자(2~A) 우선, 동점 시 문양(♠>♥>♦>♣) 순으로 결정합니다.
func indianCardRank(c Card) int {
	valueRank := map[string]int{
		"2": 2, "3": 3, "4": 4, "5": 5, "6": 6,
		"7": 7, "8": 8, "9": 9, "10": 10,
		"J": 11, "Q": 12, "K": 13, "A": 14,
	}
	suitRank := map[string]int{"♣": 1, "♦": 2, "♥": 3, "♠": 4}
	return valueRank[c.Value]*10 + suitRank[c.Suit]
}

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// IndianStateResponse는 각 플레이어에게 개별 전송되는 게임 상태입니다.
// 브로드캐스트가 아닌 client.SendJSON을 사용하여 개별 전송합니다.
type IndianStateResponse struct {
	Type   string     `json:"type"`
	RoomID string     `json:"roomId"`
	Data   IndianData `json:"data"`
}

// IndianShowdownResultResponse는 승부(콜) 후 각 플레이어에게 개별 전송되는 결과 오버레이용 메시지입니다.
type IndianShowdownResultResponse struct {
	Type   string                   `json:"type"`
	RoomID string                   `json:"roomId"`
	Data   IndianShowdownResultData `json:"data"`
}

// IndianShowdownResultData는 승부 결과의 개인화된 데이터입니다.
type IndianShowdownResultData struct {
	MyCard       Card   `json:"myCard"`       // 본인 카드 (공개)
	OpponentCard Card   `json:"opponentCard"` // 상대 카드 (공개)
	Result       string `json:"result"`       // "win" | "lose"
	HeartDelta   int    `json:"heartDelta"`   // +2 또는 -2
}

// IndianData는 indian_state 응답의 data 필드입니다.
// 각 클라이언트의 관점(본인/상대 구분)으로 개인화된 상태를 담습니다.
type IndianData struct {
	MyCard         Card   `json:"myCard"`         // 본인 카드 (Hidden=true, 앞면 불가)
	OpponentCard   Card   `json:"opponentCard"`   // 상대 카드 (Hidden=false, 앞면 공개)
	MyHearts       int    `json:"myHearts"`       // 본인 하트 수
	OpponentHearts int    `json:"opponentHearts"` // 상대 하트 수
	Turn           string `json:"turn"`           // 현재 차례인 유저 ID
	Phase          string `json:"phase"`          // "waiting" | "first_action" | "second_action"
	Round          int    `json:"round"`          // 현재 라운드 번호
	MyName         string `json:"myName"`         // 본인 유저 ID
	OpponentName   string `json:"opponentName"`   // 상대 유저 ID
	CanTakeover    bool   `json:"canTakeover,omitempty"`
}

// ── IndianGame 플러그인 ───────────────────────────────────────────────────────

// IndianGame은 1:1 PVP 인디언 포커 게임 플러그인입니다.
//
// 규칙:
//   - 각 플레이어는 10개의 하트(❤️)로 시작합니다.
//   - 매 라운드 52장 덱에서 각 1장씩 카드를 받습니다.
//   - 상대방의 카드는 보이지만 자신의 카드는 볼 수 없습니다.
//   - 선공 포기 → 선공 하트 -1 / 선공 승부 → 후공에게 차례 이전
//   - 후공 포기 → 후공 하트 -1 / 후공 승부(콜) → 카드 공개, 승자 +2 패자 -2
//   - 하트가 0 이하가 되면 게임 종료. 리매치 시 양쪽 하트 10개로 재시작.
type IndianGame struct {
	room           *Room
	players        [2]*Client // [0]=선공, [1]=후공 (라운드마다 교체)
	hearts         [2]int     // 각 플레이어의 하트 수
	cards          [2]Card   // 현재 라운드의 각 플레이어 카드
	deck           []Card
	currentTurn    int    // 현재 행동 플레이어 인덱스 (0 또는 1)
	phase          string // "waiting" | "first_action" | "second_action"
	round          int
	gameStarted    bool
	stopTick       chan struct{}
	startReady     [2]bool
	rematchReady   [2]bool
	mu             sync.Mutex
}

func NewIndianGame(room *Room) *IndianGame {
	return &IndianGame{room: room, phase: "waiting"}
}

func init() { RegisterPlugin("indian", func(room *Room) GamePlugin { return NewIndianGame(room) }) }

func (g *IndianGame) Name() string { return "인디언 포커 (Indian Poker)" }

// OnJoin은 플레이어가 방에 입장한 직후 호출됩니다.
func (g *IndianGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	switch {
	case g.players[0] == nil:
		g.players[0] = client
		g.hearts[0] = indianStartHearts
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("🃏 [%s]님이 입장했습니다. 상대방을 기다리는 중...", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: 1,
		})
		g.room.broadcastAll(upd)

	case g.players[1] == nil && g.players[0] != client:
		g.players[1] = client
		g.hearts[1] = indianStartHearts
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: 2,
		})
		g.room.broadcastAll(upd)

	default:
		// 3번째 이후 입장자 → 관전자
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		if g.gameStarted {
			g.sendStateToSpectatorLocked(client)
		}
	}
}

// OnLeave는 플레이어가 퇴장하기 직전에 호출됩니다.
func (g *IndianGame) OnLeave(client *Client, remainingCount int) {
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
		return // 관전자 (처리 불필요)
	}

	if g.gameStarted {
		winner := g.players[1-playerIdx]
		loser := client
		if winner != nil {
			winner.RecordResult("indian", "win")
		}
		loser.RecordResult("indian", "lose")

		msg := fmt.Sprintf("[%s]님이 퇴장했습니다. [%s]의 몰수승!", loser.UserID, winner.UserID)
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: msg,
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
		log.Printf("[INDIAN] room:[%s] 몰수승: winner=[%s] loser=[%s]",
			g.room.ID, winner.UserID, loser.UserID)
	} else if g.players[0] != nil && g.players[1] != nil {
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
	log.Printf("[INDIAN] room:[%s] 방 해산: [%s] 퇴장", g.room.ID, client.UserID)
}

// HandleAction은 action: "game_action" 메시지를 코어로부터 위임받아 처리합니다.
func (g *IndianGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "showdown":
		g.handleShowdown(client)
	case "give_up":
		g.handleGiveUp(client)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 인디언 포커 명령: [%s]", p.Cmd),
		})
	}
}

// handleShowdown은 승부(콜) 요청을 처리합니다.
func (g *IndianGame) handleShowdown(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}

	switch g.phase {
	case "first_action":
		// 선공이 '승부' → 후공에게 차례 이전
		g.stopTurnTimerLocked()
		chatMsg, _ := json.Marshal(ServerResponse{
			Type: "system", Message: fmt.Sprintf("[%s]님이 Call(승부)을 했습니다.", client.UserID),
			RoomID: g.room.ID,
		})
		g.room.broadcastAll(chatMsg)
		time.Sleep(1500 * time.Millisecond)
		g.currentTurn = 1
		g.phase = "second_action"
		notice, _ := json.Marshal(ServerResponse{
			Type: "game_notice",
			Message: fmt.Sprintf(
				"[%s]이 ⚔️ 승부를 선언했습니다! [%s]의 선택을 기다립니다.",
				client.UserID, g.players[1].UserID,
			),
			RoomID: g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.sendStateToBothLocked()
		g.startTurnTimerLocked()

	case "second_action":
		// 후공이 '콜(승부)' → 카드 공개 및 승패 판정
		g.resolveShowdownLocked()

	default:
		client.SendJSON(ServerResponse{Type: "error", Message: "현재 승부를 선언할 수 없습니다."})
	}
}

// handleGiveUp은 포기 요청을 처리합니다.
func (g *IndianGame) handleGiveUp(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}

	idx := g.currentTurn
	prev := g.hearts[idx]
	g.hearts[idx]--

	chatMsg, _ := json.Marshal(ServerResponse{
		Type: "system", Message: fmt.Sprintf("[%s]님이 포기했습니다.", client.UserID),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(chatMsg)
	time.Sleep(1200 * time.Millisecond)
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"[%s]이 🏳️ 포기했습니다. ❤️ %d → %d",
			client.UserID, prev, g.hearts[idx],
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	log.Printf("[INDIAN] room:[%s] 포기: [%s] hearts=%d", g.room.ID, client.UserID, g.hearts[idx])
	g.stopTurnTimerLocked()

	if g.hearts[idx] <= 0 {
		g.endGameLocked(1-idx, idx)
		return
	}
	g.nextRoundLocked()
}

// resolveShowdownLocked은 양쪽 카드를 공개하고 승패를 판정합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) resolveShowdownLocked() {
	c0, c1 := g.cards[0], g.cards[1]
	r0, r1 := indianCardRank(c0), indianCardRank(c1)

	var winnerIdx, loserIdx int
	if r0 >= r1 {
		winnerIdx, loserIdx = 0, 1
	} else {
		winnerIdx, loserIdx = 1, 0
	}

	prevW := g.hearts[winnerIdx]
	prevL := g.hearts[loserIdx]
	g.hearts[winnerIdx] += 2
	g.hearts[loserIdx] -= 2

	chatMsg, _ := json.Marshal(ServerResponse{
		Type: "system", Message: fmt.Sprintf("[%s]님이 Call(승부)을 했습니다.", g.players[1].UserID),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(chatMsg)
	time.Sleep(1200 * time.Millisecond)
	revealMsg := fmt.Sprintf(
		"🃏 공개! [%s]: %s%s  vs  [%s]: %s%s  —  [%s] 승리! ❤️ %d→%d / ❤️ %d→%d",
		g.players[0].UserID, c0.Value, c0.Suit,
		g.players[1].UserID, c1.Value, c1.Suit,
		g.players[winnerIdx].UserID,
		prevW, g.hearts[winnerIdx],
		prevL, g.hearts[loserIdx],
	)
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: revealMsg,
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	log.Printf("[INDIAN] room:[%s] 승부: winner=[%s]%s%s loser=[%s]%s%s",
		g.room.ID,
		g.players[winnerIdx].UserID, g.cards[winnerIdx].Value, g.cards[winnerIdx].Suit,
		g.players[loserIdx].UserID, g.cards[loserIdx].Value, g.cards[loserIdx].Suit,
	)

	// 각 플레이어에게 개인화된 승부 결과 오버레이 전송 (모바일/PC 중앙 토스트용)
	g.sendShowdownResultLocked(winnerIdx, loserIdx)
	g.stopTurnTimerLocked()

	if g.hearts[loserIdx] <= 0 {
		g.endGameLocked(winnerIdx, loserIdx)
		return
	}
	g.nextRoundLocked()
}

// endGameLocked은 게임을 종료하고 전적을 기록합니다. players는 유지하여 리매치를 허용합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) endGameLocked(winnerIdx, loserIdx int) {
	winner := g.players[winnerIdx]
	loser := g.players[loserIdx]

	winner.RecordResult("indian", "win")
	loser.RecordResult("indian", "lose")

	msg := fmt.Sprintf(
		"🏆 [%s] 최종 승리! (❤️×%d)  — [%s] 탈락 (❤️×%d)",
		winner.UserID, g.hearts[winnerIdx],
		loser.UserID, g.hearts[loserIdx],
	)
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		RematchEnabled: true,
	})
	g.room.broadcastAll(data)
	log.Printf("[INDIAN] room:[%s] 게임 종료: winner=[%s] loser=[%s]",
		g.room.ID, winner.UserID, loser.UserID)

	g.gameStarted = false
	g.hearts = [2]int{}
	g.cards = [2]Card{}
	g.deck = nil
	g.round = 0
	g.phase = "waiting"
	g.currentTurn = 0
	g.rematchReady = [2]bool{}
	g.stopTurnTimerLocked()
}

// ── 라운드 관리 ───────────────────────────────────────────────────────────────

// startRoundLocked은 새 라운드를 시작합니다. g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) startRoundLocked() {
	// 덱 부족 시 재셔플
	if len(g.deck) < 2 {
		g.deck = NewShuffledDeck()
	}
	g.cards[0] = g.deck[0]
	g.cards[1] = g.deck[1]
	g.deck = g.deck[2:]

	g.round++
	g.currentTurn = 0
	g.phase = "first_action"

	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"── 라운드 %d 시작! [%s]이 먼저 선택합니다.",
			g.round, g.players[0].UserID,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.sendStateToBothLocked()
	g.startTurnTimerLocked()
}

// nextRoundLocked은 선후공을 교체하고 새 라운드를 시작합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) nextRoundLocked() {
	time.Sleep(1 * time.Second)
	// 선후공 교체: players[0]↔players[1], hearts[0]↔hearts[1]
	g.players[0], g.players[1] = g.players[1], g.players[0]
	g.hearts[0], g.hearts[1] = g.hearts[1], g.hearts[0]
	g.startRoundLocked()
}

// ── 상태 전송 ─────────────────────────────────────────────────────────────────

// sendStateToBothLocked은 각 플레이어에게 개인화된 상태를 개별 전송하고,
// 관전자에게는 양쪽 카드가 모두 보이는 상태를 전송합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) sendStateToBothLocked() {
	g.sendStateToPlayerLocked(0)
	g.sendStateToPlayerLocked(1)

	// 관전자 목록 수집 후 개별 전송
	g.room.mu.RLock()
	spectators := make([]*Client, 0)
	for c := range g.room.clients {
		if c != g.players[0] && c != g.players[1] {
			spectators = append(spectators, c)
		}
	}
	g.room.mu.RUnlock()

	for _, s := range spectators {
		g.sendStateToSpectatorLocked(s)
	}
}

// sendShowdownResultLocked은 승부 후 각 플레이어에게 개인화된 결과 오버레이를 전송합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) sendShowdownResultLocked(winnerIdx, loserIdx int) {
	for i := 0; i < 2; i++ {
		if g.players[i] == nil {
			continue
		}
		oppIdx := 1 - i
		myCard := g.cards[i]
		myCard.Hidden = false
		oppCard := g.cards[oppIdx]
		oppCard.Hidden = false

		var result string
		var heartDelta int
		if i == winnerIdx {
			result = "win"
			heartDelta = 2
		} else {
			result = "lose"
			heartDelta = -2
		}

		g.players[i].SendJSON(IndianShowdownResultResponse{
			Type:   "indian_showdown_result",
			RoomID: g.room.ID,
			Data: IndianShowdownResultData{
				MyCard:       myCard,
				OpponentCard: oppCard,
				Result:       result,
				HeartDelta:   heartDelta,
			},
		})
	}
}

// sendStateToPlayerLocked은 playerIdx 플레이어에게 개인화된 상태를 전송합니다.
// 본인 카드는 뒷면(Hidden=true), 상대 카드는 앞면(Hidden=false)으로 전송합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) sendStateToPlayerLocked(playerIdx int) {
	if g.players[playerIdx] == nil {
		return
	}
	oppIdx := 1 - playerIdx

	myCard := g.cards[playerIdx]
	myCard.Hidden = true // 본인 카드는 뒤집혀 있음
	oppCard := g.cards[oppIdx]
	oppCard.Hidden = false // 상대 카드는 보임

	data := IndianData{
		MyCard:         myCard,
		OpponentCard:   oppCard,
		MyHearts:       g.hearts[playerIdx],
		OpponentHearts: g.hearts[oppIdx],
		Turn:           g.players[g.currentTurn].UserID,
		Phase:          g.phase,
		Round:          g.round,
		MyName:         g.players[playerIdx].UserID,
		OpponentName:   g.players[oppIdx].UserID,
	}
	g.players[playerIdx].SendJSON(IndianStateResponse{
		Type:   "indian_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

// sendStateToSpectatorLocked은 관전자에게 양쪽 카드가 모두 공개된 상태를 전송합니다.
// 관전자는 players[0]의 관점을 기준으로 표시됩니다.
// g.mu 보유 상태에서 호출합니다.
func (g *IndianGame) sendStateToSpectatorLocked(client *Client) {
	canTakeover := !g.gameStarted && (g.players[0] == nil || g.players[1] == nil)
	if canTakeover {
		turn := ""
		if g.players[g.currentTurn] != nil {
			turn = g.players[g.currentTurn].UserID
		}
		myName, oppName := "", ""
		if g.players[0] != nil {
			myName = g.players[0].UserID
		}
		if g.players[1] != nil {
			oppName = g.players[1].UserID
		}
		data := IndianData{
			Phase:       g.phase,
			Round:       g.round,
			Turn:        turn,
			MyName:      myName,
			OpponentName: oppName,
			CanTakeover: true,
		}
		client.SendJSON(IndianStateResponse{Type: "indian_state", RoomID: g.room.ID, Data: data})
		return
	}

	c0 := g.cards[0]
	c0.Hidden = false
	c1 := g.cards[1]
	c1.Hidden = false

	data := IndianData{
		MyCard:         c0,
		OpponentCard:   c1,
		MyHearts:       g.hearts[0],
		OpponentHearts: g.hearts[1],
		Turn:           g.players[g.currentTurn].UserID,
		Phase:          g.phase,
		Round:          g.round,
		MyName:         g.players[0].UserID,
		OpponentName:   g.players[1].UserID,
	}
	client.SendJSON(IndianStateResponse{
		Type:   "indian_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

// ── 초기화 ────────────────────────────────────────────────────────────────────

// ── 타이머 ────────────────────────────────────────────────────────────────────

func (g *IndianGame) startTurnTimerLocked() {
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
		Remaining: indianTurnTimeLimit,
	})
	g.room.broadcastAll(data)

	go func() {
		remaining := indianTurnTimeLimit
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

func (g *IndianGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

// handleTimeOver는 시간 초과 시 포기(give_up)와 동일하게 처리합니다.
func (g *IndianGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted || g.players[g.currentTurn] != timedOutPlayer {
		return
	}
	// 포기와 동일한 로직 실행
	idx := g.currentTurn
	prev := g.hearts[idx]
	g.hearts[idx]--

	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"⏰ [%s] 시간 초과! 포기 처리. ❤️ %d → %d",
			timedOutPlayer.UserID, prev, g.hearts[idx],
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	log.Printf("[INDIAN] room:[%s] 시간초과(포기): [%s] hearts=%d", g.room.ID, timedOutPlayer.UserID, g.hearts[idx])

	if g.hearts[idx] <= 0 {
		g.endGameLocked(1-idx, idx)
		return
	}
	g.nextRoundLocked()
}

// resetLocked는 게임 상태와 players를 모두 초기화합니다.
func (g *IndianGame) resetLocked() {
	g.gameStarted  = false
	g.players      = [2]*Client{}
	g.hearts       = [2]int{}
	g.cards        = [2]Card{}
	g.deck         = nil
	g.round        = 0
	g.phase        = "waiting"
	g.currentTurn  = 0
	g.rematchReady = [2]bool{}
	g.stopTick     = nil
}

// handleReady는 게임 시작 전 준비 요청을 처리합니다.
func (g *IndianGame) handleReady(client *Client) {
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
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"게임 시작! [%s] vs [%s] — 각각 ❤️×%d 하트로 시작합니다!",
			g.players[0].UserID, g.players[1].UserID, indianStartHearts,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startRoundLocked()
}

// handleRematch는 리매치 요청을 처리합니다.
func (g *IndianGame) handleRematch(client *Client) {
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

	// 양쪽 레디 → 하트 10개로 초기화, 선후공 교체 후 새 게임 시작
	g.players[0], g.players[1] = g.players[1], g.players[0]
	g.hearts = [2]int{indianStartHearts, indianStartHearts}
	g.cards = [2]Card{}
	g.deck = nil
	g.round = 0
	g.phase = "waiting"
	g.currentTurn = 0
	g.rematchReady = [2]bool{}
	g.gameStarted = true

	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice",
		Message: fmt.Sprintf(
			"🔄 리매치 시작! [%s] vs [%s] — 각각 ❤️×%d 하트로 다시 시작합니다!",
			g.players[0].UserID, g.players[1].UserID, indianStartHearts,
		),
		RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startRoundLocked()
	log.Printf("[INDIAN] room:[%s] 리매치: [%s] vs [%s]",
		g.room.ID, g.players[0].UserID, g.players[1].UserID)
}

func (g *IndianGame) handleTakeover(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 빈자리 참여가 불가합니다."})
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
		client.SendJSON(ServerResponse{Type: "error", Message: "빈자리가 없습니다."})
		return
	}

	g.players[emptySlot] = client
	g.hearts[emptySlot] = indianStartHearts

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🪑 [%s]님이 빈자리에 참여했습니다.", client.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	total := 0
	ready := 0
	for i := range g.players {
		if g.players[i] != nil {
			total++
			if g.startReady[i] {
				ready++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToBothLocked()
	log.Printf("[INDIAN] room:[%s] [%s] 빈자리 참여 (슬롯 %d)", g.room.ID, client.UserID, emptySlot)
}
