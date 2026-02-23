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
	holdemMaxPlayers     = 4
	holdemStartStars     = 10
	holdemCheckCost      = 1
	holdemTurnTimeLimit  = 20
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// HoldemPlayerInfo는 한 플레이어의 공개 정보입니다.
type HoldemPlayerInfo struct {
	UserID   string  `json:"userId"`
	Stars    int     `json:"stars"`
	Status   string  `json:"status"`   // "check" | "fold" | ""
	Cards    []Card  `json:"cards"`   // 본인은 앞면, 타인은 Hidden=true
	IsActive bool    `json:"isActive"` // 이번 라운드 생존
}

// HoldemData는 holdem_state 응답의 data 필드입니다.
type HoldemData struct {
	Phase          string             `json:"phase"`
	Round          int                `json:"round"`
	Pot            int                `json:"pot"`
	CommunityCards []Card            `json:"communityCards"`
	Players        []HoldemPlayerInfo `json:"players"`
	CurrentTurn    string             `json:"currentTurn"`
	Message        string             `json:"message,omitempty"`
	CanTakeover    bool               `json:"canTakeover,omitempty"`
	MyHandName     string             `json:"myHandName,omitempty"`
}

// HoldemStateResponse는 홀덤 게임 상태 응답입니다.
type HoldemStateResponse struct {
	Type   string     `json:"type"`
	RoomID string     `json:"roomId"`
	Data   HoldemData `json:"data"`
}

// ── HoldemGame 플러그인 ───────────────────────────────────────────────────────

// HoldemGame은 별(⭐) 서바이벌 룰의 텍사스 홀덤 플러그인입니다.
type HoldemGame struct {
	room             *Room
	players          [holdemMaxPlayers]*Client
	stars            [holdemMaxPlayers]int
	holeCards        [holdemMaxPlayers][2]Card
	communityCards   [5]Card
	deck             []Card
	pot              int
	potCarryOver     int // 무승부 시 다음 라운드 이월
	phase            string
	round            int
	foldedThisRound  [holdemMaxPlayers]bool
	actedThisPhase   [holdemMaxPlayers]bool
	dealerIdx        int
	currentPlayerIdx int
	gameStarted      bool
	playerCount      int
	startReady       map[*Client]bool
	rematchReady     map[*Client]bool
	stopTick         chan struct{}
	mu               sync.Mutex
}

// NewHoldemGame creates a new Holdem game plugin.
func NewHoldemGame(room *Room) *HoldemGame {
	return &HoldemGame{room: room, phase: "waiting", startReady: make(map[*Client]bool), rematchReady: make(map[*Client]bool)}
}

func init() { RegisterPlugin("holdem", func(room *Room) GamePlugin { return NewHoldemGame(room) }) }

func (g *HoldemGame) Name() string { return "holdem" }

// OnJoin은 플레이어 입장 시 호출됩니다.
func (g *HoldemGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 이미 플레이어인지 확인
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] == client {
			g.sendStateToAllLocked()
			return
		}
	}

	// 빈 슬롯 찾기
	slot := -1
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		// 관전자
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
	g.stars[slot] = holdemStartStars
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("♠️ [%s]님이 입장했습니다. (%d/4)", client.UserID, g.playerCount),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	if !g.gameStarted {
		total := g.playerCount
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: total,
		})
		g.room.broadcastAll(upd)
		g.sendStateToAllLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

