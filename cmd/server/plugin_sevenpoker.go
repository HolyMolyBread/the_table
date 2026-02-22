package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

const (
	sevenPokerMaxPlayers    = 4
	sevenPokerStartStars    = 10
	sevenPokerCheckCost     = 1
	sevenPokerCards         = 7
	sevenPokerTurnTimeLimit = 15
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// SevenPokerPlayerInfo는 한 플레이어의 공개 정보입니다.
type SevenPokerPlayerInfo struct {
	UserID   string `json:"userId"`
	Stars    int    `json:"stars"`
	Status   string `json:"status"`   // "check" | "fold" | ""
	Cards    []Card `json:"cards"`    // 본인은 앞면, 타인은 Hidden=true
	IsActive bool   `json:"isActive"` // 이번 라운드 생존
}

// SevenPokerData는 sevenpoker_state 응답의 data 필드입니다.
type SevenPokerData struct {
	Phase         string                 `json:"phase"`
	Round         int                    `json:"round"`
	Pot           int                    `json:"pot"`
	Players       []SevenPokerPlayerInfo `json:"players"`
	CurrentTurn   string                 `json:"currentTurn"`
	Message       string                 `json:"message,omitempty"`
}

// SevenPokerStateResponse는 세븐 포커 게임 상태 응답입니다.
type SevenPokerStateResponse struct {
	Type   string          `json:"type"`
	RoomID string          `json:"roomId"`
	Data   SevenPokerData  `json:"data"`
}

// ── SevenPokerGame 플러그인 ───────────────────────────────────────────────────

// SevenPokerGame은 별(⭐) 서바이벌 룰의 세븐 포커 플러그인입니다.
// 커뮤니티 카드 없이 각 플레이어가 7장을 받아 3~7구 분배 방식으로 진행합니다.
type SevenPokerGame struct {
	room             *Room
	players          [sevenPokerMaxPlayers]*Client
	stars            [sevenPokerMaxPlayers]int
	cards            [sevenPokerMaxPlayers][sevenPokerCards]Card
	deck             []Card
	pot              int
	potCarryOver     int
	phase            string
	round            int
	foldedThisRound  [sevenPokerMaxPlayers]bool
	actedThisPhase   [sevenPokerMaxPlayers]bool
	currentPlayerIdx int
	gameStarted      bool
	playerCount      int
	rematchReady     map[*Client]bool
	stopTick         chan struct{}
	mu               sync.Mutex
}

// NewSevenPokerGame creates a new Seven Poker game plugin.
func NewSevenPokerGame(room *Room) *SevenPokerGame {
	return &SevenPokerGame{room: room, phase: "waiting", rematchReady: make(map[*Client]bool)}
}

func (g *SevenPokerGame) Name() string { return "sevenpoker" }

// OnJoin은 플레이어 입장 시 호출됩니다.
func (g *SevenPokerGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == client {
			g.sendStateToAllLocked()
			return
		}
	}

	slot := -1
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.sendStateToSpectatorLocked(client)
		return
	}

	g.players[slot] = client
	g.stars[slot] = sevenPokerStartStars
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🃏 [%s]님이 입장했습니다. (%d/4)", client.UserID, g.playerCount),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	if g.playerCount >= 2 && !g.gameStarted {
		g.gameStarted = true
		g.startRoundLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

// OnLeave는 플레이어 퇴장 시 호출됩니다.
func (g *SevenPokerGame) OnLeave(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == client {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	delete(g.rematchReady, client)

	if g.gameStarted {
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if i == idx {
				client.RecordResult("sevenpoker", "lose")
			} else if g.players[i] != nil && g.stars[i] > 0 {
				g.players[i].RecordResult("sevenpoker", "win")
			}
		}
		msg := fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료.", client.UserID)
		data, _ := json.Marshal(GameResultResponse{
			Type:    "game_result",
			Message: msg,
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(data)
	}

	g.resetForLeaveLocked(idx)
}

// HandleAction은 game_action 메시지를 처리합니다.
func (g *SevenPokerGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "check":
		g.handleCheck(client)
	case "fold":
		g.handleFold(client)
	case "rematch":
		g.handleRematch(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 세븐 포커 명령: [%s]", p.Cmd),
		})
	}
}

func (g *SevenPokerGame) handleCheck(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 아직 시작되지 않았습니다."})
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	if g.players[g.currentPlayerIdx] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "내 차례가 아닙니다."})
		return
	}
	if g.foldedThisRound[idx] {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 폴드했습니다."})
		return
	}

	cost := 0
	if g.stars[idx] > 0 {
		cost = sevenPokerCheckCost
		g.stars[idx] -= cost
		g.pot += cost
	}

	g.actedThisPhase[idx] = true
	g.stopTurnTimerLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] ✅ 체크 (⭐ %d → 팟)", client.UserID, cost),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.advanceTurnLocked()
}

