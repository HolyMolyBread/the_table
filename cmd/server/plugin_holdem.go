package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	holdemMaxPlayers     = 4
	holdemStartStars     = 10
	holdemCheckCost      = 1
	holdemTurnTimeLimit  = 15
)

// ── 족보 판정 ─────────────────────────────────────────────────────────────────

// holdemCardRank는 카드 숫자 값을 반환합니다. 2=2, ..., A=14
func holdemCardRank(c Card) int {
	m := map[string]int{
		"2": 2, "3": 3, "4": 4, "5": 5, "6": 6,
		"7": 7, "8": 8, "9": 9, "10": 10,
		"J": 11, "Q": 12, "K": 13, "A": 14,
	}
	if v, ok := m[c.Value]; ok {
		return v
	}
	return 0
}

// holdemSuitRank는 문양 순위 (동점 시 비교용). ♣<♦<♥<♠
func holdemSuitRank(c Card) int {
	m := map[string]int{"♣": 1, "♦": 2, "♥": 3, "♠": 4}
	if v, ok := m[c.Suit]; ok {
		return v
	}
	return 0
}

// evaluateHand는 7장 카드에서 최고 5장 족보 점수를 반환합니다.
// 점수: (족보등급 << 20) | (타이브레이크 값들)
// 족보: 로티플(10) > 스트레이트플러시(9) > 포카드(8) > 풀하우스(7) > 플러시(6) > 스트레이트(5) > 트리플(4) > 투페어(3) > 원페어(2) > 하이카드(1)
func evaluateHand(cards []Card) int64 {
	if len(cards) < 5 {
		return 0
	}
	// 7장에서 5장 조합 C(7,5)=21가지 모두 평가
	best := int64(0)
	indices := make([]int, 5)
	var comb func(start, depth int)
	comb = func(start, depth int) {
		if depth == 5 {
			five := make([]Card, 5)
			for i, idx := range indices {
				five[i] = cards[idx]
			}
			score := evalFive(five)
			if score > best {
				best = score
			}
			return
		}
		for i := start; i <= len(cards)-5+depth; i++ {
			indices[depth] = i
			comb(i+1, depth+1)
		}
	}
	comb(0, 0)
	return best
}

func evalFive(cards []Card) int64 {
	ranks := make([]int, 5)
	suits := make([]int, 5)
	for i, c := range cards {
		ranks[i] = holdemCardRank(c)
		suits[i] = holdemSuitRank(c)
	}
	sort.Slice(cards, func(i, j int) bool {
		ri, rj := holdemCardRank(cards[i]), holdemCardRank(cards[j])
		if ri != rj {
			return ri > rj
		}
		return holdemSuitRank(cards[i]) > holdemSuitRank(cards[j])
	})
	rankCounts := make(map[int]int)
	for _, r := range ranks {
		rankCounts[r]++
	}
	sortedRanks := make([]int, len(ranks))
	copy(sortedRanks, ranks)
	sort.Sort(sort.Reverse(sort.IntSlice(sortedRanks)))

	flush := suits[0] == suits[1] && suits[1] == suits[2] && suits[2] == suits[3] && suits[3] == suits[4]
	straightVal := straightValue(sortedRanks)

	// 로티플: 10-J-Q-K-A 동일 문양
	if flush && straightVal == 14 {
		return (10 << 20) | int64(sortedRanks[0])
	}
	// 스트레이트 플러시
	if flush && straightVal > 0 {
		return (9 << 20) | int64(straightVal)
	}
	// 포카드
	for r, cnt := range rankCounts {
		if cnt == 4 {
			kicker := 0
			for _, v := range sortedRanks {
				if v != r {
					kicker = v
					break
				}
			}
			return (8 << 20) | (int64(r) << 8) | int64(kicker)
		}
	}
	// 풀하우스
	var trip, pair int
	for r, cnt := range rankCounts {
		if cnt == 3 {
			trip = r
		}
		if cnt == 2 {
			if pair == 0 || r > pair {
				pair = r
			}
		}
	}
	if trip > 0 && pair > 0 {
		return (7 << 20) | (int64(trip) << 8) | int64(pair)
	}
	// 플러시
	if flush {
		score := int64(6 << 20)
		for i, r := range sortedRanks {
			score |= int64(r) << (uint(4-i) * 4)
		}
		return score
	}
	// 스트레이트
	if straightVal > 0 {
		return (5 << 20) | int64(straightVal)
	}
	// 트리플
	if trip > 0 {
		kickers := make([]int, 0)
		for _, r := range sortedRanks {
			if r != trip {
				kickers = append(kickers, r)
			}
		}
		score := (4 << 20) | (int64(trip) << 12)
		for i, k := range kickers {
			if i < 2 {
				score |= int64(k) << (uint(1-i) * 4)
			}
		}
		return score
	}
	// 투페어
	pairs := make([]int, 0)
	for r, cnt := range rankCounts {
		if cnt == 2 {
			pairs = append(pairs, r)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))
	if len(pairs) >= 2 {
		kicker := 0
		for _, r := range sortedRanks {
			if r != pairs[0] && r != pairs[1] {
				kicker = r
				break
			}
		}
		return (3 << 20) | (int64(pairs[0]) << 12) | (int64(pairs[1]) << 8) | int64(kicker)
	}
	// 원페어
	if len(pairs) == 1 {
		kickers := make([]int, 0)
		for _, r := range sortedRanks {
			if r != pairs[0] {
				kickers = append(kickers, r)
			}
		}
		score := (2 << 20) | (int64(pairs[0]) << 12)
		for i, k := range kickers {
			if i < 3 {
				score |= int64(k) << (uint(2-i) * 4)
			}
		}
		return score
	}
	// 하이카드
	score := int64(1 << 20)
	for i, r := range sortedRanks {
		if i < 5 {
			score |= int64(r) << (uint(4-i) * 4)
		}
	}
	return score
}

