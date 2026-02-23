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
	Hand         []Card `json:"hand"`         // 내 패 (앞면)
	Turn         string `json:"turn"`         // 현재 차례 유저 ID
	TargetUserID string `json:"targetUserId"` // 내 차례일 때 뽑을 대상 유저 ID (비어 있으면 해당 없음)
	Players      []ThiefPlayerInfo `json:"players"`      // 전체 플레이어 (패 수 등)
	Escaped      []string `json:"escaped"`    // 탈출한 유저 ID 목록
	Message      string   `json:"message,omitempty"`
	CanTakeover  bool     `json:"canTakeover,omitempty"`
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
	targetIdx    int // 시계방향 다음 플레이어 (카드를 뺏길 대상)
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

func init() { RegisterPlugin("thief", func(room *Room) GamePlugin { return NewThiefGame(room) }) }

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
		g.sendStateToSpectatorLocked(client)
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
func (g *ThiefGame) OnLeave(client *Client, remainingCount int) {
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
		Cmd      string `json:"cmd"`
		TargetID string `json:"targetId"`
		Index    int    `json:"index"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "draw":
		g.handleDraw(client, p.TargetID, p.Index)
	case "hover":
		g.handleHover(client, p.TargetID, p.Index)
	case "ready":
		g.handleReady(client)
	case "rematch":
		g.handleRematch(client)
	case "takeover":
		g.handleTakeover(client)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 도둑잡기 명령: [%s]", p.Cmd),
		})
	}
}

func (g *ThiefGame) handleHover(client *Client, targetID string, index int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	idx := g.playerIndex(client)
	if idx < 0 || g.players[g.currentTurn] != client {
		return
	}
	if g.targetIdx < 0 || g.players[g.targetIdx] == nil || g.players[g.targetIdx].UserID != targetID {
		return
	}
	if index < 0 || index >= len(g.hands[g.targetIdx]) {
		return
	}
	msg, _ := json.Marshal(map[string]any{
		"type":     "thief_hover",
		"userId":   client.UserID,
		"targetId": targetID,
		"index":    index,
	})
	g.room.broadcastAll(msg)
}

func (g *ThiefGame) handleDraw(client *Client, targetID string, drawIndex int) {
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

	// 시계방향 강제: g.targetIdx만 유효한 대상
	if g.targetIdx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "뽑을 상대가 없습니다."})
		return
	}
	if targetID != "" && g.players[g.targetIdx].UserID != targetID {
		client.SendJSON(ServerResponse{Type: "error", Message: "잘못된 대상입니다. 시계방향 다음 플레이어만 선택할 수 있습니다."})
		return
	}

	targetHand := g.hands[g.targetIdx]
	var drawIdx int
	if targetID != "" && drawIndex >= 0 && drawIndex < len(targetHand) {
		drawIdx = drawIndex
	} else {
		drawIdx = rand.Intn(len(targetHand))
	}
	drawn := targetHand[drawIdx]
	g.hands[g.targetIdx] = append(targetHand[:drawIdx], targetHand[drawIdx+1:]...)
	g.hands[idx] = append(g.hands[idx], drawn)

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s]이 [%s]의 패에서 카드 1장을 뽑았습니다.", client.UserID, g.players[g.targetIdx].UserID),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	// 카드 추가된 상태로 먼저 전송 (페어 제거 전)
	g.sendStateToAllLocked()

	// 1.5초 후 페어 제거 및 턴 진행
	room := g.room
	time.AfterFunc(1500*time.Millisecond, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if !g.gameStarted {
			return
		}
		lenBefore := len(g.hands[idx])
		g.removePairsLocked(idx)
		lenAfter := len(g.hands[idx])
		if lenBefore != lenAfter {
			notice2, _ := json.Marshal(ServerResponse{
				Type:    "game_notice",
				Message: fmt.Sprintf("✨ [%s] 짝을 맞춰 버렸습니다!", client.UserID),
				RoomID:  g.room.ID,
			})
			g.room.broadcastAll(notice2)
			g.sendStateToAllLocked()
		}

		// 탈출 체크 (패 0장)
		if len(g.hands[idx]) == 0 {
			g.escaped[idx] = true
			client.RecordResult("thief", "win")
			notice3, _ := json.Marshal(ServerResponse{
				Type:    "game_notice",
				Message: fmt.Sprintf("🏆 [%s] 탈출 성공!", client.UserID),
				RoomID:  g.room.ID,
			})
			g.room.broadcastAll(notice3)
		}

		// 게임 종료 체크: activeCount<=1 (조커만 남은 1명 또는 뽑을 상대 없음)
		activeCount := 0
		loserIdx := -1
		for i := 0; i < thiefMaxPlayers; i++ {
			if g.players[i] != nil && !g.escaped[i] && len(g.hands[i]) > 0 {
				activeCount++
				if len(g.hands[i]) == 1 && g.hands[i][0].Value == "JOKER" {
					loserIdx = i
				}
			}
		}
		if activeCount <= 1 {
			var msg string
			if loserIdx >= 0 {
				g.players[loserIdx].RecordResult("thief", "lose")
				for i := 0; i < thiefMaxPlayers; i++ {
					if g.players[i] != nil && i != loserIdx && g.escaped[i] {
						g.players[i].RecordResult("thief", "win")
					}
				}
				msg = fmt.Sprintf("🃏 [%s]가 조커를 들고 남아 패배! 탈출한 플레이어 승리!", g.players[loserIdx].UserID)
			} else {
				for i := 0; i < thiefMaxPlayers; i++ {
					if g.players[i] != nil && len(g.hands[i]) > 0 {
						g.players[i].RecordResult("thief", "win")
						msg = fmt.Sprintf("🏆 [%s] 단독 생존 승리!", g.players[i].UserID)
						break
					}
				}
				if msg == "" {
					msg = "게임 종료."
				}
			}
			room.mu.RLock()
			totalCount := len(room.clients)
			room.mu.RUnlock()
			data, _ := json.Marshal(GameResultResponse{
				Type:           "game_result",
				Message:        msg,
				RoomID:         room.ID,
				Data:           map[string]any{"totalCount": totalCount},
				RematchEnabled: true,
			})
			room.broadcastAll(data)
			g.gameStarted = false
			g.stopTurnTimerLocked()
			return
		}

		g.advanceTurnLocked()
		g.startTurnTimerLocked()
		g.sendStateToAllLocked()
	})
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
	// 1. 다음 턴 유저 찾기 (시계방향으로 살아있는 다음 사람)
	for i := 1; i <= thiefMaxPlayers; i++ {
		idx := (g.currentTurn + i) % thiefMaxPlayers
		if g.players[idx] != nil && len(g.hands[idx]) > 0 {
			g.currentTurn = idx
			break
		}
	}
	// 2. 타겟(카드를 뺏길 사람) 찾기 (턴 유저의 시계방향 살아있는 다음 사람)
	g.targetIdx = -1
	for i := 1; i <= thiefMaxPlayers; i++ {
		tIdx := (g.currentTurn + i) % thiefMaxPlayers
		if g.players[tIdx] != nil && len(g.hands[tIdx]) > 0 {
			g.targetIdx = tIdx
			break
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

	// 페어 제거 전: 온전한 상태로 먼저 전송하고 안내 메시지
	g.sendStateToAllLocked()
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: "카드를 확인하고 페어를 정리하는 중입니다...",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	// 1초 대기(하이라이트 효과) 후 페어 제거 및 게임 진행
	room := g.room
	time.AfterFunc(1*time.Second, func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if !g.gameStarted {
			return
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
		// 타겟(시계방향 다음 플레이어) 설정
		g.targetIdx = -1
		for i := 1; i <= thiefMaxPlayers; i++ {
			tIdx := (g.currentTurn + i) % thiefMaxPlayers
			if g.players[tIdx] != nil && len(g.hands[tIdx]) > 0 {
				g.targetIdx = tIdx
				break
			}
		}

		// 게임 즉시 종료: activeCount<=1 (조커만 남은 1명 또는 뽑을 상대 없음)
		activeCount := 0
		loserIdx := -1
		for i := 0; i < thiefMaxPlayers; i++ {
			if g.players[i] != nil && !g.escaped[i] && len(g.hands[i]) > 0 {
				activeCount++
				if len(g.hands[i]) == 1 && g.hands[i][0].Value == "JOKER" {
					loserIdx = i
				}
			}
		}
		if activeCount <= 1 {
			var msg string
			if loserIdx >= 0 {
				g.players[loserIdx].RecordResult("thief", "lose")
				for i := 0; i < thiefMaxPlayers; i++ {
					if g.players[i] != nil && i != loserIdx && g.escaped[i] {
						g.players[i].RecordResult("thief", "win")
					}
				}
				msg = fmt.Sprintf("🃏 [%s]가 조커를 들고 남아 패배!", g.players[loserIdx].UserID)
			} else {
				for i := 0; i < thiefMaxPlayers; i++ {
					if g.players[i] != nil && len(g.hands[i]) > 0 {
						g.players[i].RecordResult("thief", "win")
						msg = fmt.Sprintf("🏆 [%s] 단독 생존 승리!", g.players[i].UserID)
						break
					}
				}
				if msg == "" {
					msg = "게임 종료."
				}
			}
			room.mu.RLock()
			totalCount := len(room.clients)
			room.mu.RUnlock()
			data, _ := json.Marshal(GameResultResponse{
				Type:           "game_result",
				Message:        msg,
				RoomID:         room.ID,
				Data:           map[string]any{"totalCount": totalCount},
				RematchEnabled: true,
			})
			room.broadcastAll(data)
			g.gameStarted = false
			return
		}

		notice2, _ := json.Marshal(ServerResponse{
			Type:    "game_notice",
			Message: "도둑잡기 시작! 페어를 제거하고, 다음 플레이어 패에서 카드를 뽑으세요. 패가 0장이면 탈출!",
			RoomID:  g.room.ID,
		})
		g.room.broadcastAll(notice2)
		g.startTurnTimerLocked()
		g.sendStateToAllLocked()
	})
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
	g.handleDraw(timedOutPlayer, "", -1)
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

func (g *ThiefGame) handleTakeover(client *Client) {
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
	for i := 0; i < thiefMaxPlayers; i++ {
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
		Message: fmt.Sprintf("🪑 [%s]님이 빈자리에 참여했습니다. (%d/%d)", client.UserID, g.playerCount, thiefMaxPlayers),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	total := g.playerCount
	ready := 0
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && g.startReady[g.players[i]] {
			ready++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
	log.Printf("[THIEF] room:[%s] [%s] 빈자리 참여 (슬롯 %d)", g.room.ID, client.UserID, slot)
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
		g.sendStateToSpectatorLocked(client)
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

	targetUserID := ""
	if g.targetIdx >= 0 && g.players[g.targetIdx] != nil {
		targetUserID = g.players[g.targetIdx].UserID
	}

	myHand := make([]Card, len(g.hands[idx]))
	copy(myHand, g.hands[idx])
	for i := range myHand {
		myHand[i].Hidden = false
	}

	data := ThiefData{
		Hand:         myHand,
		Turn:         turnUser,
		TargetUserID: targetUserID,
		Players:      players,
		Escaped:      escaped,
	}
	client.SendJSON(ThiefStateResponse{
		Type:   "thief_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *ThiefGame) sendStateToSpectatorLocked(client *Client) {
	players := make([]ThiefPlayerInfo, 0)
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] == nil {
			continue
		}
		players = append(players, ThiefPlayerInfo{
			UserID:    g.players[i].UserID,
			CardCount: len(g.hands[i]),
		})
	}
	canTakeover := false
	turnUser := ""
	targetUserID := ""
	if g.gameStarted {
		if g.players[g.currentTurn] != nil {
			turnUser = g.players[g.currentTurn].UserID
		}
		if g.targetIdx >= 0 && g.players[g.targetIdx] != nil {
			targetUserID = g.players[g.targetIdx].UserID
		}
	} else {
		for i := 0; i < thiefMaxPlayers; i++ {
			if g.players[i] == nil {
				canTakeover = true
				break
			}
		}
	}
	escaped := make([]string, 0)
	for i := 0; i < thiefMaxPlayers; i++ {
		if g.players[i] != nil && g.escaped[i] {
			escaped = append(escaped, g.players[i].UserID)
		}
	}
	data := ThiefData{
		Hand:         []Card{},
		Turn:         turnUser,
		TargetUserID: targetUserID,
		Players:      players,
		Escaped:      escaped,
		CanTakeover:  canTakeover,
	}
	client.SendJSON(ThiefStateResponse{
		Type:   "thief_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}