// OnLeave는 플레이어 퇴장 시 호출됩니다.
func (g *HoldemGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < holdemMaxPlayers; i++ {
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
		for i := 0; i < holdemMaxPlayers; i++ {
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
	client.RecordResult("holdem", "lose")

	// [턴 넘김] 퇴장자가 현재 차례였다면 폴드 처리 후 턴 진행
	if g.currentPlayerIdx == idx {
		g.stopTurnTimerLocked()
		g.foldedThisRound[idx] = true
		g.actedThisPhase[idx] = true
		g.advanceTurnLocked()
	}

	// [생존자 체크] 별 1개 이상인 플레이어 수
	survivorCount := 0
	for i := 0; i < holdemMaxPlayers; i++ {
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
		for i := 0; i < holdemMaxPlayers; i++ {
			if g.players[i] != nil && g.stars[i] >= 1 {
				g.players[i].RecordResult("holdem", "win")
				break
			}
		}
	}
	g.stopTurnTimerLocked()
	survivors := ""
	for i := 0; i < holdemMaxPlayers; i++ {
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
func (g *HoldemGame) HandleAction(client *Client, action string, payload json.RawMessage) {
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
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 홀덤 명령: [%s]", p.Cmd),
		})
	}
}

func (g *HoldemGame) handleCheck(client *Client) {
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

	// 체크: 별 1개 지불 (별 0개면 무료)
	cost := 0
	if g.stars[idx] > 0 {
		cost = holdemCheckCost
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

func (g *HoldemGame) handleFold(client *Client) {
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

func (g *HoldemGame) playerIndex(c *Client) int {
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] == c {
			return i
		}
	}
	return -1
}

// advanceTurnLocked는 다음 플레이어로 넘기거나, 페이즈/라운드를 진행합니다.
func (g *HoldemGame) advanceTurnLocked() {
	// 단독 생존자 체크 (나머지 모두 폴드)
	activeCount := 0
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			activeCount++
		}
	}
	if activeCount == 1 {
		g.resolveShowdownLocked()
		return
	}

	// 이번 페이즈에서 아직 액션 안 한 생존자 확인
	nextIdx := -1
	for i := 1; i <= holdemMaxPlayers; i++ {
		idx := (g.currentPlayerIdx + i) % holdemMaxPlayers
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

	// 모두 액션 완료 → 다음 페이즈
	g.nextPhaseLocked()
}

func (g *HoldemGame) nextPhaseLocked() {
	switch g.phase {
	case "preflop":
		// 플랍: 커뮤니티 카드 3장
		g.communityCards[0] = g.deck[0]
		g.communityCards[1] = g.deck[1]
		g.communityCards[2] = g.deck[2]
		g.deck = g.deck[3:]
		g.phase = "flop"
		g.resetActedLocked()
		g.setFirstActivePlayerLocked()
		g.startTurnTimerLocked()
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "── 플랍 (커뮤니티 카드 3장) ──",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

	case "flop":
		g.communityCards[3] = g.deck[0]
		g.deck = g.deck[1:]
		g.phase = "turn"
		g.resetActedLocked()
		g.setFirstActivePlayerLocked()
		g.startTurnTimerLocked()
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "── 턴 (커뮤니티 카드 +1장) ──",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

	case "turn":
		g.communityCards[4] = g.deck[0]
		g.deck = g.deck[1:]
		g.phase = "river"
		g.resetActedLocked()
		g.setFirstActivePlayerLocked()
		g.startTurnTimerLocked()
		notice, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "── 리버 (커뮤니티 카드 +1장) ──",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice)

	case "river":
		g.phase = "showdown"
		g.resolveShowdownLocked()
		return
	default:
		g.sendStateToAllLocked()
		return
	}
	g.sendStateToAllLocked()
}

func (g *HoldemGame) resetActedLocked() {
	for i := 0; i < holdemMaxPlayers; i++ {
		g.actedThisPhase[i] = false
	}
}

func (g *HoldemGame) setFirstActivePlayerLocked() {
	for i := 1; i <= holdemMaxPlayers; i++ {
		idx := (g.dealerIdx + i) % holdemMaxPlayers
		if g.players[idx] != nil && !g.foldedThisRound[idx] {
			g.currentPlayerIdx = idx
			return
		}
	}
}

func (g *HoldemGame) resolveShowdownLocked() {
	// 생존자들의 7장 카드로 족보 비교
	survivors := make([]int, 0)
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			survivors = append(survivors, i)
		}
	}

	totalPot := g.pot + g.potCarryOver
	g.pot = 0
	g.potCarryOver = 0

	if len(survivors) == 0 {
		// 모두 폴드 (비정상)
		g.startRoundLocked()
		return
	}

	if len(survivors) == 1 {
		// 단독 생존자 승리
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

	// 족보 비교
	cards7 := make([][]Card, len(survivors))
	scores := make([]int64, len(survivors))
	for i, idx := range survivors {
		cards7[i] = append([]Card{}, g.holeCards[idx][0], g.holeCards[idx][1])
		for j := 0; j < 5; j++ {
			cards7[i] = append(cards7[i], g.communityCards[j])
		}
		scores[i] = EvaluateHand(cards7[i])
	}

	// 최고 점수 찾기
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

	// 팟 분배
	share := totalPot / len(winners)
	remainder := totalPot % len(winners)
	for _, idx := range winners {
		g.stars[idx] += share
	}
	g.potCarryOver = remainder

	winningHandName := HandRankWithDetail(bestScore)
	winReason := HandWinReason(bestScore)
	participants := make([]PokerShowdownParticipant, len(survivors))
	for i, idx := range survivors {
		p := PokerShowdownParticipant{
			UserID:   g.players[idx].UserID,
			HandName: HandRankWithDetail(scores[i]),
			WinReason: HandWinReason(scores[i]),
		}
		participants[i] = p
	}
	showdownData, _ := json.Marshal(map[string]any{
		"type":   "poker_showdown_result",
		"roomId": g.room.ID,
		"data": map[string]any{
			"winnerId":     g.players[winners[0]].UserID,
			"winningHand":  winningHandName,
			"winReason":    winReason,
			"participants": participants,
		},
	})
	g.room.broadcastAll(showdownData)

	winnerNames := ""
	for i, idx := range winners {
		if i > 0 {
			winnerNames += ", "
		}
		winnerNames += g.players[idx].UserID
	}
	msg := fmt.Sprintf("🏆 [%s] 승리! 팟 ⭐×%d 분배 (나머지 %d 이월)", winnerNames, share, remainder)
	if remainder > 0 {
		msg = fmt.Sprintf("🏆 [%s] 동점! 팟 ⭐×%d씩 분배, 나머지 %d 다음 라운드 이월", winnerNames, share, remainder)
	}
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: msg,
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.afterRoundLocked()
}