// handRankName은 족보 점수에서 한글 족보명을 반환합니다.
func handRankName(score int64) string {
	rank := int(score >> 20)
	names := map[int]string{
		10: "로티플", 9: "스트레이트플러시", 8: "포카드", 7: "풀하우스",
		6: "플러시", 5: "스트레이트", 4: "트리플", 3: "투페어",
		2: "원페어", 1: "하이카드",
	}
	if n, ok := names[rank]; ok {
		return n
	}
	return "하이카드"
}

// straightValue는 정렬된 랭크에서 스트레이트 최고값. 없으면 0.
func straightValue(sorted []int) int {
	unique := make([]int, 0)
	seen := make(map[int]bool)
	for _, r := range sorted {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(unique)))
	for i := 0; i <= len(unique)-5; i++ {
		if unique[i]-unique[i+4] == 4 {
			return unique[i]
		}
	}
	// A-2-3-4-5 (휠)
	if seen[14] && seen[2] && seen[3] && seen[4] && seen[5] {
		return 5
	}
	return 0
}

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
	Phase         string             `json:"phase"`
	Round         int                `json:"round"`
	Pot           int                `json:"pot"`
	CommunityCards []Card            `json:"communityCards"`
	Players       []HoldemPlayerInfo `json:"players"`
	CurrentTurn   string             `json:"currentTurn"`
	Message       string             `json:"message,omitempty"`
}

// HoldemStateResponse는 홀덤 게임 상태 응답입니다.
type HoldemStateResponse struct {
	Type   string     `json:"type"`
	RoomID string     `json:"roomId"`
	Data   HoldemData `json:"data"`
}

// PokerShowdownParticipant는 쇼다운 참가자 정보입니다.
type PokerShowdownParticipant struct {
	UserID   string `json:"userId"`
	HandName string `json:"handName"`
}

// PokerShowdownResultData는 poker_showdown_result 메시지의 data 필드입니다.
type PokerShowdownResultData struct {
	WinnerID     string                    `json:"winnerId"`
	WinningHand  string                    `json:"winningHand"`
	Participants []PokerShowdownParticipant `json:"participants"`
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
	currentPlayerIdx int
	gameStarted      bool
	playerCount      int
	rematchReady     map[*Client]bool
	stopTick         chan struct{}
	mu               sync.Mutex
}

// NewHoldemGame creates a new Holdem game plugin.
func NewHoldemGame(room *Room) *HoldemGame {
	return &HoldemGame{room: room, phase: "waiting", rematchReady: make(map[*Client]bool)}
}

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

	if g.playerCount >= 2 && !g.gameStarted {
		g.gameStarted = true
		g.startRoundLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

// OnLeave는 플레이어 퇴장 시 호출됩니다.
func (g *HoldemGame) OnLeave(client *Client) {
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
	delete(g.rematchReady, client)

	if g.gameStarted {
		// 생존자에게 승리, 퇴장자에게 패배 기록
		for i := 0; i < holdemMaxPlayers; i++ {
			if i == idx {
				client.RecordResult("holdem", "lose")
			} else if g.players[i] != nil && g.stars[i] > 0 {
				g.players[i].RecordResult("holdem", "win")
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
	case "rematch":
		g.handleRematch(client)
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
	for i := 0; i < holdemMaxPlayers; i++ {
		if g.players[i] != nil && !g.foldedThisRound[i] {
			g.currentPlayerIdx = i
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
		scores[i] = evaluateHand(cards7[i])
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

	winningHandName := handRankName(bestScore)
	participants := make([]PokerShowdownParticipant, len(survivors))
	for i, idx := range survivors {
		participants[i] = PokerShowdownParticipant{
			UserID:   g.players[idx].UserID,
			HandName: handRankName(scores[i]),
		}
	}
	showdownData, _ := json.Marshal(map[string]any{
		"type":   "poker_showdown_result",
		"roomId": g.room.ID,
		"data": map[string]any{
			"winnerId":     g.players[winners[0]].UserID,
			"winningHand":  winningHandName,
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

	// 덱 셔플 및 카드 배분
	g.deck = newShuffledDeck()
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
				g.rematchReady[g.players[i]] = false
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

func (g *HoldemGame) resetForLeaveLocked(leaveIdx int) {
	g.players[leaveIdx] = nil
	g.stars[leaveIdx] = 0
	g.playerCount--
	g.gameStarted = false
	g.phase = "waiting"
	g.round = 0
	g.pot = 0
	g.potCarryOver = 0
	g.deck = nil
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

	return HoldemData{
		Phase:          phase,
		Round:          g.round,
		Pot:            g.pot + g.potCarryOver,
		CommunityCards: communityCards,
		Players:        players,
		CurrentTurn:    currentTurn,
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
