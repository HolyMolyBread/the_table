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
	sevenPokerMaxPlayers      = 4
	sevenPokerStartStars      = 10
	sevenPokerCheckCost       = 1
	sevenPokerCards           = 7
	sevenPokerTurnTimeLimit   = 20
	sevenPokerChoiceTimeLimit = 20
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// SevenPokerPlayerInfo는 한 플레이어의 공개 정보입니다.
type SevenPokerPlayerInfo struct {
	UserID   string `json:"userId"`
	Stars    int    `json:"stars"`
	Status   string `json:"status"`   // "check" | "fold" | ""
	Cards    []Card `json:"cards"`    // 본인은 앞면, 타인은 Hidden=true
	IsActive bool   `json:"isActive"` // 이번 라운드 생존
	IsDealer bool   `json:"isDealer"` // 딜러(D) 버튼 표시
}

// SevenPokerData는 sevenpoker_state 응답의 data 필드입니다.
type SevenPokerData struct {
	Phase        string                 `json:"phase"`
	Round        int                    `json:"round"`
	Pot          int                    `json:"pot"`
	Players      []SevenPokerPlayerInfo `json:"players"`
	CurrentTurn  string                 `json:"currentTurn"`
	Message      string                 `json:"message,omitempty"`
	CanTakeover  bool                   `json:"canTakeover,omitempty"`
	MyHandName   string                 `json:"myHandName,omitempty"`
	MyChoiceDone bool                   `json:"myChoiceDone,omitempty"` // choice 페이즈에서 본인이 이미 결정했는지
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
	choiceDone       [sevenPokerMaxPlayers]bool
	dealerIdx        int
	currentPlayerIdx int
	gameStarted      bool
	playerCount      int
	startReady       map[*Client]bool
	rematchReady     map[*Client]bool
	stopTick         chan struct{}
	mu               sync.Mutex
}

// NewSevenPokerGame creates a new Seven Poker game plugin.
func NewSevenPokerGame(room *Room) *SevenPokerGame {
	return &SevenPokerGame{room: room, phase: "waiting", startReady: make(map[*Client]bool), rematchReady: make(map[*Client]bool)}
}

func init() { RegisterPlugin("sevenpoker", func(room *Room) GamePlugin { return NewSevenPokerGame(room) }) }

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

	if !g.gameStarted {
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: g.playerCount,
		})
		g.room.broadcastAll(upd)
		g.sendStateToAllLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

