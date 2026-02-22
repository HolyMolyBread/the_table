package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
)

// handScore는 손패의 블랙잭 점수를 계산합니다.
// Hidden 카드는 제외하며 A는 버스트를 막기 위해 1 또는 11로 자동 조정합니다.
func handScore(hand []Card) int {
	total, aces := 0, 0
	for _, c := range hand {
		if c.Hidden {
			continue
		}
		switch c.Value {
		case "A":
			total += 11
			aces++
		case "J", "Q", "K":
			total += 10
		default:
			v, _ := strconv.Atoi(c.Value)
			total += v
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total
}

// isNaturalBlackjack은 2장 손패의 점수가 21인지 확인합니다.
func isNaturalBlackjack(hand []Card) bool {
	return len(hand) == 2 && handScore(hand) == 21
}

// ── 응답 타입 ────────────────────────────────────────────────────────────────

// BJHandInfo는 손패 카드와 가시 점수를 담습니다.
type BJHandInfo struct {
	Cards []Card `json:"cards"`
	Score int    `json:"score"` // Hidden 카드 제외 합계
}

// BJData는 블랙잭 게임 상태 스냅샷입니다.
type BJData struct {
	Phase        string     `json:"phase"`        // betting | player_turn | dealer_turn | settlement
	PlayerHand   BJHandInfo `json:"playerHand"`
	DealerHand   BJHandInfo `json:"dealerHand"`
	Message      string     `json:"message,omitempty"`
	MainPlayerID string     `json:"mainPlayerId,omitempty"` // 실제 플레이어 ID (관전자 판별용)
}

// BJResponse는 서버→클라이언트 블랙잭 메시지 최상위 구조입니다.
// type: "blackjack_state" 또는 "dealer_action"
type BJResponse struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	Data   BJData `json:"data"`
}

// ── 게임 페이즈 ────────────────────────────────────────────────────────────────

type BJPhase string

const (
	BJBetting    BJPhase = "betting"
	BJPlayerTurn BJPhase = "player_turn"
	BJDealerTurn BJPhase = "dealer_turn"
	BJSettlement BJPhase = "settlement"
)

// ── BlackjackGame 구조체 ──────────────────────────────────────────────────────

// BlackjackGame은 1인 플레이어 vs 딜러 AI PVE 블랙잭 플러그인입니다.
// 전적(승/무/패)은 코어의 RecordResult에 위임합니다.
type BlackjackGame struct {
	room *Room
	mu   sync.Mutex

	player     *Client
	phase      BJPhase
	deck       []Card
	playerHand []Card
	dealerHand []Card

	stopDealer chan struct{} // 딜러 AI 고루틴 중단 신호
}

// NewBlackjackGame은 새 BlackjackGame 인스턴스를 반환합니다.
func NewBlackjackGame(room *Room) *BlackjackGame {
	return &BlackjackGame{room: room, phase: BJBetting}
}

func init() { RegisterPlugin("blackjack", func(room *Room) GamePlugin { return NewBlackjackGame(room) }) }

func (g *BlackjackGame) Name() string { return "블랙잭 PVE (1 vs 딜러 AI)" }

// ── GamePlugin 인터페이스 구현 ────────────────────────────────────────────────

func (g *BlackjackGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.player == nil {
		// 첫 번째 입장자 = 플레이어
		g.player = client
		log.Printf("[BJ] room:[%s] [%s] 플레이어 입장", g.room.ID, client.UserID)
		g.broadcastStateLocked("🃏 블랙잭 테이블에 오신 것을 환영합니다! [게임 시작] 버튼을 누르세요.")
	} else if g.player == client {
		// 재입장 — 현재 상태 재전송
		g.broadcastStateLocked("")
	} else {
		// 두 번째 이후 입장자 = 관전자
		log.Printf("[BJ] room:[%s] [%s] 관전자 입장", g.room.ID, client.UserID)
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

		// 관전자에게 현재 게임 상태 스냅샷 개별 전송
		data := g.makeBJDataLocked(fmt.Sprintf("👀 관전 중입니다. (현재 %s 단계)", string(g.phase)))
		b, _ := json.Marshal(BJResponse{Type: "blackjack_state", RoomID: g.room.ID, Data: data})
		client.SafeSend(b)
	}
}

func (g *BlackjackGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.player != client {
		// 관전자 퇴장 — 별도 처리 없음
		log.Printf("[BJ] room:[%s] [%s] 관전자 퇴장", g.room.ID, client.UserID)
		return
	}

	// 방장(플레이어) 퇴장 → 딜러 중단 후 방 해산 알림
	g.stopDealerLocked()
	g.resetLocked()
	log.Printf("[BJ] room:[%s] [%s] 방장 퇴장 — 방 해산", g.room.ID, client.UserID)

	// 관전자에게 방 해산 알림 (error 타입 → 클라이언트가 자동 로비 이동)
	dissolvMsg, _ := json.Marshal(ServerResponse{
		Type:    "error",
		Message: "방장이 퇴장하여 방이 해산됩니다.",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(dissolvMsg)
}

func (g *BlackjackGame) HandleAction(client *Client, _ string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "잘못된 페이로드입니다."})
		return
	}
	switch p.Cmd {
	case "start":
		g.handleStart(client)
	case "hit":
		g.handleHit(client)
	case "stand":
		g.handleStand(client)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: "알 수 없는 명령: " + p.Cmd})
	}
}