func (g *HoldemGame) afterRoundLocked() {
	// 파산자(별 0개) 계산
	bankruptCount := 0
	totalCount := 0
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil {
			totalCount++
			if g.stars[i] <= 0 {
				bankruptCount++
			}
		}
	}

	// 종료 조건: 파산자 >= ceil(전체/2)
	threshold := int(math.Ceil(float64(totalCount) / 2))
	if bankruptCount >= threshold {
		g.endMatchLocked()
		return
	}

	g.startRoundLocked()
}

func (g *HoldemGame) endMatchLocked() {
	g.stopTurnTimerLocked()
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		if g.stars[i] > 0 {
			g.players[i].RecordResult("holdem", "win")
		} else {
			g.players[i].RecordResult("holdem", "lose")
		}
	}

	survivors := ""
	for i := 0; i < holdemMaxPlayers; i++ {
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
	log.Printf("[HOLDEM] room:[%s] 매치 종료", g.room.ID)

	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	for i := 0; i < holdemMaxPlayers; i++ {
		g.stars[i] = 0
		g.holeCards[i] = [2]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
	}
	g.communityCards = [5]Card{}
	g.deck = nil
}

func (g *HoldemGame) startRoundLocked() {
	// 별 0개인 플레이어는 이번 라운드 제외 (이미 파산)
	activeCount := 0
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			activeCount++
		}
	}
	if activeCount < 2 {
		g.sendStateToAllLocked()
		return
	}

	g.round++
	g.phase = "preflop"
	g.pot += g.potCarryOver
	g.potCarryOver = 0

	for i := 0; i < holdemMaxPlayers; i++ {
		g.foldedThisRound[i] = g.players[i] == nil || g.stars[i] <= 0 // 파산자는 라운드 제외
		g.actedThisPhase[i] = false
	}

	// 딜러 버튼 이동 (다음 생존 플레이어)
	for i := 1; i <= holdemMaxPlayers; i++ {
		idx := (g.dealerIdx + i) % holdemMaxPlayers
		if g.players[idx] != nil && g.stars[idx] > 0 {
			g.dealerIdx = idx
			break
		}
	}

	// 덱 셔플 및 카드 배분
	g.deck = NewShuffledDeck()
	cardIdx := 0
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && g.stars[i] > 0 {
			g.holeCards[i][0] = g.deck[cardIdx]
			g.holeCards[i][1] = g.deck[cardIdx+1]
			cardIdx += 2
		}
	}
	g.deck = g.deck[cardIdx:]
	g.communityCards = [5]Card{}

	g.setFirstActivePlayerLocked()
	g.startTurnTimerLocked()

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("── 라운드 %d 시작! 프리플랍 (개인 카드 2장) ──", g.round),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.sendStateToAllLocked()
}

