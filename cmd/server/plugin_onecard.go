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
	oneCardMaxPlayers     = 4
	oneCardHandSize       = 7
	oneCardTurnTimeLimit  = 15
	oneCardBankruptcyLimit = 20
)

var (
	oneCardBJoker = Card{Suit: "🃏", Value: "B_JOKER"}
	oneCardCJoker = Card{Suit: "🃏", Value: "C_JOKER"}
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// OneCardData는 onecard_state 응답의 data 필드입니다.
type OneCardData struct {
	Hand            []Card `json:"hand"`
	TopCard         Card   `json:"topCard"`
	TargetSuit      string `json:"targetSuit,omitempty"`      // 7 카드로 강제된 문양
	DeckCount       int    `json:"deckCount"`
	DiscardCount    int    `json:"discardCount"`
	Turn            string `json:"turn"`
	Direction       int    `json:"direction"`
	AttackPenalty   int    `json:"attackPenalty"`
	OneCardVulnerable string `json:"oneCardVulnerable,omitempty"` // 손패 1장인 유저 (원카드 콜 대상)
	Players         []OneCardPlayerInfo `json:"players"`
	Message         string `json:"message,omitempty"`
	CanTakeover     bool   `json:"canTakeover,omitempty"`
}

// OneCardPlayerInfo는 한 플레이어의 공개 정보입니다.
type OneCardPlayerInfo struct {
	UserID    string `json:"userId"`
	CardCount int    `json:"cardCount"`
	IsTurn    bool   `json:"isTurn"`
	Status    string `json:"status,omitempty"` // "escaped" | "bankrupt"
}

// OneCardStateResponse는 원카드 게임 상태 응답입니다.
type OneCardStateResponse struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId"`
	Data   OneCardData `json:"data"`
}

// ── OneCardGame 플러그인 ──────────────────────────────────────────────────────

type OneCardGame struct {
	room             *Room
	players          [oneCardMaxPlayers]*Client
	hands            [oneCardMaxPlayers][]Card
	deck             []Card
	discardPile      []Card
	topCard          Card
	targetSuit       string // 7 카드로 강제된 문양 (빈 문자열이면 topCard 기준)
	direction        int
	currentTurn      int
	skipNext         bool
	attackPenalty    int
	playAgain        bool   // K 카드로 인한 연속 턴
	oneCardVulnerable string   // 손패 1장인 유저 ID
	playerCount      int
	winners          []string // 탈출 성공 (손패 0장)
	losers           []string // 파산 (20장 초과)
	initialPlayerCount int    // 게임 시작 시 플레이어 수 (종료 조건용)
	gameStarted      bool
	stopTick         chan struct{}
	startReady       map[*Client]bool
	rematchReady     map[*Client]bool
	mu               sync.Mutex
}

func NewOneCardGame(room *Room) *OneCardGame {
	return &OneCardGame{room: room, direction: 1, startReady: make(map[*Client]bool), rematchReady: make(map[*Client]bool)}
}

func init() { RegisterPlugin("onecard", func(room *Room) GamePlugin { return NewOneCardGame(room) }) }

func (g *OneCardGame) Name() string { return "onecard" }

