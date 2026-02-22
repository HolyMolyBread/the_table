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
	oneCardMaxPlayers   = 4
	oneCardHandSize     = 7
	oneCardTurnTimeLimit = 15
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// OneCardData는 onecard_state 응답의 data 필드입니다.
type OneCardData struct {
	Hand         []Card `json:"hand"`         // 내 패
	TopCard      Card   `json:"topCard"`      // 바닥(탑) 카드
	DeckCount    int    `json:"deckCount"`    // 남은 덱 수
	Turn         string `json:"turn"`         // 현재 차례 유저 ID
	Direction    int    `json:"direction"`    // 1=시계, -1=반시계
	Players      []OneCardPlayerInfo `json:"players"`
	Message      string `json:"message,omitempty"`
}

// OneCardPlayerInfo는 한 플레이어의 공개 정보입니다.
type OneCardPlayerInfo struct {
	UserID    string `json:"userId"`
	CardCount int    `json:"cardCount"`
}

// OneCardStateResponse는 원카드 게임 상태 응답입니다.
type OneCardStateResponse struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId"`
	Data   OneCardData `json:"data"`
}

// ── OneCardGame 플러그인 ──────────────────────────────────────────────────────

// OneCardGame은 2~4인 원카드 게임 플러그인입니다.
// 탑 카드와 문양/숫자 일치 카드만 낼 수 있음. J=스킵, Q=방향반전. 패 0장이면 승리.
type OneCardGame struct {
	room          *Room
	players       [oneCardMaxPlayers]*Client
	hands         [oneCardMaxPlayers][]Card
	deck          []Card
	topCard       Card
	direction     int    // 1=시계, -1=반시계
	currentTurn   int
	skipNext      bool   // J로 인한 스킵
	playerCount   int
	gameStarted   bool
	stopTick      chan struct{}
	startReady    map[*Client]bool
	rematchReady  map[*Client]bool
	mu            sync.Mutex
}

func NewOneCardGame(room *Room) *OneCardGame {
	return &OneCardGame{room: room, direction: 1, startReady: make(map[*Client]bool), rematchReady: make(map[*Client]bool)}
}

func (g *OneCardGame) Name() string { return "onecard" }

