package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// ── PVP 블랙잭 데이터 구조 ─────────────────────────────────────────────────────

// PVPBJPlayer는 PVP 블랙잭 플레이어 한 명의 상태입니다.
type PVPBJPlayer struct {
	UserID string   `json:"userId"`
	Hand   []Card   `json:"hand"`
	Hearts int      `json:"hearts"`
	State  string   `json:"state"` // "playing" | "stand" | "bust" | "out"
}

// RaidResultData는 라운드/게임 종료 시 HP 정산 데이터입니다.
type RaidResultData struct {
	DealerHP       int            `json:"dealerHp"`
	DealerDamage   int            `json:"dealerDamage"`
	PlayerChanges  map[string]int  `json:"playerChanges"` // { "userId": -10 } (음수=손실)
}

// PVPBJData는 PVP 블랙잭 게임 상태 스냅샷입니다.
type PVPBJData struct {
	Phase          string                  `json:"phase"`
	Players        map[string]*PVPBJPlayer `json:"players"`
	TurnOrder      []string                `json:"turnOrder"`
	CurrentTurnIdx int                     `json:"currentTurnIdx"`
	DealerHand     []Card                  `json:"dealerHand"`
	DealerHearts   int                     `json:"dealerHearts"`
	ReadyStatus    map[string]bool         `json:"readyStatus,omitempty"`
	Message        string                 `json:"message,omitempty"`
	GameOverWin    bool                    `json:"gameOverWin,omitempty"`
	RaidResult     *RaidResultData         `json:"raidResult,omitempty"`
}

// PVPBJResponse는 PVP 블랙잭 메시지 최상위 구조입니다.
type PVPBJResponse struct {
	Type   string     `json:"type"`
	RoomID string     `json:"roomId"`
	Data   PVPBJData  `json:"data"`
}

// ── BlackjackPVPGame ──────────────────────────────────────────────────────────

type BJPVPPhase string

const (
	BJPVPBetting    BJPVPPhase = "betting"
	BJPVPPlayerTurn BJPVPPhase = "player_turn"
	BJPVPDealerTurn BJPVPPhase = "dealer_turn"
	BJPVPSettlement BJPVPPhase = "settlement"
	BJPVPGameOver   BJPVPPhase = "game_over"
)

const bjPvpPlayerHearts = 10

type BlackjackPVPGame struct {
	room *Room
	mu   sync.Mutex

	players        map[string]*PVPBJPlayer
	clientByUserID map[string]*Client
	readyStatus    map[string]bool
	turnOrder      []string
	currentTurnIdx int
	dealerHand     []Card
	dealerHearts   int
	deck           []Card
	phase          BJPVPPhase
	gameStarted    bool
	stopDealer     chan struct{}
	lastRaidResult *RaidResultData
}

func NewBlackjackPVPGame(room *Room) *BlackjackPVPGame {
	return &BlackjackPVPGame{
		room:           room,
		players:        make(map[string]*PVPBJPlayer),
		clientByUserID: make(map[string]*Client),
		readyStatus:    make(map[string]bool),
		turnOrder:      nil,
		phase:          BJPVPBetting,
	}
}

func init() { RegisterPlugin("blackjack_raid", func(room *Room) GamePlugin { return NewBlackjackPVPGame(room) }) }

func (g *BlackjackPVPGame) Name() string { return "PVP 딜러 레이드 블랙잭" }

// ── GamePlugin 구현 ───────────────────────────────────────────────────────────

func (g *BlackjackPVPGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.players[client.UserID]; ok {
		g.broadcastStateLocked("")
		return
	}
	g.players[client.UserID] = &PVPBJPlayer{UserID: client.UserID, Hearts: 0, State: "playing"}
	g.clientByUserID[client.UserID] = client

	n := len(g.players)
	msg := fmt.Sprintf("🃏 PVP 딜러 레이드 블랙잭! (%d명 대기 중) [준비] 버튼을 누르세요.", n)
	g.broadcastStateLocked(msg)
}

func (g *BlackjackPVPGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.players, client.UserID)
	delete(g.clientByUserID, client.UserID)
	delete(g.readyStatus, client.UserID)
	g.stopDealerLocked()
	if len(g.players) == 0 {
		g.resetLocked()
	}
}

func (g *BlackjackPVPGame) HandleAction(client *Client, _ string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "잘못된 페이로드입니다."})
		return
	}
	switch p.Cmd {
	case "ready":
		g.handleReady(client)
	case "hit":
		g.handleHit(client)
	case "stand":
		g.handleStand(client)
	case "rematch":
		g.handleRematch(client)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: "알 수 없는 명령: " + p.Cmd})
	}
}

// ── 액션 핸들러 ──────────────────────────────────────────────────────────────