// ── 액션 핸들러 ──────────────────────────────────────────────────────────────

// handleStart는 새 라운드를 시작합니다.
// betting 또는 settlement 상태에서 호출 가능합니다.
func (g *BlackjackGame) handleStart(client *Client) {
	g.mu.Lock()

	if g.player != client {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "이 테이블의 플레이어가 아닙니다."})
		return
	}
	if g.phase != BJBetting && g.phase != BJSettlement {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 게임을 시작할 수 없습니다."})
		return
	}
	// settlement에서 재시작 시 이전 라운드 상태를 정리합니다.
	if g.phase == BJSettlement {
		g.resetLocked() // stopDealerLocked 포함, phase → BJBetting
	}

	// 덱 셔플 및 딜 (딜러 두 번째 카드는 블라인드)
	g.deck = NewShuffledDeck()
	g.playerHand = []Card{g.deck[0], g.deck[2]}
	g.dealerHand = []Card{
		g.deck[1],
		{Suit: g.deck[3].Suit, Value: g.deck[3].Value, Hidden: true},
	}
	g.deck = g.deck[4:]

	log.Printf("[BJ] room:[%s] [%s] 게임 시작 — 딜", g.room.ID, client.UserID)

	// 플레이어 블랙잭 시 즉시 딜러 턴
	if isNaturalBlackjack(g.playerHand) {
		g.phase = BJDealerTurn
		g.broadcastStateLocked("🎴 블랙잭! 딜러 카드를 확인합니다...")
		g.mu.Unlock()
		go g.runDealerAI()
		return
	}
	g.phase = BJPlayerTurn
	g.broadcastStateLocked("카드가 분배되었습니다. Hit 또는 Stand를 선택하세요.")
	g.mu.Unlock()
}

func (g *BlackjackGame) handleHit(client *Client) {
	g.mu.Lock()

	if g.player != client || g.phase != BJPlayerTurn {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 Hit할 수 없습니다."})
		return
	}
	card := g.drawCardLocked()
	g.playerHand = append(g.playerHand, card)
	score := handScore(g.playerHand)
	log.Printf("[BJ] room:[%s] [%s] Hit → %s%s (합:%d)", g.room.ID, client.UserID, card.Suit, card.Value, score)

	if score > 21 {
		g.phase = BJSettlement
		g.broadcastStateLocked(fmt.Sprintf("💥 버스트! %d점 초과 — 패배. 다시 시작하려면 [게임 시작] 버튼을 누르세요.", score))
		g.mu.Unlock()
		client.RecordResult("blackjack", "lose")
		return
	}
	g.broadcastStateLocked(fmt.Sprintf("Hit! 현재 합계: %d", score))
	g.mu.Unlock()
}

func (g *BlackjackGame) handleStand(client *Client) {
	g.mu.Lock()

	if g.player != client || g.phase != BJPlayerTurn {
		g.mu.Unlock()
		client.SendJSON(ServerResponse{Type: "error", Message: "지금은 Stand할 수 없습니다."})
		return
	}
	g.phase = BJDealerTurn
	g.broadcastStateLocked("Stand! 딜러 턴...")
	g.mu.Unlock()
	go g.runDealerAI()
}

// ── 딜러 AI 고루틴 ───────────────────────────────────────────────────────────

// runDealerAI는 블라인드 공개 → 히트 루프 → 정산을 담당합니다.
// 고루틴으로 실행됩니다.
func (g *BlackjackGame) runDealerAI() {
	// 새 stop 채널 생성
	stopCh := make(chan struct{})
	g.mu.Lock()
	g.stopDealerLocked()
	g.stopDealer = stopCh
	g.mu.Unlock()

	// 중단 여부 확인 헬퍼
	stopped := func() bool {
		select {
		case <-stopCh:
			return true
		default:
			return false
		}
	}

	// 1) 블라인드 카드 공개
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

	// 2) 딜러 히트 루프 — 16 이하면 반드시 Hit
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

	// 3) 정산
	if stopped() {
		return
	}
	g.settle(stopCh)
}