// OnJoin
func (g *OneCardGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 재접속: 동일 UserID의 기존 플레이어가 있으면 포인터만 최신화 후 상태 전송
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			oldClient := g.players[i]
			g.players[i] = client
			if g.startReady[oldClient] {
				g.startReady[client] = true
				delete(g.startReady, oldClient)
			}
			if g.rematchReady[oldClient] {
				g.rematchReady[client] = true
				delete(g.rematchReady, oldClient)
			}
			g.sendStateToClientLocked(client)
			return
		}
	}

	slot := -1
	for i := 0; i < oneCardMaxPlayers; i++ {
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
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🃏 [%s]님이 입장했습니다. (%d/%d)", client.UserID, g.playerCount, oneCardMaxPlayers),
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

// OnLeave
func (g *OneCardGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	delete(g.startReady, client)
	delete(g.rematchReady, client)

	g.players[idx] = nil
	g.hands[idx] = nil
	g.playerCount--
	remaining := g.playerCount

	if !g.gameStarted {
		readyCount := 0
		for i := 0; i < oneCardMaxPlayers; i++ {
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

	g.losers = append(g.losers, client.UserID)
	client.RecordResult("onecard", "lose")
	if g.checkGameEndLocked() {
		g.room.mu.RLock()
		totalCount := len(g.room.clients)
		g.room.mu.RUnlock()
		msg := fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료.", client.UserID)
		data, _ := json.Marshal(GameResultResponse{
			Type:           "game_result",
			Message:        msg,
			RoomID:         g.room.ID,
			Data:           map[string]any{"totalCount": totalCount},
			RematchEnabled: true,
		})
		g.room.broadcastAll(data)
		g.gameStarted = false
		g.startReady = make(map[*Client]bool)
		g.rematchReady = make(map[*Client]bool)
		g.stopTurnTimerLocked()
		log.Printf("[ONECARD] room:[%s] [%s] 퇴장 — 매치 종료 (losers 추가)", g.room.ID, client.UserID)
		return
	}

	if idx == g.currentTurn {
		g.stopTurnTimerLocked()
		g.advanceTurnLocked()
	}

	survivorCount := 0
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && len(g.hands[i]) > 0 {
			survivorCount++
		}
	}

	if remaining >= 2 && survivorCount >= 2 {
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 퇴장했습니다. 게임 계속!", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.startTurnTimerLocked()
		g.sendStateToAllLocked()
		return
	}

	g.stopTurnTimerLocked()
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && len(g.hands[i]) == 0 {
			g.players[i].RecordResult("onecard", "win")
		}
	}
	g.room.mu.RLock()
	totalCount := len(g.room.clients)
	g.room.mu.RUnlock()
	msg := fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료.", client.UserID)
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		Data:           map[string]any{"totalCount": totalCount},
		RematchEnabled: true,
	})
	g.room.broadcastAll(data)
	g.gameStarted = false
	g.startReady = make(map[*Client]bool)
	g.rematchReady = make(map[*Client]bool)
	log.Printf("[ONECARD] room:[%s] [%s] 퇴장 — 매치 종료 (생존자 부족)", g.room.ID, client.UserID)
}

