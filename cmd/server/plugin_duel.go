package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// DuelData는 duel_state 응답의 data 필드입니다.
type DuelData struct {
	Phase   string   `json:"phase"`   // "waiting" | "ready" | "draw" | "result"
	Players [2]string `json:"players"` // [0]=왼쪽, [1]=오른쪽
}

// DuelStateResponse는 결투 게임 상태 응답입니다.
type DuelStateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   DuelData  `json:"data"`
}

// DuelDrawMessage는 draw 신호 브로드캐스트입니다.
type DuelDrawMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomId"`
	DrawAt int64  `json:"drawAt"` // 서버 시각(ms, Unix)
}

// ── DuelGame 플러그인 ─────────────────────────────────────────────────────────

type DuelGame struct {
	room       *Room
	players    [2]*Client
	phase      string // "waiting" | "ready" | "draw" | "result"
	startReady [2]bool
	drawAt     int64  // draw 신호 시각 (ms)
	shootMs    map[string]int64
	stopCh     chan struct{}
	mu         sync.Mutex
}

func NewDuelGame(room *Room) *DuelGame {
	return &DuelGame{
		room:    room,
		phase:   "waiting",
		shootMs: make(map[string]int64),
	}
}

func init() { RegisterPlugin("duel", func(room *Room) GamePlugin { return NewDuelGame(room) }) }

func (g *DuelGame) Name() string { return "서부의 결투 (Western Duel)" }

func (g *DuelGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			g.players[i] = client
			g.sendStateToClientLocked(client)
			return
		}
	}

	slot := -1
	for i := 0; i < 2; i++ {
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
		g.sendStateToClientLocked(client)
		return
	}

	g.players[slot] = client
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("서부의 결투 [%s]님이 입장했습니다. (%d/2)", client.UserID, slot+1),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	readyCount := 0
	for i := 0; i < 2; i++ {
		if g.startReady[i] {
			readyCount++
		}
	}
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
}

func (g *DuelGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] == client {
			g.players[i] = nil
			g.startReady[i] = false
			break
		}
	}
	if remainingCount == 0 {
		log.Printf("[duel] 방 [%s] 비어서 초기화", g.room.ID)
		g.phase = "waiting"
		g.startReady = [2]bool{false, false}
		g.stopReadyTimerLocked()
	}
}

func (g *DuelGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var base struct {
		Cmd string `json:"cmd"`
	}
	if err := json.Unmarshal(payload, &base); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	switch base.Cmd {
	case "ready":
		g.handleReadyLocked(client)
	case "shoot":
		g.handleShootLocked(client, payload)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 cmd: %s", base.Cmd)})
	}
}