func (g *BlackjackPVPGame) handleReady(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.players[client.UserID]; !ok {
		client.SendJSON(ServerResponse{Type: "error", Message: "이 테이블의 플레이어가 아닙니다."})
		return
	}
	if g.phase != BJPVPBetting && g.phase != BJPVPSettlement {
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 준비할 수 없습니다."})
		return
	}
	if g.phase == BJPVPGameOver {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임이 종료되었습니다. [한 판 더] 버튼으로 리매치하세요."})
		return
	}

	g.readyStatus[client.UserID] = true

	// 전원(인간+봇) 준비 시에만 실제 게임 시작
	allReady := true
	for uid := range g.players {
		if !g.readyStatus[uid] {
			allReady = false
			break
		}
	}
	if allReady && len(g.players) > 0 {
		// readyStatus 초기화 후 handleStart 로직 실행
		g.readyStatus = make(map[string]bool)
		g.handleStartLocked()
		return
	}
	g.broadcastStateLocked("")
}

func (g *BlackjackPVPGame) handleStartLocked() {
	if !g.gameStarted {
		n := len(g.players)
		g.dealerHearts = n * 10
		for _, p := range g.players {
			p.Hearts = bjPvpPlayerHearts
		}
		g.gameStarted = true
	}

	if g.phase == BJPVPSettlement {
		g.stopDealerLocked()
		g.phase = BJPVPBetting
		g.deck = nil
		g.dealerHand = nil
		for _, p := range g.players {
			p.Hand = nil
			p.State = "playing"
		}
	}

	g.deck = NewShuffledDeck()
	g.turnOrder = make([]string, 0, len(g.players))
	for uid := range g.players {
		g.turnOrder = append(g.turnOrder, uid)
	}
	g.currentTurnIdx = 0

	// 딜 순서: p1, p2, ..., pN, dealer, p1, p2, ..., pN, dealer (블랙잭 표준)
	n := len(g.turnOrder)
	for j, uid := range g.turnOrder {
		p := g.players[uid]
		p.Hand = []Card{g.deck[j], g.deck[n+1+j]}
		p.State = "playing"
	}
	g.dealerHand = []Card{
		g.deck[n],
		{Suit: g.deck[2*n+1].Suit, Value: g.deck[2*n+1].Value, Hidden: true},
	}
	g.deck = g.deck[2*n+2:]

	log.Printf("[BJ-PVP] room:[%s] 게임 시작 — %d명 vs 딜러 %d하트", g.room.ID, len(g.turnOrder), g.dealerHearts)

	for _, uid := range g.turnOrder {
		p := g.players[uid]
		if isNaturalBlackjack(p.Hand) {
			p.State = "stand"
		}
	}
	g.advanceToNextPlayerOrDealerLocked()
}

func (g *BlackjackPVPGame) advanceToNextPlayerOrDealerLocked() {
	for g.currentTurnIdx < len(g.turnOrder) {
		uid := g.turnOrder[g.currentTurnIdx]
		p := g.players[uid]
		if p.State == "playing" && p.Hearts > 0 {
			g.phase = BJPVPPlayerTurn
			g.broadcastStateLocked(fmt.Sprintf("[%s]의 차례 — Hit 또는 Stand", uid))
			return
		}
		g.currentTurnIdx++
	}
	g.phase = BJPVPDealerTurn
	g.broadcastStateLocked("딜러 턴...")
	go g.runDealerAI()
}

func (g *BlackjackPVPGame) handleHit(client *Client) {
	g.mu.Lock()

	if g.phase != BJPVPPlayerTurn {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 Hit할 수 없습니다."})
		return
	}
	uid := client.UserID
	if g.currentTurnIdx >= len(g.turnOrder) || g.turnOrder[g.currentTurnIdx] != uid {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 당신의 차례가 아닙니다."})
		return
	}
	p := g.players[uid]
	if p.State != "playing" {
		g.mu.Unlock()
		return
	}

	card := g.drawCardLocked()
	p.Hand = append(p.Hand, card)
	score := handScore(p.Hand)
	log.Printf("[BJ-PVP] room:[%s] [%s] Hit → %s%s (합:%d)", g.room.ID, uid, card.Suit, card.Value, score)

	if score > 21 {
		p.State = "bust"
		p.Hearts--
		g.dealerHearts++
		if p.Hearts <= 0 {
			p.State = "out"
			if c := g.clientByUserID[uid]; c != nil {
				c.RecordResult("blackjack_raid", "lose")
			}
		}
		g.broadcastStateLocked(fmt.Sprintf("💥 [%s] 버스트! %d점", uid, score))
		g.currentTurnIdx++
		g.advanceToNextPlayerOrDealerLocked()
		g.mu.Unlock()
		return
	}
	g.broadcastStateLocked(fmt.Sprintf("Hit! [%s] 합계: %d", uid, score))
	g.mu.Unlock()
}