func (g *SevenPokerGame) handleFold(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 아직 시작되지 않았습니다."})
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	if g.players[g.currentPlayerIdx] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "내 차례가 아닙니다."})
		return
	}
	if g.foldedThisRound[idx] {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 폴드했습니다."})
		return
	}

	g.foldedThisRound[idx] = true
	g.actedThisPhase[idx] = true
	g.stopTurnTimerLocked()

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] 🏳️ 폴드", client.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.advanceTurnLocked()
}

func (g *SevenPokerGame) playerIndex(c *Client) int {
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == c {
			return i
		}
	}
	return -1
}

func (g *SevenPokerGame) advanceTurnLocked() {
	nextIdx := -1
	for i := 1; i <= sevenPokerMaxPlayers; i++ {
		idx := (g.currentPlayerIdx + i) % sevenPokerMaxPlayers
		if g.players[idx] == nil || g.foldedThisRound[idx] {
			continue
		}
		if !g.actedThisPhase[idx] {
			nextIdx = idx
			break
		}
	}

	if nextIdx >= 0 {
		g.currentPlayerIdx = nextIdx
		g.startTurnTimerLocked()
		g.sendStateToAllLocked()
		return
	}

	g.nextPhaseLocked()
}

func (g *SevenPokerGame) nextPhaseLocked() {
	switch g.phase {
	case "deal3":
		g.dealOneLocked(3, false) // deal4: 4번째 카드, visible
		g.phase = "deal4"
		g.resetActedAndAdvanceLocked("── 4구 (4번째 카드) ──")

	case "deal4":
		g.dealOneLocked(4, false)
		g.phase = "deal5"
		g.resetActedAndAdvanceLocked("── 5구 (5번째 카드) ──")

	case "deal5":
		g.dealOneLocked(5, false)
		g.phase = "deal6"
		g.resetActedAndAdvanceLocked("── 6구 (6번째 카드) ──")

	case "deal6":
		g.dealOneLocked(6, true) // 7번째 카드는 히든
		g.phase = "deal7"
		g.resetActedAndAdvanceLocked("── 7구 (7번째 카드, 히든) ──")

	case "deal7":
		g.phase = "showdown"
		g.resolveShowdownLocked()
		return

	default:
		g.sendStateToAllLocked()
		return
	}
	g.sendStateToAllLocked()
}

func (g *SevenPokerGame) dealOneLocked(cardIdx int, hidden bool) {
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] && g.stars[i] > 0 {
			g.cards[i][cardIdx] = g.deck[0]
			g.cards[i][cardIdx].Hidden = hidden
			g.deck = g.deck[1:]
		}
	}
}

func (g *SevenPokerGame) resetActedAndAdvanceLocked(msg string) {
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.actedThisPhase[i] = false
	}
	g.setFirstActivePlayerLocked()
	g.startTurnTimerLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: msg,
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
}

func (g *SevenPokerGame) setFirstActivePlayerLocked() {
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			g.currentPlayerIdx = i
			return
		}
	}
}