// OnJoin은 플레이어 입장 시 호출됩니다.
func (g *OneCardGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] == client {
			g.sendStateToAllLocked()
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

// OnLeave는 플레이어 퇴장 시 호출됩니다.
// 방폭 방지: 남은 인원 2명 이상이면 방을 깨지 않고 게임 계속 진행.
func (g *OneCardGame) OnLeave(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < oneCardMaxPlayers; i++ {
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

	// 퇴장자에게 lose 전적 기록
	client.RecordResult("onecard", "lose")

	// 퇴장자가 현재 차례였다면 타이머 정지 후 턴 진행
	if idx == g.currentTurn {
		g.stopTurnTimerLocked()
		g.advanceTurnLocked()
	}

	// 생존자(패 보유) 수
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

	// 생존자 1명 이하 → 매치 종료
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
	log.Printf("[ONECARD] room:[%s] [%s] 퇴장 — 매치 종료", g.room.ID, client.UserID)
}

// HandleAction은 game_action 메시지를 처리합니다.
func (g *OneCardGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd   string `json:"cmd"`
		Index int    `json:"index"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "play":
		g.handlePlay(client, p.Index)
	case "draw":
		g.handleDraw(client)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 원카드 명령: [%s]", p.Cmd),
		})
	}
}

func (g *OneCardGame) handlePlay(client *Client, index int) {
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
	if g.players[g.currentTurn] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "내 차례가 아닙니다."})
		return
	}
	g.stopTurnTimerLocked()
	if index < 0 || index >= len(g.hands[idx]) {
		client.SendJSON(ServerResponse{Type: "error", Message: "유효하지 않은 카드 인덱스입니다."})
		return
	}

	card := g.hands[idx][index]
	if !g.canPlay(card) {
		client.SendJSON(ServerResponse{Type: "error", Message: "바닥 카드와 문양 또는 숫자가 일치해야 합니다."})
		return
	}

	// 카드 제거
	g.hands[idx] = append(g.hands[idx][:index], g.hands[idx][index+1:]...)
	g.topCard = card

	notice := ""
	if card.Value == "J" {
		g.skipNext = true
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 다음 턴 스킵!", client.UserID, card.Suit, card.Value)
	} else if card.Value == "Q" {
		g.direction *= -1
		notice = fmt.Sprintf("[%s]가 %s%s를 내서 방향 반전!", client.UserID, card.Suit, card.Value)
	} else {
		notice = fmt.Sprintf("[%s]가 %s%s를 냈습니다.", client.UserID, card.Suit, card.Value)
	}
	msg, _ := json.Marshal(ServerResponse{Type: "game_notice", Message: notice, RoomID: g.room.ID})
	g.room.broadcastAll(msg)

	// 승리 체크
	if len(g.hands[idx]) == 0 {
		client.RecordResult("onecard", "win")
		for i := 0; i < oneCardMaxPlayers; i++ {
			if g.players[i] != nil && i != idx {
				g.players[i].RecordResult("onecard", "lose")
			}
		}
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

	g.advanceTurnLocked()
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *OneCardGame) handleDraw(client *Client) {
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
	if g.players[g.currentTurn] != client {
		client.SendJSON(ServerResponse{Type: "error", Message: "내 차례가 아닙니다."})
		return
	}
	g.stopTurnTimerLocked()
	if len(g.deck) == 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "덱에 카드가 없습니다."})
		return
	}

	// 덱에서 1장 드로우
	drawn := g.deck[len(g.deck)-1]
	g.deck = g.deck[:len(g.deck)-1]
	g.hands[idx] = append(g.hands[idx], drawn)

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s]가 %s%s를 드로우했습니다.", client.UserID, drawn.Suit, drawn.Value),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	g.advanceTurnLocked()
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *OneCardGame) canPlay(c Card) bool {
	t := g.topCard
	return c.Suit == t.Suit || c.Value == t.Value
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
		if g.players[g.currentTurn] != nil && len(g.hands[g.currentTurn]) > 0 {
			return
		}
	}
}

func (g *OneCardGame) playerIndex(c *Client) int {
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] == c {
			return i
		}
	}
	return -1
}

func (g *OneCardGame) startGameLocked() {
	deck := NewShuffledDeck()
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	playerIndices := make([]int, 0, oneCardMaxPlayers)
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] != nil {
			playerIndices = append(playerIndices, i)
		}
	}

	// 각 7장씩 분배
	for _, pi := range playerIndices {
		for j := 0; j < oneCardHandSize && len(deck) > 0; j++ {
			g.hands[pi] = append(g.hands[pi], deck[len(deck)-1])
			deck = deck[:len(deck)-1]
		}
	}

	// 탑 카드
	if len(deck) > 0 {
		g.topCard = deck[len(deck)-1]
		deck = deck[:len(deck)-1]
	}
	g.deck = deck
	g.direction = 1
	g.skipNext = false
	g.currentTurn = playerIndices[0]

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: "원카드 시작! 바닥 카드와 문양 또는 숫자가 같은 카드를 내세요. J=스킵, Q=방향반전. 낼 카드가 없으면 드로우!",
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
	if !g.gameStarted || g.players[g.currentTurn] != timedOutPlayer {
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
		g.gameStarted = true
		g.startGameLocked()
	}
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
		return
	}

	players := make([]OneCardPlayerInfo, 0)
	for i := 0; i < oneCardMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		players = append(players, OneCardPlayerInfo{
			UserID:    g.players[i].UserID,
			CardCount: len(g.hands[i]),
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
		Hand:      myHand,
		TopCard:   g.topCard,
		DeckCount: len(g.deck),
		Turn:      turnUser,
		Direction: g.direction,
		Players:   players,
	}
	client.SendJSON(OneCardStateResponse{
		Type:   "onecard_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}