// OnLeave는 플레이어 퇴장 시 호출됩니다.
func (g *SevenPokerGame) OnLeave(client *Client, remainingCount int) {
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
	delete(g.startReady, client)
	delete(g.rematchReady, client)

	// 슬롯 비우기
	g.players[idx] = nil
	g.stars[idx] = 0
	g.playerCount--

	if !g.gameStarted {
		readyCount := 0
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] != nil && g.startReady[g.players[i]] {
				readyCount++
			}
		}
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: g.playerCount,
		})
		g.room.broadcastAll(upd)
		g.sendStateToAllLocked()
		return
	}

	// 퇴장자에게만 lose 전적 기록 (게임 진행 중이었을 때)
	client.RecordResult("sevenpoker", "lose")

	// [턴 넘김] 퇴장자가 현재 차례였다면 폴드 처리 후 턴 진행
	if g.currentPlayerIdx == idx {
		g.stopTurnTimerLocked()
		g.foldedThisRound[idx] = true
		g.actedThisPhase[idx] = true
		g.advanceTurnLocked()
	}

	// [생존자 체크] 별 1개 이상인 플레이어 수
	survivorCount := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] >= 1 {
			survivorCount++
		}
	}

	if survivorCount >= 2 {
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 퇴장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.sendStateToAllLocked()
		return
	}

	// 생존자 1명 이하 → 매치 종료
	if survivorCount == 1 {
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] != nil && g.stars[i] >= 1 {
				g.players[i].RecordResult("sevenpoker", "win")
				break
			}
		}
	}
	g.stopTurnTimerLocked()
	survivors := ""
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			if survivors != "" {
				survivors += ", "
			}
			survivors += g.players[i].UserID
		}
	}
	msg := fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료! 생존자 [%s] 승리!", client.UserID, survivors)
	if survivors == "" {
		msg = fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료.", client.UserID)
	}
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
	g.resetForLeaveLocked()
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
	case "choice":
		g.handleChoice(client, payload)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
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
		return
	}
	if g.phase == "choice" {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentPlayerIdx] == nil || g.players[g.currentPlayerIdx].UserID != client.UserID {
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		return
	}
	if g.foldedThisRound[idx] {
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

func (g *SevenPokerGame) handleChoice(client *Client, payload json.RawMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted || g.phase != "choice" {
		client.SendJSON(ServerResponse{Type: "error", Message: "초이스 단계가 아닙니다."})
		return
	}
	idx := g.playerIndex(client)
	if idx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	if g.foldedThisRound[idx] {
		client.SendJSON(ServerResponse{Type: "error", Message: "폴드한 상태에서는 초이스를 할 수 없습니다."})
		return
	}
	if g.choiceDone[idx] {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 초이스를 완료했습니다."})
		return
	}

	var p struct {
		DiscardIdx int `json:"discardIdx"`
		OpenIdx    int `json:"openIdx"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "초이스 페이로드 파싱 오류"})
		return
	}
	if p.DiscardIdx < 0 || p.DiscardIdx > 3 || p.OpenIdx < 0 || p.OpenIdx > 3 || p.DiscardIdx == p.OpenIdx {
		client.SendJSON(ServerResponse{Type: "error", Message: "discardIdx와 openIdx는 0~3 범위이며 서로 달라야 합니다."})
		return
	}

	// 4장 중 discardIdx 제외한 3장을 인덱스 0,1,2에 배치
	// 공개 카드는 UI 상 가장 오른쪽(배열 맨 끝, 인덱스 2)에 오도록 정렬
	kept := make([]Card, 0, 3)
	var openCard Card
	for j := 0; j < 4; j++ {
		if j == p.DiscardIdx {
			continue
		}
		if j == p.OpenIdx {
			openCard = g.cards[idx][j]
		} else {
			kept = append(kept, g.cards[idx][j])
		}
	}
	kept = append(kept, openCard) // 공개 카드를 맨 끝(오른쪽)에 배치

	for j := 0; j < 3; j++ {
		g.cards[idx][j] = kept[j]
		g.cards[idx][j].Hidden = (j != 2) // 인덱스 2 = 맨 끝 = 공개
	}
	for j := 3; j < sevenPokerCards; j++ {
		g.cards[idx][j] = Card{}
	}

	g.choiceDone[idx] = true

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] 초이스 완료 (1장 버림, 1장 공개)", client.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	allDone := true
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] && !g.choiceDone[i] {
			allDone = false
			break
		}
	}
	if allDone {
		g.stopTurnTimerLocked()
		g.phase = "bet3"
		g.resetActedAndAdvanceLocked("── 3구 베팅 ──")
		g.sendStateToAllLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

func (g *SevenPokerGame) handleFold(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	if g.phase == "choice" {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentPlayerIdx] == nil || g.players[g.currentPlayerIdx].UserID != client.UserID {
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
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
	// 단독 생존자 체크 (나머지 모두 폴드)
	activeCount := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			activeCount++
		}
	}
	if activeCount == 1 {
		g.resolveShowdownLocked()
		return
	}

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
	case "bet3":
		g.dealOneLocked(3, false) // 4번째 카드
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
	bestScore := -1
	bestIdx := -1
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			score := g.evaluateOpenCards(i)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
	}
	if bestIdx >= 0 {
		g.currentPlayerIdx = bestIdx
	} else {
		g.currentPlayerIdx = 0
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
		cards7[i] = make([]Card, 0, sevenPokerCards)
		for j := 0; j < sevenPokerCards; j++ {
			c := g.cards[idx][j]
			if c.Value == "" && c.Suit == "" {
				continue
			}
			c.Hidden = false
			cards7[i] = append(cards7[i], c)
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

	winningHandName := HandRankWithDetail(bestScore)
	winReason := HandWinReason(bestScore)
	participants := make([]PokerShowdownParticipant, len(survivors))
	for i, idx := range survivors {
		p := PokerShowdownParticipant{
			UserID:   g.players[idx].UserID,
			HandName: HandRankWithDetail(scores[i]),
			WinReason: HandWinReason(scores[i]),
		}
		if int(bestScore>>20) == 1 {
			p.HighCardHighlightIdx = EvaluateHandHighCardIdx(cards7[i])
		}
		participants[i] = p
	}
	showdownData, _ := json.Marshal(map[string]any{
		"type":   "poker_showdown_result",
		"roomId": g.room.ID,
		"data": map[string]any{
			"winnerId":     g.players[winnerIdx].UserID,
			"winningHand":  winningHandName,
			"winReason":    winReason,
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
	g.dealerIdx = 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.stars[i] = 0
		g.cards[i] = [sevenPokerCards]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
		g.choiceDone[i] = false
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

	// 딜러 버튼 이동 (다음 생존 플레이어)
	for i := 1; i <= sevenPokerMaxPlayers; i++ {
		idx := (g.dealerIdx + i) % sevenPokerMaxPlayers
		if g.players[idx] != nil && g.stars[idx] > 0 {
			g.dealerIdx = idx
			break
		}
	}

	g.round++
	g.phase = "choice"
	g.pot += g.potCarryOver
	g.potCarryOver = 0

	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.cards[i] = [sevenPokerCards]Card{} // 핵심: 지난 라운드의 남은 카드 완벽히 초기화
		g.foldedThisRound[i] = g.players[i] == nil || g.stars[i] <= 0
		g.actedThisPhase[i] = false
		g.choiceDone[i] = false
	}

	g.deck = NewShuffledDeck()
	cardIdx := 0

	// choice: 4장 분배 (모두 Hidden)
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			for j := 0; j < 4; j++ {
				g.cards[i][j] = g.deck[cardIdx]
				g.cards[i][j].Hidden = true
				cardIdx++
			}
		}
	}
	g.deck = g.deck[cardIdx:]

	g.setFirstActivePlayerLocked()
	g.startTurnTimerLocked()

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("── 라운드 %d 시작! 4장 초이스 (1장 버리고 1장 공개) ──", g.round),
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
	if currentPlayer == nil {
		return
	}
	room := g.room
	limit := sevenPokerTurnTimeLimit
	if g.phase == "choice" {
		limit = sevenPokerChoiceTimeLimit
	}
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

func (g *SevenPokerGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *SevenPokerGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted {
		return
	}

	// 초이스 페이즈 타임아웃: 전원 동시 진행이므로 currentPlayerIdx 검사 없이 전역 처리
	if g.phase == "choice" {
		g.stopTurnTimerLocked()
		for idx := 0; idx < sevenPokerMaxPlayers; idx++ {
			if g.players[idx] != nil && !g.foldedThisRound[idx] && !g.choiceDone[idx] {
				// 강제 초이스: 3번 버림, 2번 오픈
				discardIdx, openIdx := 3, 2
				kept := make([]Card, 0, 3)
				for j := 0; j < 4; j++ {
					if j != discardIdx {
						kept = append(kept, g.cards[idx][j])
					}
				}
				relOpenIdx := 0
				for j := 0; j < 4; j++ {
					if j == discardIdx {
						continue
					}
					if j == openIdx {
						break
					}
					relOpenIdx++
				}
				for j := 0; j < 3; j++ {
					g.cards[idx][j] = kept[j]
					g.cards[idx][j].Hidden = (j != relOpenIdx)
				}
				for j := 3; j < sevenPokerCards; j++ {
					g.cards[idx][j] = Card{}
				}
				g.choiceDone[idx] = true
			}
		}
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "⏰ 초이스 시간 초과! 미완료 플레이어 자동 선택 진행",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

		g.phase = "bet3"
		g.resetActedAndAdvanceLocked("── 3구 베팅 ──")
		g.sendStateToAllLocked()
		return
	}

	if g.players[g.currentPlayerIdx] != timedOutPlayer {
		return
	}
	idx := g.playerIndex(timedOutPlayer)
	if idx < 0 || g.foldedThisRound[idx] {
		return
	}
	g.stopTurnTimerLocked()

	g.foldedThisRound[idx] = true
	g.actedThisPhase[idx] = true
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
		g.rematchReady = make(map[*Client]bool)
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] != nil {
				g.stars[i] = sevenPokerStartStars
			}
		}
		g.pot = 0
		g.potCarryOver = 0
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			g.cards[i] = [sevenPokerCards]Card{}
			g.foldedThisRound[i] = false
			g.actedThisPhase[i] = false
			g.choiceDone[i] = false
		}
		g.gameStarted = true
		g.startRoundLocked()
	}
}

// handleReady는 게임 시작 전 준비 요청을 처리합니다.
func (g *SevenPokerGame) handleReady(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 이미 시작되었습니다."})
		return
	}
	// waiting 상태에서는 playerIndex 검사 생략: UserID로 플레이어 매칭 후 ready 등록
	var readyClient *Client
	if g.phase == "waiting" {
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] != nil && g.players[i].UserID == client.UserID {
				readyClient = g.players[i]
				break
			}
		}
		if readyClient == nil {
			return // 관전자는 무시
		}
	} else {
		idx := g.playerIndex(client)
		if idx < 0 {
			client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
			return
		}
		readyClient = client
	}
	g.startReady[readyClient] = true
	total := 0
	ready := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				ready++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	if ready == total && total >= 2 {
		g.startReady = make(map[*Client]bool)
		g.gameStarted = true
		g.startRoundLocked()
	}
}

// resetForLeaveLocked는 매치가 인원 부족으로 종료되었을 때만 호출됩니다.
// 방의 모든 게임 상태를 대기(waiting) 상태로 초기화합니다.
func (g *SevenPokerGame) resetForLeaveLocked() {
	g.stopTurnTimerLocked()
	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	g.dealerIdx = 0
	g.deck = nil
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		g.cards[i] = [sevenPokerCards]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
		g.choiceDone[i] = false
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

		// 실제 존재하는 카드만 append (폴드한 유저는 카드 증식 방지)
		cards := make([]Card, 0, sevenPokerCards)
		for j := 0; j < sevenPokerCards; j++ {
			c := g.cards[i][j]
			if c.Value == "" && c.Suit == "" {
				continue
			}
			if i != viewerIdx {
				if c.Hidden {
					c.Suit = ""
					c.Value = "" // 타인 뒷면 카드는 클라이언트 조작 방지를 위해 빈 값 전송
				}
			}
			// 본인(i == viewerIdx)일 경우 Hidden 원본 값 그대로 전송 (프론트에서 히든/공개 구분)
			cards = append(cards, c)
		}

		players = append(players, SevenPokerPlayerInfo{
			UserID:   g.players[i].UserID,
			Stars:    g.stars[i],
			Status:   status,
			Cards:    cards,
			IsActive: !g.foldedThisRound[i],
			IsDealer: i == g.dealerIdx,
		})
	}

	currentTurn := ""
	if g.players[g.currentPlayerIdx] != nil {
		currentTurn = g.players[g.currentPlayerIdx].UserID
	}

	canTakeover := false
	if viewerIdx < 0 && phase == "waiting" && !g.gameStarted {
		for i := 0; i < sevenPokerMaxPlayers; i++ {
			if g.players[i] == nil {
				canTakeover = true
				break
			}
		}
	}

	myHandName := ""
	if viewerIdx >= 0 && !g.foldedThisRound[viewerIdx] {
		cards7 := make([]Card, 0, sevenPokerCards)
		for j := 0; j < sevenPokerCards; j++ {
			c := g.cards[viewerIdx][j]
			if c.Suit != "" || c.Value != "" {
				cards7 = append(cards7, c)
			}
		}
		if len(cards7) >= 5 {
			score := EvaluateHand(cards7)
			myHandName = PokerHandDisplayName(HandRankName(score))
		}
	}

	myChoiceDone := false
	if viewerIdx >= 0 {
		myChoiceDone = g.choiceDone[viewerIdx]
	}

	return SevenPokerData{
		Phase:        phase,
		Round:        g.round,
		Pot:          g.pot + g.potCarryOver,
		Players:      players,
		CurrentTurn:  currentTurn,
		CanTakeover:  canTakeover,
		MyHandName:   myHandName,
		MyChoiceDone: myChoiceDone,
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

func (g *SevenPokerGame) handleTakeover(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 빈자리 참여가 불가합니다."})
		return
	}
	if g.playerIndex(client) >= 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 플레이어입니다."})
		return
	}
	slot := -1
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "빈자리가 없습니다."})
		return
	}

	g.players[slot] = client
	g.stars[slot] = sevenPokerStartStars
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🪑 [%s]님이 빈자리에 참여했습니다. (%d/4)", client.UserID, g.playerCount),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	total := g.playerCount
	ready := 0
	for i := 0; i < sevenPokerMaxPlayers; i++ {
		if g.players[i] != nil && g.startReady[g.players[i]] {
			ready++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
	log.Printf("[SEVENPOKER] room:[%s] [%s] 빈자리 참여 (슬롯 %d)", g.room.ID, client.UserID, slot)
}

// evaluateOpenCards는 플레이어의 오픈(앞면) 카드들만으로 족보 점수를 계산합니다.
// 액면가 보스 룰: 가장 높은 점수의 플레이어가 선(Boss)이 됩니다.
func (g *SevenPokerGame) evaluateOpenCards(playerIdx int) int {
	counts := make(map[int]int)
	var cards []Card
	for j := 0; j < sevenPokerCards; j++ {
		c := g.cards[playerIdx][j]
		if c.Value != "" && !c.Hidden {
			cards = append(cards, c)
			counts[cardRank(c)]++
		}
	}
	if len(cards) == 0 {
		return 0
	}
	maxFreq, freqRank, highRank, highSuit := 0, 0, 0, 0
	for r, count := range counts {
		if count > maxFreq || (count == maxFreq && r > freqRank) {
			maxFreq = count
			freqRank = r
		}
	}
	for _, c := range cards {
		r, s := cardRank(c), suitRank(c)
		if r > highRank || (r == highRank && s > highSuit) {
			highRank = r
			highSuit = s
		}
	}
	if maxFreq == 4 {
		return 40000 + freqRank*100
	}
	if maxFreq == 3 {
		return 30000 + freqRank*100
	}
	if maxFreq == 2 {
		return 20000 + freqRank*100 + highSuit
	}
	return 10000 + highRank*10 + highSuit
}