// ── 정산 ────────────────────────────────────────────────────────────────────

// settle은 최종 승패를 계산하고 전적을 갱신합니다.
func (g *BlackjackGame) settle(stopCh chan struct{}) {
	g.mu.Lock()
	player := g.player
	if player == nil {
		g.mu.Unlock()
		return
	}

	pScore := handScore(g.playerHand)
	dScore := handScore(g.dealerHand)
	playerBJ := isNaturalBlackjack(g.playerHand)
	dealerBJ := isNaturalBlackjack(g.dealerHand)

	var result string
	var msg string

	const restartHint = " [게임 시작] 버튼으로 다음 라운드를 시작하세요."

	switch {
	case playerBJ && dealerBJ:
		result = "draw"
		msg = "🤝 블랙잭 vs 블랙잭 — 무승부(Push)!" + restartHint
	case playerBJ:
		result = "win"
		msg = "🎴 블랙잭! 승리!" + restartHint
	case dScore > 21:
		result = "win"
		msg = "🎉 딜러 버스트! 승리!" + restartHint
	case pScore > dScore:
		result = "win"
		msg = fmt.Sprintf("🏆 승리! (나 %d vs 딜러 %d)%s", pScore, dScore, restartHint)
	case pScore == dScore:
		result = "draw"
		msg = fmt.Sprintf("🤝 무승부(Push)! (둘 다 %d점)%s", pScore, restartHint)
	default:
		result = "lose"
		msg = fmt.Sprintf("😞 패배. 딜러 %d vs 나 %d%s", dScore, pScore, restartHint)
	}

	g.phase = BJSettlement
	g.mu.Unlock()

	select {
	case <-stopCh:
		return
	default:
	}

	// 전적 갱신 (코어 위임)
	player.RecordResult("blackjack", result)

	g.mu.Lock()
	g.broadcastStateLocked(msg)
	g.mu.Unlock()

	log.Printf("[BJ] room:[%s] 정산 — p:%d d:%d result:%s", g.room.ID, pScore, dScore, result)
	// ※ 자동 재시작 없음 — 유저가 [게임 시작]을 눌러야 다음 라운드 시작
}

// ── 내부 헬퍼 ────────────────────────────────────────────────────────────────

// drawCardLocked는 덱에서 카드 한 장을 뽑습니다. g.mu 보유 상태에서 호출.
func (g *BlackjackGame) drawCardLocked() Card {
	if len(g.deck) == 0 {
		g.deck = NewShuffledDeck()
	}
	c := g.deck[0]
	g.deck = g.deck[1:]
	return c
}

// stopDealerLocked는 딜러 AI 고루틴을 중단합니다. g.mu 보유 상태에서 호출.
func (g *BlackjackGame) stopDealerLocked() {
	if g.stopDealer != nil {
		close(g.stopDealer)
		g.stopDealer = nil
	}
}

// resetLocked는 게임 상태를 초기화합니다. g.mu 보유 상태에서 호출.
func (g *BlackjackGame) resetLocked() {
	g.stopDealerLocked()
	g.phase = BJBetting
	g.deck = nil
	g.playerHand = nil
	g.dealerHand = nil
}

// makeBJDataLocked는 현재 게임 상태 스냅샷을 빌드합니다. g.mu 보유 상태에서 호출.
func (g *BlackjackGame) makeBJDataLocked(msg string) BJData {
	mainPlayerID := ""
	if g.player != nil {
		mainPlayerID = g.player.UserID
	}
	return BJData{
		Phase:        string(g.phase),
		PlayerHand:   BJHandInfo{Cards: g.playerHand, Score: handScore(g.playerHand)},
		DealerHand:   BJHandInfo{Cards: g.dealerHand, Score: handScore(g.dealerHand)},
		Message:      msg,
		MainPlayerID: mainPlayerID,
	}
}

// broadcastStateLocked는 "blackjack_state" 메시지를 방 전체에 브로드캐스트합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *BlackjackGame) broadcastStateLocked(msg string) {
	data := g.makeBJDataLocked(msg)
	b, _ := json.Marshal(BJResponse{Type: "blackjack_state", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(b)
}

// broadcastDealerActionLocked는 "dealer_action" 메시지를 방 전체에 브로드캐스트합니다.
// g.mu 보유 상태에서 호출합니다.
func (g *BlackjackGame) broadcastDealerActionLocked(msg string) {
	data := g.makeBJDataLocked(msg)
	b, _ := json.Marshal(BJResponse{Type: "dealer_action", RoomID: g.room.ID, Data: data})
	g.room.broadcastAll(b)
}