func (g *SevenPokerGame) resolveShowdownLocked() {
	survivors := make([]int, 0)
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			survivors = append(survivors, i)
		}
	}

	totalPot := g.pot + g.potCarryOver
	g.pot = 0
	g.potCarryOver = 0

	if len(survivors) == 0 {
		g.startRoundLocked()
		return
	}

	if len(survivors) == 1 {
		idx := survivors[0]
		g.stars[idx] += totalPot
		g.stopTurnTimerLocked()
		showdownData, _ := json.Marshal(map[string]any{
			"type": "poker_showdown_result",
			"roomId": g.room.ID,
			"data": map[string]any{
				"winnerId":    g.players[idx].UserID,
				"winningHand": "단독생존",
				"participants": []PokerShowdownParticipant{{UserID: g.players[idx].UserID, HandName: "단독생존"}},
			},
		})
		g.room.broadcastAll(showdownData)
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("🏆 [%s] 단독 생존! 팟 ⭐×%d 획득!", g.players[idx].UserID, totalPot),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.afterRoundLocked()
		return
	}

	g.stopTurnTimerLocked()

	// evaluateHand 사용 (plugin_holdem.go와 동일)
	cards7 := make([][]Card, len(survivors))
	scores := make([]int64, len(survivors))
	for i, idx := range survivors {
		cards7[i] = make([]Card, sevenPokerCards)
		for j := 0; j < sevenPokerCards; j++ {
			cards7[i][j] = g.cards[idx][j]
			cards7[i][j].Hidden = false // 쇼다운 시 모두 공개
		}
		scores[i] = EvaluateHand(cards7[i])
	}

	bestScore := int64(0)
	for _, s := range scores {
		if s > bestScore {
			bestScore = s
		}
	}
	winners := make([]int, 0)
	for i, s := range scores {
		if s == bestScore {
			winners = append(winners, survivors[i])
		}
	}

	// 한국 세븐 포커 룰: 동점 시 7장 중 가장 높은 카드(숫자+문양)로 단독 승자 결정, 팟 독식
	if len(winners) > 1 {
		idxToCards := make(map[int]int)
		for i, idx := range survivors {
			idxToCards[idx] = i
		}
		singleWinner := winners[0]
		bestRank, bestSuit := 0, 0
		for _, idx := range winners {
			cards := cards7[idxToCards[idx]]
			playerBestR, playerBestS := 0, 0
			for _, c := range cards {
				r, s := cardRank(c), suitRank(c)
				if r > playerBestR || (r == playerBestR && s > playerBestS) {
					playerBestR, playerBestS = r, s
				}
			}
			if playerBestR > bestRank || (playerBestR == bestRank && playerBestS > bestSuit) {
				bestRank, bestSuit = playerBestR, playerBestS
				singleWinner = idx
			}
		}
		winners = []int{singleWinner}
	}

	// 단독 승자에게 팟 전액 지급
	winnerIdx := winners[0]
	g.stars[winnerIdx] += totalPot

	winningHandName := HandRankName(bestScore)
	participants := make([]PokerShowdownParticipant, len(survivors))
	for i, idx := range survivors {
		participants[i] = PokerShowdownParticipant{
			UserID:   g.players[idx].UserID,
			HandName: HandRankName(scores[i]),
		}
	}
	showdownData, _ := json.Marshal(map[string]any{
		"type":   "poker_showdown_result",
		"roomId": g.room.ID,
		"data": map[string]any{
			"winnerId":     g.players[winnerIdx].UserID,
			"winningHand":  winningHandName,
			"participants": participants,
		},
	})
	g.room.broadcastAll(showdownData)

	msg := fmt.Sprintf("🏆 [%s] 승리! 팟 ⭐×%d 독식!", g.players[winnerIdx].UserID, totalPot)
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: msg,
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.afterRoundLocked()
}

func (g *SevenPokerGame) afterRoundLocked() {
	bankruptCount := 0
	totalCount := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil {
			totalCount++
			if g.stars[i] <= 0 {
				bankruptCount++
			}
		}
	}

	threshold := int(math.Ceil(float64(totalCount) / 2))
	if bankruptCount >= threshold {
		g.endMatchLocked()
		return
	}

	g.startRoundLocked()
}

func (g *SevenPokerGame) endMatchLocked() {
	g.stopTurnTimerLocked()
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		if g.stars[i] > 0 {
			g.players[i].RecordResult("sevenpoker", "win")
		} else {
			g.players[i].RecordResult("sevenpoker", "lose")
		}
	}

	survivors := ""
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			if survivors != "" {
				survivors += ", "
			}
			survivors += g.players[i].UserID
		}
	}
	msg := fmt.Sprintf("🏆 매치 종료! 생존자 [%s] 승리!", survivors)
	g.room.mu.RLock()
	totalCount := len(g.room.clients)
	g.room.mu.RUnlock()
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		Data:           map[string]any{"totalCount": totalCount},
		RematchEnabled: true,
	})
	g.room.broadcastAll(data)
	log.Printf("[SEVENPOKER] room:[%s] 매치 종료", g.room.ID)

	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.stars[i] = 0
		g.cards[i] = [sevenPokerCards]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
	}
	g.deck = nil
}

func (g *SevenPokerGame) startRoundLocked() {
	activeCount := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			activeCount++
		}
	}
	if activeCount < 2 {
		g.sendStateToAllLocked()
		return
	}

	g.round++
	g.phase = "deal3"
	g.pot += g.potCarryOver
	g.potCarryOver = 0

	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.foldedThisRound[i] = g.players[i] == nil || g.stars[i] <= 0
		g.actedThisPhase[i] = false
	}

	g.deck = NewShuffledDeck()
	cardIdx := 0

	// deal3: 3장 분배 (0,1 Hidden, 2 visible)
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			for j := 0; j < 3; j++ {
				g.cards[i][j] = g.deck[cardIdx]
				g.cards[i][j].Hidden = (j < 2) // 0,1 히든, 2 공개
				cardIdx++
			}
		}
	}
	g.deck = g.deck[cardIdx:]

	g.setFirstActivePlayerLocked()
	g.startTurnTimerLocked()

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("── 라운드 %d 시작! 3구 (3장 분배, 2장 히든) ──", g.round),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.sendStateToAllLocked()
}

