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
	thiefMaxPlayers    = 4
	thiefTurnTimeLimit  = 15
)

var thiefJoker = Card{Suit: "🃏", Value: "JOKER"}

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// ThiefData는 thief_state 응답의 data 필드입니다.
type ThiefData struct {
	Hand        []Card `json:"hand"`        // 내 패 (앞면)
	Turn        string `json:"turn"`       // 현재 차례 유저 ID
	Players     []ThiefPlayerInfo `json:"players"`     // 전체 플레이어 (패 수 등)
	Escaped     []string `json:"escaped"`   // 탈출한 유저 ID 목록
	Message     string   `json:"message,omitempty"`
}

// ThiefPlayerInfo는 한 플레이어의 공개 정보입니다.
type ThiefPlayerInfo struct {
	UserID   string `json:"userId"`
	CardCount int   `json:"cardCount"` // 패 수 (내 패만 실제 카드, 타인은 숫자만)
}

// ThiefStateResponse는 도둑잡기 게임 상태 응답입니다.
type ThiefStateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   ThiefData `json:"data"`
}

// ── ThiefGame 플러그인 ────────────────────────────────────────────────────────

// ThiefGame은 2~4인 도둑잡기 게임 플러그인입니다.
// 53장(52+조커) 분배 후 페어 제거. 턴마다 다음 생존자 패에서 1장 뽑기. 패 0장이면 탈출(Win). 조커만 남은 1명이 패배(Lose).
type ThiefGame struct {
	room         *Room
	players      [thiefMaxPlayers]*Client
	hands        [thiefMaxPlayers][]Card
	escaped      [thiefMaxPlayers]bool
	currentTurn  int
	playerCount  int
	gameStarted  bool
	stopTick     chan struct{}
	startReady   map[*Client]bool
	rematchReady map[*Client]bool
	mu           sync.Mutex
}

func NewThiefGame(room *Room) *ThiefGame {
	return &ThiefGame{room: room, startReady: make(map[*Client]bool), rematchReady: make(map[*Client]bool)}
}

func (g *ThiefGame) Name() string { return "thief" }

// OnJoin은 플레이어 입장 시 호출됩니다.
func (g *ThiefGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] == client {
			g.sendStateToAllLocked()
			return
		}
	}

	slot := -1
	for i := 0; i < thiefMaxPlayers; i++ {
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
		Message: fmt.Sprintf("🃏 [%s]님이 입장했습니다. (%d/%d)", client.UserID, g.playerCount, thiefMaxPlayers),
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
func (g *ThiefGame) OnLeave(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < thiefMaxPlayers; i++ {
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

	g.hands[idx] = nil
	g.players[idx] = nil
	g.playerCount--
	remaining := g.playerCount

	if !g.gameStarted {
		readyCount := 0
		for i := 0; i < thiefMaxPlayers; i++ {
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
	client.RecordResult("thief", "lose")

	// 퇴장자가 현재 차례였다면 타이머 정지 후 턴 진행
	if idx == g.currentTurn {
		g.stopTurnTimerLocked()
		g.advanceTurnLocked()
	}

	// 생존자(패 보유 또는 탈출) 수
	survivorCount := 0
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && (len(g.hands[i]) > 0 || g.escaped[i]) {
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
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && g.escaped[i] {
			g.players[i].RecordResult("thief", "win")
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
	log.Printf("[THIEF] room:[%s] [%s] 퇴장 — 매치 종료", g.room.ID, client.UserID)
}

// HandleAction은 game_action 메시지를 처리합니다.
func (g *ThiefGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "draw":
		g.handleDraw(client)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 도둑잡기 명령: [%s]", p.Cmd),
		})
	}
}

func (g *ThiefGame) handleDraw(client *Client) {
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
	if g.escaped[idx] {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 탈출했습니다."})
		return
	}

	// 다음 생존 플레이어 찾기
	targetIdx := -1
	for i := 1; i <= thiefMaxPlayers; i++ {
		next := (g.currentTurn + i) % thiefMaxPlayers
		if g.players[next] != nil && !g.escaped[next] && len(g.hands[next]) > 0 {
			targetIdx = next
			break
		}
	}
	if targetIdx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "뽑을 상대가 없습니다."})
		return
	}

	// 타겟 패에서 무작위 1장 뽑기
	targetHand := g.hands[targetIdx]
	drawIdx := rand.Intn(len(targetHand))
	drawn := targetHand[drawIdx]
	g.hands[targetIdx] = append(targetHand[:drawIdx], targetHand[drawIdx+1:]...)
	g.hands[idx] = append(g.hands[idx], drawn)

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s]이 [%s]의 패에서 카드 1장을 뽑았습니다.", client.UserID, g.players[targetIdx].UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	// 페어 제거 (같은 숫자 2장)
	g.removePairsLocked(idx)

	// 탈출 체크 (패 0장)
	if len(g.hands[idx]) == 0 {
		g.escaped[idx] = true
		client.RecordResult("thief", "win")
		notice2, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: fmt.Sprintf("🏆 [%s] 탈출 성공!", client.UserID),
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice2)
	}

	// 게임 종료 체크: 조커만 남은 1명
	remaining := 0
	loserIdx := -1
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && !g.escaped[i] {
			remaining++
			if len(g.hands[i]) == 1 && g.hands[i][0].Value == "JOKER" {
				loserIdx = i
			}
		}
	}
	if remaining == 1 && loserIdx >= 0 {
		g.players[loserIdx].RecordResult("thief", "lose")
		for i := 0; i < thiefMaxPlayers; i++ {
			if g.players[i] != nil && i != loserIdx && g.escaped[i] {
				g.players[i].RecordResult("thief", "win")
			}
		}
		msg := fmt.Sprintf("🃏 [%s]가 조커를 들고 남아 패배! 탈출한 플레이어 승리!", g.players[loserIdx].UserID)
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
		log.Printf("[THIEF] room:[%s] 게임 종료: loser=[%s]", g.room.ID, g.players[loserIdx].UserID)
		g.gameStarted = false
		g.stopTurnTimerLocked()
		return
	}

	// 다음 턴으로
	g.advanceTurnLocked()
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *ThiefGame) removePairsLocked(playerIdx int) {
	hand := g.hands[playerIdx]
	for {
		removed := false
		for i := 0; i < len(hand) && !removed; i++ {
			for j := i + 1; j < len(hand); j++ {
				if hand[i].Value == hand[j].Value {
					hand = append(hand[:j], hand[j+1:]...)
					hand = append(hand[:i], hand[i+1:]...)
					removed = true
					break
				}
			}
		}
		if !removed {
			break
		}
	}
	g.hands[playerIdx] = hand
}