// HandleAction
func (g *OneCardGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd         string `json:"cmd"`
		Index       int    `json:"index"`
		TargetSuit  string `json:"targetSuit"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "play":
		g.handlePlay(client, p.Index, p.TargetSuit)
	case "draw":
		g.handleDraw(client)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	case "call_onecard":
		g.handleCallOneCard(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 원카드 명령: [%s]", p.Cmd),
		})
	}
}

func (g *OneCardGame) replenishDeckLocked() {
	if len(g.deck) > 0 {
		return
	}
	if len(g.discardPile) < 2 {
		return
	}
	// 탑 카드 제외하고 나머지를 셔플하여 덱으로
	top := g.topCard
	newDeck := make([]Card, 0, len(g.discardPile))
	for _, c := range g.discardPile {
		newDeck = append(newDeck, c)
	}
	g.discardPile = nil
	rand.Shuffle(len(newDeck), func(i, j int) { newDeck[i], newDeck[j] = newDeck[j], newDeck[i] })
	g.deck = newDeck
	g.topCard = top
}

func (g *OneCardGame) drawFromDeckLocked() (Card, bool) {
	g.replenishDeckLocked()
	if len(g.deck) == 0 {
		return Card{}, false
	}
	c := g.deck[len(g.deck)-1]
	g.deck = g.deck[:len(g.deck)-1]
	return c, true
}

func (g *OneCardGame) discardLocked(c Card) {
	g.discardPile = append(g.discardPile, c)
}

// isAttackCard returns penalty amount (0 = not attack)
func isAttackCard(c Card) int {
	switch c.Value {
	case "A":
		return 3
	case "B_JOKER":
		return 5
	case "C_JOKER":
		return 7
	}
	return 0
}

// canDefend returns true if defCard can defend against attackCard
func canDefend(attackCard, defCard Card) bool {
	switch attackCard.Value {
	case "A":
		return defCard.Value == "A" || defCard.Value == "B_JOKER" || defCard.Value == "C_JOKER"
	case "B_JOKER":
		return defCard.Value == "C_JOKER"
	case "C_JOKER":
		return false
	}
	return false
}

func (g *OneCardGame) canPlayNormal(c Card) bool {
	t := g.topCard
	suit := g.targetSuit
	if suit == "" {
		suit = t.Suit
	}
	// 조커 후속: 흑백 조커(B_JOKER) 위에는 검은색(♠, ♣), 컬러 조커(C_JOKER) 위에는 붉은색(♥, ♦) 가능
	if t.Value == "B_JOKER" {
		blackSuits := c.Suit == "♠" || c.Suit == "♣"
		return blackSuits || c.Value == "B_JOKER" || c.Value == "C_JOKER"
	}
	if t.Value == "C_JOKER" {
		redSuits := c.Suit == "♥" || c.Suit == "♦"
		return redSuits || c.Value == "B_JOKER" || c.Value == "C_JOKER"
	}
	return c.Suit == suit || c.Value == t.Value || c.Value == "B_JOKER" || c.Value == "C_JOKER"
}

func (g *OneCardGame) canPlay(c Card) bool {
	if g.attackPenalty > 0 {
		return canDefend(g.topCard, c)
	}
	return g.canPlayNormal(c)
}

func (g *OneCardGame) hasDefenseCard(idx int) bool {
	t := g.topCard
	for _, c := range g.hands[idx] {
		if canDefend(t, c) {
			return true
		}
	}
	return false
}

func (g *OneCardGame) handlePlay(client *Client, index int, targetSuit string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		return
	}
	g.stopTurnTimerLocked()
	if index < 0 || index >= len(g.hands[idx]) {
		client.SendJSON(ServerResponse{Type: "error", Message: "유효하지 않은 카드 인덱스입니다."})
		return
	}

	card := g.hands[idx][index]

	// 7 카드는 targetSuit 필수
	if card.Value == "7" {
		validSuits := map[string]bool{"♠": true, "♥": true, "♦": true, "♣": true}
		if targetSuit == "" || !validSuits[targetSuit] {
			client.SendJSON(ServerResponse{Type: "error", Message: "7 카드는 무늬(♠♥♦♣)를 선택해야 합니다."})
			return
		}
	}

	if !g.canPlay(card) {
		client.SendJSON(ServerResponse{Type: "error", Message: "낼 수 없는 카드입니다."})
		return
	}

	g.hands[idx] = append(g.hands[idx][:index], g.hands[idx][index+1:]...)
	oldTop := g.topCard
	g.discardLocked(g.topCard)
	g.topCard = card
	g.targetSuit = ""

	wasDefense := g.attackPenalty > 0 && canDefend(oldTop, card)
	penalty := isAttackCard(card)
	notice := ""

	if wasDefense {
		// 방어 성공: 기존 패널티에 내 공격 카드의 패널티를 더해서 다음 사람에게 넘김
		g.attackPenalty += penalty
		notice = fmt.Sprintf("[%s]가 %s%s로 방어! 공격 누적 +%d! (총 %d장)", client.UserID, card.Suit, card.Value, penalty, g.attackPenalty)
	} else if penalty > 0 {
		// 새로운 공격 시작
		g.attackPenalty += penalty
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 공격 +%d! (총 %d장)", client.UserID, card.Suit, card.Value, penalty, g.attackPenalty)
	} else if card.Value == "7" {
		g.targetSuit = targetSuit
		notice = fmt.Sprintf("[%s]가 7을 내서 다음 문양을 %s로 변경!", client.UserID, targetSuit)
	} else if card.Value == "J" {
		g.skipNext = true
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 다음 턴 스킵!", client.UserID, card.Suit, card.Value)
	} else if card.Value == "Q" {
		g.direction *= -1
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 방향 반전!", client.UserID, card.Suit, card.Value)
	} else if card.Value == "K" {
		g.playAgain = true
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 한 번 더!", client.UserID, card.Suit, card.Value)
	} else {
		notice = fmt.Sprintf("[%s]가 %s%s를 냈습니다.", client.UserID, card.Suit, card.Value)
	}

	// 원카드 취약 상태 갱신 (손패 1장인 사람이 없으면 비활성화)
	g.oneCardVulnerable = ""
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && len(g.hands[i]) == 1 {
			g.oneCardVulnerable = g.players[i].UserID
			break
		}
	}

	msg, _ := json.Marshal(ServerResponse{Type: "game_notice", Message: notice, RoomID: g.room.ID})
	g.room.broadcastAll(msg)

	// 탈출 체크 (손패 0장)
	if len(g.hands[idx]) == 0 {
		g.winners = append(g.winners, client.UserID)
		if g.checkGameEndLocked() {
			resultMsg := fmt.Sprintf("🏆 [%s] 승리!", client.UserID)
			g.room.mu.RLock()
			totalCount := len(g.room.clients)
			g.room.mu.RUnlock()
			data, _ := json.Marshal(GameResultResponse{
				Type:           "game_result",
				Message:        resultMsg,
				RoomID:         g.room.ID,
				Data:           map[string]any{"totalCount": totalCount},
				RematchEnabled: true,
			})
			g.room.broadcastAll(data)
			g.gameStarted = false
			g.stopTurnTimerLocked()
			return
		}
	}

	// K가 아니면 턴 진행
	if !g.playAgain {
		g.advanceTurnLocked()
	}
	g.playAgain = false
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *OneCardGame) handleDraw(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		return
	}
	g.stopTurnTimerLocked()

	if g.attackPenalty > 0 {
		// 공격 중: 방어 불가 → 패널티만큼 드로우
		count := g.attackPenalty
		g.attackPenalty = 0
		for i := 0; i < count; i++ {
			if c, ok := g.drawFromDeckLocked(); ok {
				g.hands[idx] = append(g.hands[idx], c)
			}
		}
		// 조커 위에 있을 때: 다음 플레이어가 낼 수 있는 문양을 targetSuit로 고정
		if g.topCard.Value == "B_JOKER" {
			g.targetSuit = "♠"
		} else if g.topCard.Value == "C_JOKER" {
			g.targetSuit = "♥"
		}
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]가 공격을 맞아 %d장 드로우!", client.UserID, count),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
	} else {
		c, ok := g.drawFromDeckLocked()
		if !ok {
			client.SendJSON(ServerResponse{Type: "error", Message: "덱에 카드가 없습니다."})
			return
		}
		g.hands[idx] = append(g.hands[idx], c)
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("🎴 [%s] 님이 카드를 1장 뽑았습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
	}

	// 원카드 취약 상태 갱신 (손패 1장인 사람이 없으면 비활성화)
	g.oneCardVulnerable = ""
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && len(g.hands[i]) == 1 {
			g.oneCardVulnerable = g.players[i].UserID
			break
		}
	}

	// 파산 체크 (20장 초과)
	if len(g.hands[idx]) > oneCardBankruptcyLimit {
		g.losers = append(g.losers, client.UserID)
		msg := fmt.Sprintf("💀 [%s] 파산! (손패 %d장 초과)", client.UserID, oneCardBankruptcyLimit)
		notice, _ := json.Marshal(ServerResponse{Type: "game_notice", Message: msg, RoomID: g.room.ID})
		g.room.broadcastAll(notice)
		if g.checkGameEndLocked() {
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
			g.gameStarted = false
			g.stopTurnTimerLocked()
			return
		}
	}

	g.advanceTurnLocked()
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *OneCardGame) handleCallOneCard(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted || g.oneCardVulnerable == "" {
		client.SendJSON(ServerResponse{Type: "error", Message: "원카드 콜할 대상이 없습니다."})
		return
	}

	if client.UserID == g.oneCardVulnerable {
		g.oneCardVulnerable = ""
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]가 원카드!를 선언하여 안전해졌습니다!", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)
	} else {
		victimIdx := -1
		for i := 0; i < oneCardMaxPlayers; i++ {
			if g.players[i] != nil && g.players[i].UserID == g.oneCardVulnerable {
				victimIdx = i
				break
			}
		}
		if victimIdx >= 0 {
			if c, ok := g.drawFromDeckLocked(); ok {
				g.hands[victimIdx] = append(g.hands[victimIdx], c)
			}
			notice, _ := json.Marshal(ServerResponse{
				Type:    "game_notice",
				Message: fmt.Sprintf("[%s]가 [%s]에게 원카드!를 외쳐 벌칙 1장 드로우!", client.UserID, g.oneCardVulnerable),
				RoomID:  g.room.ID,
			})
			g.room.broadcastAll(notice)
			// 파산 체크
			if len(g.hands[victimIdx]) > oneCardBankruptcyLimit {
				victim := g.players[victimIdx]
				g.losers = append(g.losers, victim.UserID)
				g.oneCardVulnerable = ""
				msg := fmt.Sprintf("💀 [%s] 파산! (손패 %d장 초과)", victim.UserID, oneCardBankruptcyLimit)
				notice, _ := json.Marshal(ServerResponse{Type: "game_notice", Message: msg, RoomID: g.room.ID})
				g.room.broadcastAll(notice)
				if g.checkGameEndLocked() {
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
					g.gameStarted = false
					g.stopTurnTimerLocked()
					return
				}
				g.sendStateToAllLocked()
				return
			}
			// 원카드 취약 상태 갱신 (벌칙 후)
			g.oneCardVulnerable = ""
			for i := 0; i < oneCardMaxPlayers; i++ {
				if g.players[i] != nil && len(g.hands[i]) == 1 {
					g.oneCardVulnerable = g.players[i].UserID
					break
				}
			}
		} else {
			g.oneCardVulnerable = ""
		}
	}
	g.sendStateToAllLocked()
}

func (g *OneCardGame) isWinnerOrLoser(userID string) bool {
	for _, w := range g.winners {
		if w == userID {
			return true
		}
	}
	for _, l := range g.losers {
		if l == userID {
			return true
		}
	}
	return false
}

func (g *OneCardGame) advanceTurnLocked() {
	step := 1
	if g.skipNext {
		step = 2
		g.skipNext = false
	}
	step *= g.direction

	for i := 0; i < oneCardMaxPlayers; i++ {
		g.currentTurn = (g.currentTurn + step + oneCardMaxPlayers*10) % oneCardMaxPlayers
		p := g.players[g.currentTurn]
		if p == nil {
			continue
		}
		if g.isWinnerOrLoser(p.UserID) {
			continue
		}
		if len(g.hands[g.currentTurn]) > 0 {
			return
		}
	}
}

func (g *OneCardGame) checkGameEndLocked() bool {
	n := g.initialPlayerCount
	winThresh, loseThresh := 2, 2
	switch n {
	case 2:
		winThresh, loseThresh = 1, 1
	case 3:
		winThresh, loseThresh = 1, 2
	case 4:
		winThresh, loseThresh = 2, 2
	default:
		winThresh, loseThresh = 1, 1
	}
	if len(g.winners) >= winThresh || len(g.losers) >= loseThresh {
		// 승패 기록: winners + 필드에 남은 유저 = win, losers = lose
		winSet := make(map[string]bool)
		for _, w := range g.winners {
			winSet[w] = true
		}
		for i := 0; i < oneCardMaxPlayers; i++ {
			if g.players[i] != nil && len(g.hands[i]) > 0 && !g.isWinnerOrLoser(g.players[i].UserID) {
				winSet[g.players[i].UserID] = true
			}
		}
		for uid := range winSet {
			for i := 0; i < oneCardMaxPlayers; i++ {
				if g.players[i] != nil && g.players[i].UserID == uid {
					g.players[i].RecordResult("onecard", "win")
					break
				}
			}
		}
		for _, uid := range g.losers {
			for i := 0; i < oneCardMaxPlayers; i++ {
				if g.players[i] != nil && g.players[i].UserID == uid {
					g.players[i].RecordResult("onecard", "lose")
					break
				}
			}
		}
		return true
	}
	return false
}

func (g *OneCardGame) playerIndex(c *Client) int {
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == c.UserID {
			return i
		}
	}
	return -1
}

func (g *OneCardGame) newOneCardDeck() []Card {
	deck := NewShuffledDeck()
	deck = append(deck, oneCardBJoker, oneCardCJoker)
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

func (g *OneCardGame) startGameLocked() {
	deck := g.newOneCardDeck()

	playerIndices := make([]int, 0, oneCardMaxPlayers)
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil {
			playerIndices = append(playerIndices, i)
		}
	}

	for _, pi := range playerIndices {
		for j := 0; j < oneCardHandSize && len(deck) > 0; j++ {
			g.hands[pi] = append(g.hands[pi], deck[len(deck)-1])
			deck = deck[:len(deck)-1]
		}
	}

	if len(deck) > 0 {
		g.topCard = deck[len(deck)-1]
		deck = deck[:len(deck)-1]
	}
	g.deck = deck
	g.discardPile = nil
	g.targetSuit = ""
	g.direction = 1
	g.skipNext = false
	g.attackPenalty = 0
	g.playAgain = false
	g.oneCardVulnerable = ""
	g.winners = nil
	g.losers = nil
	g.initialPlayerCount = len(playerIndices)
	g.currentTurn = playerIndices[0]

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: "원카드 시작! A=+3, 조커=+5/+7 공격. 7=문양 변경, K=한 번 더. 20장 초과 시 파산!",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *OneCardGame) startTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	currentPlayer := g.players[g.currentTurn]
	if currentPlayer == nil {
		return
	}
	room := g.room
	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  currentPlayer.UserID,
		Remaining: oneCardTurnTimeLimit,
	})
	g.room.broadcastAll(data)
	go func() {
		remaining := oneCardTurnTimeLimit
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

func (g *OneCardGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *OneCardGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	if !g.gameStarted || g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != timedOutPlayer.UserID {
		g.mu.Unlock()
		return
	}
	g.stopTurnTimerLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("⏰ [%s] 시간 초과! 덱에서 1장 드로우합니다.", timedOutPlayer.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.mu.Unlock()
	g.handleDraw(timedOutPlayer)
}

func (g *OneCardGame) handleReady(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 이미 시작되었습니다."})
		return
	}
	idx := g.playerIndex(client)
	if idx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	g.startReady[client] = true
	total := 0
	ready := 0
	for i := 0; i < oneCardMaxPlayers; i++ {
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
		g.startGameLocked()
	}
}

func (g *OneCardGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}
	g.rematchReady[client] = true
	total := 0
	ready := 0
	for i := 0; i < oneCardMaxPlayers; i++ {
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
		for i := 0; i < oneCardMaxPlayers; i++ {
			g.hands[i] = nil
		}
		g.deck = nil
		g.discardPile = nil
		g.gameStarted = true
		g.startGameLocked()
	}
}

func (g *OneCardGame) handleTakeover(client *Client) {
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
	for i := 0; i < oneCardMaxPlayers; i++ {
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
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🪑 [%s]님이 빈자리에 참여했습니다. (%d/%d)", client.UserID, g.playerCount, oneCardMaxPlayers),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	total := g.playerCount
	ready := 0
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil && g.startReady[g.players[i]] {
			ready++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
	log.Printf("[ONECARD] room:[%s] [%s] 빈자리 참여 (슬롯 %d)", g.room.ID, client.UserID, slot)
}

func (g *OneCardGame) sendStateToAllLocked() {
	g.room.mu.RLock()
	clients := make([]*Client, 0, len(g.room.clients))
	for c := range g.room.clients {
		clients = append(clients, c)
	}
	g.room.mu.RUnlock()

	for _, c := range clients {
		g.sendStateToClientLocked(c)
	}
}

func (g *OneCardGame) sendStateToClientLocked(client *Client) {
	idx := g.playerIndex(client)
	if idx < 0 {
		g.sendStateToSpectatorLocked(client)
		return
	}

	players := make([]OneCardPlayerInfo, 0)
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		status := ""
		for _, w := range g.winners {
			if w == g.players[i].UserID {
				status = "escaped"
				break
			}
		}
		if status == "" {
			for _, l := range g.losers {
				if l == g.players[i].UserID {
					status = "bankrupt"
					break
				}
			}
		}
		players = append(players, OneCardPlayerInfo{
			UserID:    g.players[i].UserID,
			CardCount: len(g.hands[i]),
			IsTurn:    i == g.currentTurn,
			Status:    status,
		})
	}

	turnUser := ""
	if g.players[g.currentTurn] != nil {
		turnUser = g.players[g.currentTurn].UserID
	}

	myHand := make([]Card, len(g.hands[idx]))
	copy(myHand, g.hands[idx])
	for i := range myHand {
		myHand[i].Hidden = false
	}

	data := OneCardData{
		Hand:              myHand,
		TopCard:           g.topCard,
		TargetSuit:        g.targetSuit,
		DeckCount:         len(g.deck),
		DiscardCount:      len(g.discardPile),
		Turn:              turnUser,
		Direction:         g.direction,
		AttackPenalty:     g.attackPenalty,
		OneCardVulnerable: g.oneCardVulnerable,
		Players:           players,
	}
	client.SendJSON(OneCardStateResponse{
		Type:   "onecard_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *OneCardGame) sendStateToSpectatorLocked(client *Client) {
	players := make([]OneCardPlayerInfo, 0)
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		status := ""
		for _, w := range g.winners {
			if w == g.players[i].UserID {
				status = "escaped"
				break
			}
		}
		if status == "" {
			for _, l := range g.losers {
				if l == g.players[i].UserID {
					status = "bankrupt"
					break
				}
			}
		}
		players = append(players, OneCardPlayerInfo{
			UserID:    g.players[i].UserID,
			CardCount: len(g.hands[i]),
			IsTurn:    i == g.currentTurn,
			Status:    status,
		})
	}
	canTakeover := false
	if !g.gameStarted {
		for i := 0; i < oneCardMaxPlayers; i++ {
			if g.players[i] == nil {
				canTakeover = true
				break
			}
		}
	}
	turnUser := ""
	if g.players[g.currentTurn] != nil {
		turnUser = g.players[g.currentTurn].UserID
	}
	data := OneCardData{
		Hand:         []Card{},
		TopCard:      g.topCard,
		DeckCount:    len(g.deck),
		DiscardCount: len(g.discardPile),
		Turn:         turnUser,
		Direction:    g.direction,
		Players:      players,
		CanTakeover:  canTakeover,
	}
	client.SendJSON(OneCardStateResponse{
		Type:   "onecard_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}
