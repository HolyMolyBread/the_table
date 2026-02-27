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
	suikaRechargeSec  = 3
	suikaMaxCharges   = 2
	suikaMaxPlayers   = 4
	suikaContainerW   = 400
	suikaContainerH   = 500
	suikaFruitLevels  = 11 // 1~11 (Cherry ~ Watermelon)
)

// SuikaFruitDef는 레벨별 과일 정의입니다.
type SuikaFruitDef struct {
	Radius float64 `json:"radius"`
	Score  int     `json:"score"`
}

// 점수: 2^n (체리 2, 딸기 4, ..., 수박 2048)
var suikaFruitDefs = [11]SuikaFruitDef{
	{12, 2},    // 1: Cherry
	{15, 4},    // 2: Strawberry
	{18, 8},    // 3: Grape
	{22, 16},   // 4: Hallabong
	{26, 32},   // 5: Persimmon
	{30, 64},   // 6: Apple
	{35, 128},  // 7: Pear
	{40, 256},  // 8: Peach
	{46, 512},  // 9: Pineapple
	{52, 1024}, // 10: Melon
	{60, 2048}, // 11: Watermelon
}

// SuikaFruit는 컨테이너 내 과일 하나입니다.
type SuikaFruit struct {
	ID          int                `json:"id"`
	Type        int                `json:"type"`        // 0~10 (레벨 1~11)
	X           float64            `json:"x"`
	Y           float64            `json:"y"`
	Radius      float64            `json:"radius"`
	OwnerEquity map[string]float64 `json:"ownerEquity"` // userId -> 지분율 (0~1)
}

// PlayerCharge는 유저별 충전 상태입니다.
type PlayerCharge struct {
	ChargedCount  int64 `json:"chargedCount"`
	LastChargeAt  int64 `json:"lastChargeAt"`
}

// SuikaData는 suika_state 응답의 data 필드입니다.
type SuikaData struct {
	Players     [4]string       `json:"players"`
	Fruits      []SuikaFruit    `json:"fruits"`
	Charges     [4]PlayerCharge `json:"charges"`
	Scores      [4]float64      `json:"scores"` // 지분 비율에 따른 소수점 정밀 배분
	GameStarted bool            `json:"gameStarted"`
	GameOver    bool            `json:"gameOver"`
	HostUserID  string          `json:"hostUserId"` // 첫 번째 플레이어 = Host
}

// SuikaStateResponse는 수박게임 상태 응답입니다.
type SuikaStateResponse struct {
	Type   string    `json:"type"`
	RoomID string    `json:"roomId"`
	Data   SuikaData `json:"data"`
}

// SuikaDropResultResponse는 drop 결과를 클라이언트에 전달합니다.
type SuikaDropResultResponse struct {
	Type    string `json:"type"`
	RoomID  string `json:"roomId"`
	Success bool   `json:"success"`
}

const (
	suikaForbiddenLineY = 50
	suikaGameOverPenalty = 100
)

type SuikaGame struct {
	room           *Room
	players        [4]*Client
	fruits         []SuikaFruit
	charges        [4]PlayerCharge
	scores         [4]float64
	gameStarted    bool
	gameOver       bool
	startReady     map[*Client]bool
	nextFruitID    int
	lastDropUserID string // 게임오버 시 패널티 대상
	mu             sync.Mutex
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
	case "merge":
		g.handleMergeLocked(client, payload)
	case "sync_all":
		g.handleSyncAllLocked(client, payload)
	case "game_over":
		g.handleGameOverLocked(client, payload)
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
	g.gameOver = false
	g.fruits = nil
	g.scores = [4]float64{0, 0, 0, 0}
	g.nextFruitID = 0
	g.lastDropUserID = ""
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
	g.gameOver = false
	g.fruits = nil
	g.scores = [4]float64{0, 0, 0, 0}
	g.lastDropUserID = ""
	for i := 0; i < suikaMaxPlayers; i++ {
		g.charges[i] = PlayerCharge{}
	}
	g.startReady = make(map[*Client]bool)
}

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