func (g *BlackjackPVPGame) handleStand(client *Client) {
	g.mu.Lock()

	if g.phase != BJPVPPlayerTurn {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 Stand할 수 없습니다."})
		return
	}
	uid := client.UserID
	if g.currentTurnIdx >= len(g.turnOrder) || g.turnOrder[g.currentTurnIdx] != uid {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 당신의 차례가 아닙니다."})
		return
	}
	p := g.players[uid]
	if p.State != "playing" {
		g.mu.Unlock()
		return
	}

	p.State = "stand"
	g.broadcastStateLocked(fmt.Sprintf("Stand! [%s]", uid))
	g.currentTurnIdx++
	g.advanceToNextPlayerOrDealerLocked()
	g.mu.Unlock()
}

// ── 딜러 AI ──────────────────────────────────────────────────────────────────

func (g *BlackjackPVPGame) runDealerAI() {
	stopCh := make(chan struct{})
	g.mu.Lock()
	g.stopDealerLocked()
	g.stopDealer = stopCh
	g.mu.Unlock()

	stopped := func() bool {
		select {
		case <-stopCh:
			return true
		default:
			return false
		}
	}

	time.Sleep(900 * time.Millisecond)
	if stopped() {
		return
	}
	g.mu.Lock()
	if len(g.dealerHand) > 1 {
		g.dealerHand[1].Hidden = false
	}
	g.broadcastDealerActionLocked("딜러 카드 공개!")
	g.mu.Unlock()

	for {
		if stopped() {
			return
		}
		time.Sleep(1 * time.Second)
		if stopped() {
			return
		}
		g.mu.Lock()
		score := handScore(g.dealerHand)
		if score >= 17 {
			g.mu.Unlock()
			break
		}
		card := g.drawCardLocked()
		g.dealerHand = append(g.dealerHand, card)
		newScore := handScore(g.dealerHand)
		msg := fmt.Sprintf("딜러 Hit → %s%s  (합: %d)", card.Suit, card.Value, newScore)
		g.broadcastDealerActionLocked(msg)
		g.mu.Unlock()
	}

	if stopped() {
		return
	}
	g.settle(stopCh)
}

func (g *BlackjackPVPGame) settle(stopCh chan struct{}) {
	g.mu.Lock()
	defer g.mu.Unlock()

	prevDealerHearts := g.dealerHearts
	prevPlayerHearts := make(map[string]int)
	for uid, p := range g.players {
		prevPlayerHearts[uid] = p.Hearts
	}

	dScore := handScore(g.dealerHand)
	dealerBJ := isNaturalBlackjack(g.dealerHand)

	for _, uid := range g.turnOrder {
		p := g.players[uid]
		if p.Hearts <= 0 {
			p.State = "out"
			continue
		}
		if p.State == "bust" {
			continue
		}
		pScore := handScore(p.Hand)
		playerBJ := isNaturalBlackjack(p.Hand)

		var pDelta, dDelta int
		switch {
		case playerBJ && dealerBJ:
			pDelta, dDelta = 0, 0
		case playerBJ:
			pDelta, dDelta = 2, -2
		case dScore > 21:
			pDelta, dDelta = 1, -1
		case pScore > dScore:
			pDelta, dDelta = 1, -1
		case pScore == dScore:
			pDelta, dDelta = 0, 0
		default:
			pDelta, dDelta = -1, 1
		}
		p.Hearts += pDelta
		g.dealerHearts += dDelta
		if p.Hearts <= 0 {
			p.State = "out"
			if c := g.clientByUserID[uid]; c != nil {
				c.RecordResult("blackjack_raid", "lose")
			}
		}
	}

	g.phase = BJPVPSettlement

	playerChanges := make(map[string]int)
	for uid, p := range g.players {
		diff := prevPlayerHearts[uid] - p.Hearts
		if diff != 0 {
			playerChanges[uid] = -diff
		}
	}
	dealerDamage := prevDealerHearts - g.dealerHearts
	if dealerDamage < 0 {
		dealerDamage = 0
	}
	g.lastRaidResult = &RaidResultData{
		DealerHP:      g.dealerHearts,
		DealerDamage:  dealerDamage,
		PlayerChanges: playerChanges,
	}

	if g.dealerHearts <= 0 {
		g.finishGameOverLocked(true)
		return
	}
	allOut := true
	for _, p := range g.players {
		if p.Hearts > 0 {
			allOut = false
			break
		}
	}
	if allOut {
		g.finishGameOverLocked(false)
		return
	}

	msg := fmt.Sprintf("정산 완료. ❤️ 딜러 %d | [게임 시작]으로 다음 라운드", g.dealerHearts)
	g.broadcastStateLocked(msg)
}