func (g *DuelGame) handleReadyLocked(client *Client) {
	if g.phase != "waiting" {
		return
	}
	idx := -1
	for i := 0; i < 2; i++ {
		if g.players[i] == client {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	g.startReady[idx] = true

	readyCount := 0
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[i] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total,
	})
	g.room.broadcastAll(upd)

	if readyCount >= 2 && total >= 2 {
		g.startReadyPhaseLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

func (g *DuelGame) startReadyPhaseLocked() {
	g.phase = "ready"
	g.shootMs = make(map[string]int64)
	g.sendStateToAllLocked()

	delayMs := 2000 + rand.Intn(3001)
	delay := time.Duration(delayMs) * time.Millisecond
	stopCh := make(chan struct{})
	g.stopCh = stopCh

	go func() {
		select {
		case <-stopCh:
			return
		case <-time.After(delay):
			g.broadcastDrawLocked()
		}
	}()
}

func (g *DuelGame) stopReadyTimerLocked() {
	if g.stopCh != nil {
		close(g.stopCh)
		g.stopCh = nil
	}
}

func (g *DuelGame) broadcastDrawLocked() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "ready" {
		return
	}
	g.stopReadyTimerLocked()
	g.phase = "draw"
	g.drawAt = time.Now().UnixMilli()

	msg, _ := json.Marshal(DuelDrawMessage{
		Type:   "duel_draw",
		RoomID: g.room.ID,
		DrawAt: g.drawAt,
	})
	g.room.broadcastAll(msg)
	g.sendStateToAllLocked()
}

func (g *DuelGame) handleShootLocked(client *Client, payload json.RawMessage) {
	var p struct {
		Ms int64 `json:"ms"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "shoot 페이로드 파싱 오류"})
		return
	}

	idx := -1
	for i := 0; i < 2; i++ {
		if g.players[i] == client {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	if g.phase == "ready" {
		g.endDuelFoulLocked(idx, client.UserID)
		return
	}

	if g.phase != "draw" {
		return
	}

	if _, ok := g.shootMs[client.UserID]; ok {
		return
	}

	g.shootMs[client.UserID] = p.Ms

	if len(g.shootMs) >= 2 {
		g.resolveDuelLocked()
	}
}

func (g *DuelGame) endDuelFoulLocked(foulIdx int, foulUserID string) {
	g.phase = "result"
	g.stopReadyTimerLocked()
	winnerIdx := 1 - foulIdx
	if g.players[winnerIdx] != nil {
		g.players[winnerIdx].RecordResult("duel", "win")
	}
	if g.players[foulIdx] != nil {
		g.players[foulIdx].RecordResult("duel", "lose")
	}
	g.room.mu.RLock()
	totalCount := len(g.room.clients)
	g.room.mu.RUnlock()
	msg, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        fmt.Sprintf("[%s]님이 너무 빨리 쏘아 반칙 패배!", foulUserID),
		RoomID:         g.room.ID,
		Data:           map[string]any{"totalCount": totalCount, "foul": true},
		RematchEnabled: true,
	})
	g.room.broadcastAll(msg)
	g.phase = "waiting"
	g.startReady = [2]bool{false, false}
}

func (g *DuelGame) resolveDuelLocked() {
	g.phase = "result"
	var winnerIdx int = -1
	var minMs int64 = 1 << 62
	for i := 0; i < 2; i++ {
		if g.players[i] == nil {
			continue
		}
		ms, ok := g.shootMs[g.players[i].UserID]
		if ok && ms < minMs {
			minMs = ms
			winnerIdx = i
		}
	}

	loserIdx := 1 - winnerIdx
	if winnerIdx >= 0 && g.players[winnerIdx] != nil {
		g.players[winnerIdx].RecordResult("duel", "win")
	}
	if loserIdx >= 0 && g.players[loserIdx] != nil {
		g.players[loserIdx].RecordResult("duel", "lose")
	}

	winnerName := ""
	if winnerIdx >= 0 && g.players[winnerIdx] != nil {
		winnerName = g.players[winnerIdx].UserID
	}
	g.room.mu.RLock()
	totalCount := len(g.room.clients)
	g.room.mu.RUnlock()
	resultMsg := fmt.Sprintf("[%s] 승리! 반응 속도 %dms", winnerName, minMs)
	msg, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        resultMsg,
		RoomID:         g.room.ID,
		Data:           map[string]any{"totalCount": totalCount, "winnerMs": minMs},
		RematchEnabled: true,
	})
	g.room.broadcastAll(msg)
	g.phase = "waiting"
	g.startReady = [2]bool{false, false}
}

func (g *DuelGame) sendStateToAllLocked() {
	msg, _ := json.Marshal(DuelStateResponse{
		Type:   "duel_state",
		RoomID: g.room.ID,
		Data:   g.makeDataLocked(),
	})
	g.room.broadcastAll(msg)
}

func (g *DuelGame) sendStateToClientLocked(client *Client) {
	msg, _ := json.Marshal(DuelStateResponse{
		Type:   "duel_state",
		RoomID: g.room.ID,
		Data:   g.makeDataLocked(),
	})
	client.SafeSend(msg)
}

func (g *DuelGame) makeDataLocked() DuelData {
	var players [2]string
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			players[i] = g.players[i].UserID
		}
	}
	return DuelData{
		Phase:   g.phase,
		Players: players,
	}
}
