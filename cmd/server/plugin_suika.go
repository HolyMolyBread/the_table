package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	suikaRechargeSec   = 3
	suikaMaxCharges    = 2
	suikaMaxPlayers    = 4
	suikaContainerW    = 400
	suikaContainerH    = 500
	suikaFruitTypes    = 11 // Cherry(0) ~ Watermelon(10)
)

// SuikaFruit는 컨테이너 내 과일 하나입니다.
type SuikaFruit struct {
	ID       int     `json:"id"`
	Type     int     `json:"type"`     // 0=Cherry ~ 10=Watermelon
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Radius   float64 `json:"radius"`
	OwnerSlot int    `json:"ownerSlot"` // 지분 100% 소유 슬롯
}

// PlayerCharge는 유저별 충전 상태입니다.
type PlayerCharge struct {
	ChargedCount int64 `json:"chargedCount"` // 0~2
	LastChargeAt int64 `json:"lastChargeAt"` // Unix ms, 마지막 충전 시각
}

// SuikaData는 suika_state 응답의 data 필드입니다.
type SuikaData struct {
	Players      [4]string       `json:"players"`
	Fruits       []SuikaFruit    `json:"fruits"`
	Charges      [4]PlayerCharge `json:"charges"` // 각 슬롯별 충전 상태
	GameStarted  bool            `json:"gameStarted"`
}

// SuikaStateResponse는 수박게임 상태 응답입니다.
type SuikaStateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   SuikaData `json:"data"`
}

// SuikaDropResultResponse는 drop 결과(성공/실패)를 클라이언트에 전달합니다.
type SuikaDropResultResponse struct {
	Type    string `json:"type"`
	RoomID  string `json:"roomId"`
	Success bool   `json:"success"`
}

// 과일 반지름 (타입별, 픽셀 근사)
var suikaRadii = [11]float64{12, 15, 18, 22, 26, 30, 35, 40, 46, 52, 60}

type SuikaGame struct {
	room        *Room
	players     [4]*Client
	fruits      []SuikaFruit
	charges     [4]PlayerCharge
	gameStarted bool
	startReady  map[*Client]bool
	nextFruitID int
	mu          sync.Mutex
}

func NewSuikaGame(room *Room) *SuikaGame {
	return &SuikaGame{
		room:       room,
		startReady: make(map[*Client]bool),
	}
}

func init() { RegisterPlugin("suika", func(room *Room) GamePlugin { return NewSuikaGame(room) }) }

func (g *SuikaGame) Name() string { return "수박게임 (Suika)" }

func (g *SuikaGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			g.players[i] = client
			g.sendStateToClientLocked(client)
			return
		}
	}

	slot := -1
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		notice, _ := json.Marshal(ServerResponse{
			Type: "game_notice", Message: fmt.Sprintf("[%s]님이 관전자로 입장했습니다.", client.UserID), RoomID: g.room.ID,
		})
		g.room.broadcastAll(notice)
		g.sendStateToClientLocked(client)
		return
	}

	g.players[slot] = client
	notice, _ := json.Marshal(ServerResponse{
		Type: "game_notice", Message: fmt.Sprintf("수박게임 [%s]님이 입장했습니다. (%d/4)", client.UserID, slot+1), RoomID: g.room.ID,
	})
	g.room.broadcastAll(notice)

	readyCount, total := 0, 0
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
}

func (g *SuikaGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] == client {
			g.players[i] = nil
			g.charges[i] = PlayerCharge{}
			delete(g.startReady, client)
			break
		}
	}
	if remainingCount == 0 {
		log.Printf("[suika] 방 [%s] 비어서 초기화", g.room.ID)
		g.resetLocked()
	}
}

func (g *SuikaGame) HandleAction(client *Client, action string, payload json.RawMessage) {
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
	case "drop":
		g.handleDropLocked(client, payload)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 cmd: %s", base.Cmd)})
	}
}