func (g *SuikaGame) userIdToSlot(userId string) int {
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil && g.players[i].UserID == userId {
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
	if !g.gameStarted || g.gameOver {
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
		X    float64 `json:"x"`
		Type *int    `json:"type"` // 클라이언트 Ghost와 동기화 (0~3)
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		g.sendDropResult(client, false)
		return
	}

	x := p.X
	def := suikaFruitDefs[0]
	if x < def.Radius {
		x = def.Radius
	}
	if x > suikaContainerW-def.Radius {
		x = suikaContainerW - def.Radius
	}

	userId := g.players[slot].UserID
	g.charges[slot].ChargedCount--
	g.charges[slot].LastChargeAt = time.Now().UnixMilli()
	g.lastDropUserID = userId

	// 클라이언트 Ghost와 동기화: type이 유효(0~3)하면 사용, 아니면 랜덤
	fruitType := rand.Intn(4)
	if p.Type != nil && *p.Type >= 0 && *p.Type <= 3 {
		fruitType = *p.Type
	}
	def = suikaFruitDefs[fruitType]
	ownerEquity := map[string]float64{userId: 1.0}

	g.fruits = append(g.fruits, SuikaFruit{
		ID:          g.nextFruitID,
		Type:        fruitType,
		X:           x,
		Y:           60, // 금지선(50px) 바로 아래에서 화면에 보이도록
		Radius:      def.Radius,
		OwnerEquity: ownerEquity,
	})
	g.nextFruitID++

	g.sendDropResult(client, true)
	g.sendStateToAllLocked()
}

func (g *SuikaGame) findFruitByID(id int) *SuikaFruit {
	for i := range g.fruits {
		if g.fruits[i].ID == id {
			return &g.fruits[i]
		}
	}
	return nil
}

func (g *SuikaGame) handleMergeLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted || g.gameOver {
		return
	}

	var p struct {
		AID int     `json:"aid"`
		BID int     `json:"bid"`
		CX  float64 `json:"cx"`
		CY  float64 `json:"cy"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}

	fa := g.findFruitByID(p.AID)
	fb := g.findFruitByID(p.BID)
	if fa == nil || fb == nil || fa.Type != fb.Type {
		return
	}
	if fa.Type >= suikaFruitLevels-1 {
		return
	}

	// NewEquity[user] = (A.Equity[user] + B.Equity[user]) / 2
	merged := make(map[string]float64)
	allUsers := make(map[string]bool)
	for u := range fa.OwnerEquity {
		allUsers[u] = true
	}
	for u := range fb.OwnerEquity {
		allUsers[u] = true
	}
	for u := range allUsers {
		a := fa.OwnerEquity[u]
		b := fb.OwnerEquity[u]
		merged[u] = (a + b) / 2
	}

	newType := fa.Type + 1
	def := suikaFruitDefs[newType]
	scoreToAdd := float64(def.Score)

	// 지분 비율에 따라 소수점까지 정밀 배분
	for userId, eq := range merged {
		if eq <= 0 {
			continue
		}
		s := g.userIdToSlot(userId)
		if s >= 0 {
			g.scores[s] += scoreToAdd * eq
		}
	}

	// 새 과일 생성 (클라이언트 전달 좌표 우선, 없으면 두 과일 중간)
	cx, cy := p.CX, p.CY
	if cx == 0 && cy == 0 {
		cx = (fa.X + fb.X) / 2
		cy = (fa.Y + fb.Y) / 2
	}

	// 기존 과일 제거
	newFruits := make([]SuikaFruit, 0, len(g.fruits)-2)
	for _, f := range g.fruits {
		if f.ID != p.AID && f.ID != p.BID {
			newFruits = append(newFruits, f)
		}
	}
	g.fruits = newFruits

	g.fruits = append(g.fruits, SuikaFruit{
		ID:          g.nextFruitID,
		Type:        newType,
		X:           cx,
		Y:           cy,
		Radius:      def.Radius,
		OwnerEquity: merged,
	})
	g.nextFruitID++

	g.sendStateToAllLocked()
}

func (g *SuikaGame) makeDataLocked() SuikaData {
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			g.updateChargesLocked(i)
		}
	}

	fruitsCopy := make([]SuikaFruit, len(g.fruits))
	for i := range g.fruits {
		fruitsCopy[i] = g.fruits[i]
		if g.fruits[i].OwnerEquity != nil {
			eq := make(map[string]float64)
			for k, v := range g.fruits[i].OwnerEquity {
				eq[k] = v
			}
			fruitsCopy[i].OwnerEquity = eq
		}
	}

	return SuikaData{
		Players:     g.playersUserIDsLocked(),
		Fruits:      fruitsCopy,
		Charges:     g.charges,
		Scores:      g.scores,
		GameStarted: g.gameStarted,
		GameOver:    g.gameOver,
		HostUserID:  g.hostUserIDLocked(),
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

func (g *SuikaGame) hostUserIDLocked() string {
	for i := 0; i < suikaMaxPlayers; i++ {
		if g.players[i] != nil {
			return g.players[i].UserID
		}
	}
	return ""
}

func (g *SuikaGame) isHostLocked(client *Client) bool {
	return g.hostUserIDLocked() == client.UserID
}

// SuikaSyncBody는 sync_all 페이로드의 바디 하나입니다.
type SuikaSyncBody struct {
	ID int     `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	VX float64 `json:"vx"`
	VY float64 `json:"vy"`
}

// SuikaSyncAllResponse는 sync_all 결과를 모든 클라이언트에 브로드캐스트합니다.
type SuikaSyncAllResponse struct {
	Type   string          `json:"type"`
	RoomID string          `json:"roomId"`
	Bodies []SuikaSyncBody `json:"bodies"`
}

func (g *SuikaGame) handleSyncAllLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted || g.gameOver {
		return
	}
	if !g.isHostLocked(client) {
		return
	}
	var p struct {
		Bodies []SuikaSyncBody `json:"bodies"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return
	}
	msg, _ := json.Marshal(SuikaSyncAllResponse{
		Type: "suika_sync_all", RoomID: g.room.ID, Bodies: p.Bodies,
	})
	g.room.broadcastAll(msg)
}

func (g *SuikaGame) handleGameOverLocked(client *Client, payload json.RawMessage) {
	if !g.gameStarted || g.gameOver {
		return
	}
	if !g.isHostLocked(client) {
		return
	}
	g.gameOver = true
	// 마지막 투하 유저에게 패널티
	if g.lastDropUserID != "" {
		s := g.userIdToSlot(g.lastDropUserID)
		if s >= 0 {
			g.scores[s] -= suikaGameOverPenalty
			if g.scores[s] < 0 {
				g.scores[s] = 0
			}
		}
	}
	g.sendStateToAllLocked()
}

func (g *SuikaGame) sendStateToAllLocked() {
	msg, _ := json.Marshal(SuikaStateResponse{Type: "suika_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	g.room.broadcastAll(msg)
}

func (g *SuikaGame) sendStateToClientLocked(client *Client) {
	msg, _ := json.Marshal(SuikaStateResponse{Type: "suika_state", RoomID: g.room.ID, Data: g.makeDataLocked()})
	client.SafeSend(msg)
}