func (g *ThiefGame) advanceTurnLocked() {
	for i := 1; i <= thiefMaxPlayers; i++ {
		next := (g.currentTurn + i) % thiefMaxPlayers
		if g.players[next] != nil && !g.escaped[next] && len(g.hands[next]) > 0 {
			g.currentTurn = next
			return
		}
	}
}

func (g *ThiefGame) playerIndex(c *Client) int {
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] == c {
			return i
		}
	}
	return -1
}

func (g *ThiefGame) startGameLocked() {
	deck := NewShuffledDeck()
	deck = append(deck, thiefJoker)
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	// 라운드 로빈으로 분배
	playerIndices := make([]int, 0, thiefMaxPlayers)
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil {
			playerIndices = append(playerIndices, i)
		}
	}
	for i, c := range deck {
		pi := playerIndices[i%len(playerIndices)]
		g.hands[pi] = append(g.hands[pi], c)
	}
	for _, pi := range playerIndices {
		g.removePairsLocked(pi)
		if len(g.hands[pi]) == 0 {
			g.escaped[pi] = true
			g.players[pi].RecordResult("thief", "win")
		}
	}

	g.currentTurn = 0
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && !g.escaped[i] && len(g.hands[i]) > 0 {
			g.currentTurn = i
			break
		}
	}

	// 게임 즉시 종료: 조커만 남은 1명
	remaining := 0
	loserIdx := -1
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && !g.escaped[i] {
			remaining++
			if len(g.hands[i]) == 1 && g.hands[i][0].Value == "JOKER" {
				loserIdx = i
			}
		}
	}
	if remaining == 1 && loserIdx >= 0 {
		g.players[loserIdx].RecordResult("thief", "lose")
		for i := 0; i < thiefMaxPlayers; i++ {
			if g.players[i] != nil && i != loserIdx && g.escaped[i] {
				g.players[i].RecordResult("thief", "win")
			}
		}
		msg := fmt.Sprintf("🃏 [%s]가 조커를 들고 남아 패배!", g.players[loserIdx].UserID)
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
		return
	}

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: "도둑잡기 시작! 페어를 제거하고, 다음 플레이어 패에서 카드를 뽑으세요. 패가 0장이면 탈출!",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *ThiefGame) startTurnTimerLocked() {
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
		Remaining: thiefTurnTimeLimit,
	})
	g.room.broadcastAll(data)
	go func() {
		remaining := thiefTurnTimeLimit
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

func (g *ThiefGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *ThiefGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	if !g.gameStarted || g.players[g.currentTurn] != timedOutPlayer {
		g.mu.Unlock()
		return
	}
	g.stopTurnTimerLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("⏰ [%s] 시간 초과! 자동으로 카드를 뽑습니다.", timedOutPlayer.UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.mu.Unlock()
	g.handleDraw(timedOutPlayer)
}

func (g *ThiefGame) handleReady(client *Client) {
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
	for i := 0; i < thiefMaxPlayers; i++ {
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

func (g *ThiefGame) handleRematch(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gameStarted {
		client.SendJSON(ServerResponse{Type: "error", Message: "게임 진행 중에는 리매치를 요청할 수 없습니다."})
		return
	}
	g.rematchReady[client] = true
	total := 0
	ready := 0
	for i := 0; i < thiefMaxPlayers; i++ {
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
		for i := 0; i < thiefMaxPlayers; i++ {
			g.hands[i] = nil
			g.escaped[i] = false
		}
		g.gameStarted = true
		g.startGameLocked()
	}
}

func (g *ThiefGame) sendStateToAllLocked() {
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

func (g *ThiefGame) sendStateToClientLocked(client *Client) {
	idx := g.playerIndex(client)
	if idx < 0 {
		return
	}

	players := make([]ThiefPlayerInfo, 0)
	escaped := make([]string, 0)
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		players = append(players, ThiefPlayerInfo{
			UserID:    g.players[i].UserID,
			CardCount: len(g.hands[i]),
		})
		if g.escaped[i] {
			escaped = append(escaped, g.players[i].UserID)
		}
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

	data := ThiefData{
		Hand:    myHand,
		Turn:    turnUser,
		Players: players,
		Escaped: escaped,
	}
	client.SendJSON(ThiefStateResponse{
		Type:   "thief_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}