func (g *SuikaGame) handleReadyLocked(client *Client) {
	if g.gameStarted {
		return
	}
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] == client {
			g.startReady[client] = true
			break
		}
	}

	readyCount, total := 0, 0
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total})
	g.room.broadcastAll(upd)

	if readyCount >= 2 && total >= 2 {
		g.startGameLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

func (g *SuikaGame) startGameLocked() {
	g.startReady = make(map[*Client]bool)
	g.gameStarted = true
	g.fruits = nil
	g.nextFruitID = 0
	now := time.Now().UnixMilli()
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			g.charges[i] = PlayerCharge{ChargedCount: 1, LastChargeAt: now}
		}
	}
	g.sendStateToAllLocked()
}

func (g *SuikaGame) resetLocked() {
	g.gameStarted = false
	g.fruits = nil
	for i := 0; i < suikaMaxPlayers; i++ {
		g.charges[i] = PlayerCharge{}
	}
	g.startReady = make(map[*Client]bool)
}

// updateChargesLocked: 경과 시간에 따라 충전 수를 갱신합니다.
func (g *SuikaGame) updateChargesLocked(slot int) {
	if slot < 0 || slot >= suikaMaxPlayers || g.players[slot] == nil {
		return
	}
	c := &g.charges[slot]
	now := time.Now().UnixMilli()
	elapsed := now - c.LastChargeAt
	for c.ChargedCount < suikaMaxCharges && elapsed >= suikaRechargeSec*1000 {
		c.ChargedCount++
		elapsed -= suikaRechargeSec * 1000
		c.LastChargeAt += suikaRechargeSec * 1000
	}
}

func (g *SuikaGame) clientSlot(client *Client) int {
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] == client {
			return i
		}
	}
	return -1
}

func (g *SuikaGame) sendDropResult(client *Client, success bool) {
	msg, _ := json.Marshal(SuikaDropResultResponse{
		Type: "suika_drop_result", RoomID: g.room.ID, Success: success,
	})
	client.SafeSend(msg)
}

func (g *SuikaGame) handleDropLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted {
		return
	}
	slot := g.clientSlot(client)
	if slot < 0 {
		return
	}

	g.updateChargesLocked(slot)
	if g.charges[slot].ChargedCount < 1 {
		g.sendDropResult(client, false)
		return
	}

	var p struct {
		X float64 `json:"x"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		g.sendDropResult(client, false)
		return
	}

	// x 범위: 컨테이너 내부
	x := p.X
	if x < suikaRadii[0] {
		x = suikaRadii[0]
	}
	if x > suikaContainerW-suikaRadii[0] {
		x = suikaContainerW - suikaRadii[0]
	}

	// 충전 소모
	g.charges[slot].ChargedCount--
	g.charges[slot].LastChargeAt = time.Now().UnixMilli()

	// 과일 생성 (타입 0=Cherry, 지분 100% 소유)
	fruitType := 0
	radius := suikaRadii[fruitType]
	g.fruits = append(g.fruits, SuikaFruit{
		ID:        g.nextFruitID,
		Type:      fruitType,
		X:         x,
		Y:         0,
		Radius:    radius,
		OwnerSlot: slot,
	})
	g.nextFruitID++

	g.sendDropResult(client, true)
	g.sendStateToAllLocked()
}

func (g *SuikaGame) makeDataLocked() SuikaData {
	// 모든 플레이어 충전 상태 갱신
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			g.updateChargesLocked(i)
		}
	}

	fruitsCopy := make([]SuikaFruit, len(g.fruits))
	copy(fruitsCopy, g.fruits)

	return SuikaData{
		Players:     g.playersUserIDsLocked(),
		Fruits:      fruitsCopy,
		Charges:     g.charges,
		GameStarted: g.gameStarted,
	}
}

func (g *SuikaGame) playersUserIDsLocked() [4]string {
	var out [4]string
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			out[i] = g.players[i].UserID
		}
	}
	return out
}

func (g *SuikaGame) sendStateToAllLocked() {
	msg, _ := json.Marshal(SuikaStateResponse{Type: "suika_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	g.room.broadcastAll(msg)
}

func (g *SuikaGame) sendStateToClientLocked(client *Client) {
	msg, _ := json.Marshal(SuikaStateResponse{Type: "suika_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	client.SafeSend(msg)
}