func (g *BlackjackPVPGame) finishGameOverLocked(playerWin bool) {
	g.phase = BJPVPGameOver
	g.stopDealerLocked()
	g.deck = nil
	g.dealerHand = nil
	for _, p := range g.players {
		p.Hand = nil
		p.State = "out"
	}

	winner := "딜러"
	if playerWin {
		winner = "유저들"
		for uid, p := range g.players {
			if p.Hearts > 0 {
				if c := g.clientByUserID[uid]; c != nil {
					c.RecordResult("blackjack_raid", "win")
				}
			}
		}
	}
	// playerWin=false(딜러 승리) 시: 탈락 시점에 이미 RecordResult("blackjack_raid","lose") 기록됨 → 중복 기록 없음

	msg := fmt.Sprintf("🏆 게임 종료! [%s] 승리!", winner)
	resultData := map[string]any{"playerWin": playerWin}
	if g.lastRaidResult != nil {
		resultData["dealerHp"] = g.lastRaidResult.DealerHP
		resultData["dealerDamage"] = g.lastRaidResult.DealerDamage
		resultData["playerChanges"] = g.lastRaidResult.PlayerChanges
	}
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        msg,
		RoomID:         g.room.ID,
		RematchEnabled: true,
		Data:           resultData,
	})
	g.room.broadcastAll(data)
	g.broadcastStateLocked(msg)
}

func (g *BlackjackPVPGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.players[client.UserID]; !ok {
		client.SendJSON(ServerResponse{Type: "error", Message: "이 테이블의 플레이어가 아닙니다."})
		return
	}
	if g.phase != BJPVPGameOver {
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 리매치할 수 없습니다."})
		return
	}
	n := len(g.players)
	g.dealerHearts = n * 10
	for _, p := range g.players {
		p.Hearts = bjPvpPlayerHearts
		p.State = "playing"
	}
	g.phase = BJPVPBetting
	g.readyStatus = make(map[string]bool)
	g.broadcastStateLocked("🔄 리매치! [준비] 버튼을 누르세요.")
}

// ── 헬퍼 ──────────────────────────────────────────────────────────────────────

func (g *BlackjackPVPGame) drawCardLocked() Card {
	if len(g.deck) == 0 {
		g.deck = NewShuffledDeck()
	}
	c := g.deck[0]
	g.deck = g.deck[1:]
	return c
}

func (g *BlackjackPVPGame) stopDealerLocked() {
	if g.stopDealer != nil {
		close(g.stopDealer)
		g.stopDealer = nil
	}
}

func (g *BlackjackPVPGame) resetLocked() {
	g.stopDealerLocked()
	g.phase = BJPVPBetting
	g.readyStatus = make(map[string]bool)
	g.deck = nil
	g.dealerHand = nil
	g.turnOrder = nil
	g.gameStarted = false
	for _, p := range g.players {
		p.Hand = nil
		p.State = "playing"
	}
}

func (g *BlackjackPVPGame) makePVPBJDataLocked(msg string) PVPBJData {
	playersCopy := make(map[string]*PVPBJPlayer)
	for uid, p := range g.players {
		handCopy := make([]Card, len(p.Hand))
		copy(handCopy, p.Hand)
		playersCopy[uid] = &PVPBJPlayer{
			UserID: p.UserID,
			Hand:   handCopy,
			Hearts: p.Hearts,
			State:  p.State,
		}
	}
	dealerCopy := make([]Card, len(g.dealerHand))
	copy(dealerCopy, g.dealerHand)
	turnOrderCopy := make([]string, len(g.turnOrder))
	copy(turnOrderCopy, g.turnOrder)
	readyCopy := make(map[string]bool)
	for uid, v := range g.readyStatus {
		readyCopy[uid] = v
	}

	data := PVPBJData{
		Phase:          string(g.phase),
		Players:        playersCopy,
		TurnOrder:      turnOrderCopy,
		CurrentTurnIdx: g.currentTurnIdx,
		DealerHand:     dealerCopy,
		DealerHearts:   g.dealerHearts,
		ReadyStatus:    readyCopy,
		Message:        msg,
		GameOverWin:    g.phase == BJPVPGameOver && g.dealerHearts <= 0,
	}
	if g.phase == BJPVPSettlement || g.phase == BJPVPGameOver {
		data.RaidResult = g.lastRaidResult
	}
	return data
}

func (g *BlackjackPVPGame) broadcastStateLocked(msg string) {
	data := g.makePVPBJDataLocked(msg)
	b, _ := json.Marshal(PVPBJResponse{Type: "blackjack_pvp_state", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(b)
}

func (g *BlackjackPVPGame) broadcastDealerActionLocked(msg string) {
	data := g.makePVPBJDataLocked(msg)
	b, _ := json.Marshal(PVPBJResponse{Type: "dealer_action", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(b)
}