func (g *SevenPokerGame) startTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	currentPlayer := g.players[g.currentPlayerIdx]
	room := g.room
	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  currentPlayer.UserID,
		Remaining: sevenPokerTurnTimeLimit,
	})
	g.room.broadcastAll(data)
	go func() {
		remaining := sevenPokerTurnTimeLimit
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

func (g *SevenPokerGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *SevenPokerGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted || g.players[g.currentPlayerIdx] != timedOutPlayer {
		return
	}
	idx := g.playerIndex(timedOutPlayer)
	if idx < 0 || g.foldedThisRound[idx] {
		return
	}
	g.foldedThisRound[idx] = true
	g.actedThisPhase[idx] = true
	g.stopTurnTimerLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("⏰ [%s] 시간 초과! 폴드 처리.", timedOutPlayer.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.advanceTurnLocked()
}

func (g *SevenPokerGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}
	g.rematchReady[client] = true
	total := 0
	ready := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.rematchReady[g.players[i]] {
				ready++
			}
		}
	}
	upd, _ := json.Marshal(RematchUpdateMessage{
		Type:       "rematch_update",
		RoomID:     g.room.ID,
		ReadyCount: ready,
		TotalCount: total,
	})
	g.room.broadcastAll(upd)
	if ready == total && total > 1 {
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] != nil {
				g.stars[i] = sevenPokerStartStars
				g.rematchReady[g.players[i]] = false
			}
		}
		g.pot = 0
		g.potCarryOver = 0
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			g.cards[i] = [sevenPokerCards]Card{}
			g.foldedThisRound[i] = false
			g.actedThisPhase[i] = false
		}
		g.gameStarted = true
		g.startRoundLocked()
	}
}

func (g *SevenPokerGame) resetForLeaveLocked(leaveIdx int) {
	g.stopTurnTimerLocked()
	g.players[leaveIdx] = nil
	g.stars[leaveIdx] = 0
	g.playerCount--
	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	g.deck = nil
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.cards[i] = [sevenPokerCards]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
	}
}

// ── 상태 전송 ─────────────────────────────────────────────────────────────────

func (g *SevenPokerGame) sendStateToAllLocked() {
	g.room.mu.RLock()
	clients := make([]*Client, 0, len(g.room.clients))
	for c := range g.room.clients {
		clients = append(clients, c)
	}
	g.room.mu.RUnlock()

	for _, client := range clients {
		idx := g.playerIndex(client)
		if idx >= 0 {
			g.sendStateToPlayerLocked(client, idx)
		} else {
			g.sendStateToSpectatorLocked(client)
		}
	}
}

func (g *SevenPokerGame) buildSevenPokerDataForPlayer(viewerIdx int) SevenPokerData {
	phase := g.phase
	if phase == "" {
		phase = "waiting"
	}

	players := make([]SevenPokerPlayerInfo, 0)
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}

		status := ""
		if g.foldedThisRound[i] {
			status = "fold"
		} else if g.actedThisPhase[i] {
			status = "check"
		}

		// 카드 수는 현재 페이즈에 따라 다름
		cardCount := 0
		switch g.phase {
		case "deal3":
			cardCount = 3
		case "deal4":
			cardCount = 4
		case "deal5":
			cardCount = 5
		case "deal6":
			cardCount = 6
		case "deal7", "showdown":
			cardCount = 7
		}

		cards := make([]Card, cardCount)
		for j := 0; j < cardCount; j++ {
			cards[j] = g.cards[i][j]
			if i != viewerIdx {
				cards[j].Hidden = true // 타인 카드는 뒷면
			}
		}

		players = append(players, SevenPokerPlayerInfo{
			UserID:   g.players[i].UserID,
			Stars:    g.stars[i],
			Status:   status,
			Cards:    cards,
			IsActive: !g.foldedThisRound[i],
		})
	}

	currentTurn := ""
	if g.players[g.currentPlayerIdx] != nil {
		currentTurn = g.players[g.currentPlayerIdx].UserID
	}

	return SevenPokerData{
		Phase:       phase,
		Round:       g.round,
		Pot:         g.pot + g.potCarryOver,
		Players:     players,
		CurrentTurn: currentTurn,
	}
}

func (g *SevenPokerGame) sendStateToPlayerLocked(client *Client, playerIdx int) {
	data := g.buildSevenPokerDataForPlayer(playerIdx)
	client.SendJSON(SevenPokerStateResponse{
		Type:   "sevenpoker_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *SevenPokerGame) sendStateToSpectatorLocked(client *Client) {
	data := g.buildSevenPokerDataForPlayer(-1)
	client.SendJSON(SevenPokerStateResponse{
		Type:   "sevenpoker_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}