func (g *HoldemGame) startTurnTimerLocked() {
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
		Remaining: holdemTurnTimeLimit,
	})
	g.room.broadcastAll(data)
	go func() {
		remaining := holdemTurnTimeLimit
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

func (g *HoldemGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *HoldemGame) handleTimeOver(timedOutPlayer *Client) {
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

func (g *HoldemGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}
	g.rematchReady[client] = true
	total := 0
	ready := 0
	for i := 0; i < holdemMaxPlayers; i++ {
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
		for i := 0; i < holdemMaxPlayers; i++ {
			if g.players[i] != nil {
				g.stars[i] = holdemStartStars
			}
		}
		g.pot = 0
		g.potCarryOver = 0
		g.communityCards = [5]Card{}
		for i := 0; i < holdemMaxPlayers; i++ {
			g.holeCards[i] = [2]Card{}
			g.foldedThisRound[i] = false
			g.actedThisPhase[i] = false
		}
		g.gameStarted = true
		g.startRoundLocked()
	}
}

// handleReady는 게임 시작 전 준비 요청을 처리합니다.
func (g *HoldemGame) handleReady(client *Client) {
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
	for i := 0; i < holdemMaxPlayers; i++ {
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
func (g *HoldemGame) resetForLeaveLocked() {
	g.stopTurnTimerLocked()
	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	g.deck = nil
	g.dealerIdx = 0
	for i := 0; i < holdemMaxPlayers; i++ {
		g.holeCards[i] = [2]Card{}
		g.foldedThisRound[i] = false
		g.actedThisPhase[i] = false
	}
	g.communityCards = [5]Card{}
}

// ── 상태 전송 ─────────────────────────────────────────────────────────────────

func (g *HoldemGame) sendStateToAllLocked() {
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

func (g *HoldemGame) buildHoldemDataForPlayer(viewerIdx int) HoldemData {
	phase := g.phase
	if phase == "" {
		phase = "waiting"
	}
	communityVisible := 0
	switch g.phase {
	case "flop":
		communityVisible = 3
	case "turn":
		communityVisible = 4
	case "river", "showdown":
		communityVisible = 5
	}

	communityCards := make([]Card, 5)
	for i := 0; i < 5; i++ {
		if i < communityVisible {
			communityCards[i] = g.communityCards[i]
		} else {
			communityCards[i] = Card{Hidden: true}
		}
	}

	players := make([]HoldemPlayerInfo, 0)
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		status := ""
		if g.foldedThisRound[i] {
			status = "fold"
		} else if g.actedThisPhase[i] {
			status = "check"
		}

		cards := make([]Card, 2)
		for j := 0; j < 2; j++ {
			cards[j] = g.holeCards[i][j]
			if i != viewerIdx {
				cards[j].Hidden = true
			}
		}

		players = append(players, HoldemPlayerInfo{
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

	canTakeover := false
	if viewerIdx < 0 && phase == "waiting" && !g.gameStarted {
		for i := 0; i < holdemMaxPlayers; i++ {
			if g.players[i] == nil {
				canTakeover = true
				break
			}
		}
	}

	myHandName := ""
	if viewerIdx >= 0 && !g.foldedThisRound[viewerIdx] {
		cards7 := make([]Card, 0, 7)
		for _, c := range []Card{g.holeCards[viewerIdx][0], g.holeCards[viewerIdx][1]} {
			if c.Suit != "" || c.Value != "" {
				cards7 = append(cards7, c)
			}
		}
		for j := 0; j < communityVisible; j++ {
			c := g.communityCards[j]
			if c.Suit != "" || c.Value != "" {
				cards7 = append(cards7, c)
			}
		}
		if len(cards7) >= 5 {
			score := EvaluateHand(cards7)
			myHandName = PokerHandDisplayName(HandRankName(score))
		}
	}

	return HoldemData{
		Phase:          phase,
		Round:          g.round,
		Pot:            g.pot + g.potCarryOver,
		CommunityCards: communityCards,
		Players:        players,
		CurrentTurn:    currentTurn,
		CanTakeover:    canTakeover,
		MyHandName:     myHandName,
	}
}

func (g *HoldemGame) sendStateToPlayerLocked(client *Client, playerIdx int) {
	data := g.buildHoldemDataForPlayer(playerIdx)
	client.SendJSON(HoldemStateResponse{
		Type:   "holdem_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *HoldemGame) sendStateToSpectatorLocked(client *Client) {
	data := g.buildHoldemDataForPlayer(-1) // 관전자는 모든 카드 뒷면
	client.SendJSON(HoldemStateResponse{
		Type:   "holdem_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *HoldemGame) handleTakeover(client *Client) {
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
	for i := 0; i < holdemMaxPlayers; i++ {
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
	g.stars[slot] = holdemStartStars
	g.playerCount++

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("🪑 [%s]님이 빈자리에 참여했습니다. (%d/4)", client.UserID, g.playerCount),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	total := g.playerCount
	ready := 0
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && g.startReady[g.players[i]] {
			ready++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
	log.Printf("[HOLDEM] room:[%s] [%s] 빈자리 참여 (슬롯 %d)", g.room.ID, client.UserID, slot)
}
