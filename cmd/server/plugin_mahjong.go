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
	mahjongMaxPlayers    = 4
	mahjongTilesPerHand  = 13
	mahjongTurnTimeLimit = 20
)

// MahjongTile은 마작 패 한 장을 표현합니다.
// Type: "man"(만수), "pin"(삭수), "sou"(통수), "honor"(자패)
// Value: 1~9 (수패), 1~7 (자패: 1동 2남 3서 4북 5백 6발 7중)
type MahjongTile struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
}

// MahjongMeld는 공개된 패(후로)를 의미합니다.
type MahjongMeld struct {
	Type  string        `json:"type"`  // "chi", "pon"
	Tiles []MahjongTile `json:"tiles"` // 3장 (치/퐁)
}

// MahjongPlayerInfo는 한 플레이어의 공개 정보입니다.
type MahjongPlayerInfo struct {
	UserID    string         `json:"userId"`
	HandCount int            `json:"handCount"` // 손패 수 (타인은 숫자만)
	Discards  []MahjongTile  `json:"discards"`  // 버림패
	Melds     []MahjongMeld  `json:"melds"`    // 공개된 패(후로)
	IsTurn    bool           `json:"isTurn"`
}

// MahjongData는 mahjong_state 응답의 data 필드입니다.
type MahjongData struct {
	WallCount       int                 `json:"wallCount"`   // 남은 패 수
	Players         []MahjongPlayerInfo `json:"players"`
	CurrentTurn     string              `json:"currentTurn"`
	Message         string              `json:"message,omitempty"`
	CanTakeover     bool                `json:"canTakeover,omitempty"`
	MyHand          []MahjongTile       `json:"myHand,omitempty"` // 본인 손패 (14장일 때 타패 가능)
	CallWindow      bool                `json:"callWindow"`      // 콜 대기 상태 여부
	LastDiscard     *MahjongTile        `json:"lastDiscard,omitempty"`
	LastDiscarderID string              `json:"lastDiscarderId,omitempty"`
}

// MahjongStateResponse는 마작 게임 상태 응답입니다.
type MahjongStateResponse struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId"`
	Data   MahjongData `json:"data"`
}

// MahjongGame은 4인 마작 플러그인입니다.
// Phase 1: 패 분배 + 쯔모/타패 + 콜 대기(치/퐁) 뼈대
type MahjongGame struct {
	room             *Room
	players          [mahjongMaxPlayers]*Client
	wall             []MahjongTile
	hands            [mahjongMaxPlayers][]MahjongTile
	discards         [mahjongMaxPlayers][]MahjongTile
	melds            [mahjongMaxPlayers][]MahjongMeld
	currentPlayerIdx int
	playerCount      int
	gameStarted      bool
	startReady       map[*Client]bool
	stopTick         chan struct{}
	mu               sync.Mutex

	// 콜 대기(Call Window) 상태
	state            string       // "playing" | "call_window"
	lastDiscard      MahjongTile
	lastDiscarderIdx int
	callPassed       map[*Client]bool // 패스한 플레이어
	callTimerStop    chan struct{}
}

// NewMahjongGame creates a new Mahjong game plugin.
func NewMahjongGame(room *Room) *MahjongGame {
	return &MahjongGame{room: room, startReady: make(map[*Client]bool)}
}

func init() { RegisterPlugin("mahjong", func(room *Room) GamePlugin { return NewMahjongGame(room) }) }

func (g *MahjongGame) Name() string { return "mahjong" }

func (g *MahjongGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] == client {
			g.sendStateToAllLocked()
			return
		}
	}

	slot := -1
	for i := 0; i < mahjongMaxPlayers; i++ {
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
		Message: fmt.Sprintf("🀄 [%s]님이 입장했습니다. (%d/%d)", client.UserID, g.playerCount, mahjongMaxPlayers),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	if !g.gameStarted {
		readyCount := 0
		for i := 0; i < mahjongMaxPlayers; i++ {
			if g.players[i] != nil && g.startReady[g.players[i]] {
				readyCount++
			}
		}
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: mahjongMaxPlayers,
		})
		g.room.broadcastAll(upd)
	}
	g.sendStateToAllLocked()
}

func (g *MahjongGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := -1
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] == client {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	delete(g.startReady, client)
	g.players[idx] = nil
	g.playerCount--

	if !g.gameStarted {
		readyCount := 0
		for i := 0; i < mahjongMaxPlayers; i++ {
			if g.players[i] != nil && g.startReady[g.players[i]] {
				readyCount++
			}
		}
		upd, _ := json.Marshal(ReadyUpdateMessage{
			Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: mahjongMaxPlayers,
		})
		g.room.broadcastAll(upd)
		g.sendStateToAllLocked()
		return
	}

	// 게임 중 퇴장: 매치 종료
	g.stopTurnTimerLocked()
	g.room.mu.RLock()
	totalCount := len(g.room.clients)
	g.room.mu.RUnlock()
	data, _ := json.Marshal(GameResultResponse{
		Type:           "game_result",
		Message:        fmt.Sprintf("[%s]님이 퇴장했습니다. 매치 종료.", client.UserID),
		RoomID:         g.room.ID,
		Data:           map[string]any{"totalCount": totalCount},
		RematchEnabled: false,
	})
	g.room.broadcastAll(data)
	g.gameStarted = false
	g.wall = nil
	for i := 0; i < mahjongMaxPlayers; i++ {
		g.hands[i] = nil
		g.discards[i] = nil
	}
}

// HandleAction은 게임 액션을 처리합니다. handleReady/handleDiscard/handleCall 각각이 g.mu.Lock()으로
// 보호되어 spam-click 및 동시 요청을 직렬화합니다.
func (g *MahjongGame) HandleAction(client *Client, action string, payload json.RawMessage) {
	var p struct {
		Cmd        string          `json:"cmd"`
		Index      int             `json:"index"`
		CallType   string          `json:"callType"`
		TargetTiles []MahjongTile  `json:"targetTiles"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "game_action 페이로드 파싱 오류"})
		return
	}

	switch p.Cmd {
	case "ready":
		g.handleReady(client)
	case "discard":
		g.handleDiscard(client, p.Index)
	case "call":
		g.handleCall(client, p.CallType, p.TargetTiles)
	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 마작 명령: [%s]", p.Cmd),
		})
	}
}

func (g *MahjongGame) handleReady(client *Client) {
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
	total := 0
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] != nil {
			total++
		}
	}
	if total < mahjongMaxPlayers {
		client.SendJSON(ServerResponse{Type: "error", Message: "4인 대전 전용 게임입니다. 인원을 기다려주세요."})
		return
	}
	g.startReady[client] = true
	ready := 0
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] != nil && g.startReady[g.players[i]] {
			ready++
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: ready, TotalCount: mahjongMaxPlayers,
	})
	g.room.broadcastAll(upd)
	if ready == mahjongMaxPlayers && total == mahjongMaxPlayers {
		g.startReady = make(map[*Client]bool)
		g.gameStarted = true
		g.startRoundLocked()
	}
}

func (g *MahjongGame) handleDiscard(client *Client, index int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted {
		return
	}
	// 턴 검증 최우선: 찰나의 순간에 턴이 넘어갔다면 상태 변경 없이 즉시 return
	if g.players[g.currentPlayerIdx] == nil || g.players[g.currentPlayerIdx].UserID != client.UserID {
		return
	}

	idx := g.playerIndex(client)
	if idx < 0 {
		return
	}
	hand := g.hands[idx]
	if len(hand) != 14 {
		client.SendJSON(ServerResponse{Type: "error", Message: "타패하려면 14장이어야 합니다."})
		return
	}
	if index < 0 || index >= len(hand) {
		client.SendJSON(ServerResponse{Type: "error", Message: "유효하지 않은 패 인덱스입니다."})
		return
	}

	// 타패: index 위치의 패를 제거하여 discards에 추가
	discarded := hand[index]
	g.hands[idx] = append(hand[:index], hand[index+1:]...)
	sortMahjongHand(g.hands[idx])
	g.discards[idx] = append(g.discards[idx], discarded)

	tileStr := g.tileDisplayName(discarded)
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] %s 버림", client.UserID, tileStr),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	// 콜 대기 상태로 전이 (바로 advanceTurnLocked 호출하지 않음)
	g.state = "call_window"
	g.lastDiscard = discarded
	g.lastDiscarderIdx = idx
	g.callPassed = make(map[*Client]bool)
	g.stopTurnTimerLocked()
	g.startCallTimerLocked()
	g.sendStateToAllLocked()
}

func (g *MahjongGame) startCallTimerLocked() {
	if g.callTimerStop != nil {
		close(g.callTimerStop)
		g.callTimerStop = nil
	}
	stopCh := make(chan struct{})
	g.callTimerStop = stopCh
	room := g.room
	go func() {
		select {
		case <-stopCh:
			return
		case <-time.After(5 * time.Second):
			g.mu.Lock()
			if g.state == "call_window" {
				g.endCallWindowLocked()
			}
			g.mu.Unlock()
		}
		_ = room
	}()
}

func (g *MahjongGame) endCallWindowLocked() {
	if g.callTimerStop != nil {
		close(g.callTimerStop)
		g.callTimerStop = nil
	}
	g.state = "playing"
	g.callPassed = nil
	g.advanceTurnLocked()
}

func (g *MahjongGame) handleCall(client *Client, callType string, targetTiles []MahjongTile) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.gameStarted || g.state != "call_window" {
		client.SendJSON(ServerResponse{Type: "error", Message: "콜 대기 상태가 아닙니다."})
		return
	}
	callerIdx := g.playerIndex(client)
	if callerIdx < 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}
	if callerIdx == g.lastDiscarderIdx {
		client.SendJSON(ServerResponse{Type: "error", Message: "버린 사람은 콜할 수 없습니다."})
		return
	}

	switch callType {
	case "pass":
		g.callPassed[client] = true
		passedCount := len(g.callPassed)
		canCallCount := 0
		for i := 0; i < mahjongMaxPlayers; i++ {
			if i != g.lastDiscarderIdx && g.players[i] != nil {
				canCallCount++
			}
		}
		if passedCount >= canCallCount {
			g.endCallWindowLocked()
			return
		}
		g.sendStateToAllLocked()
	case "pon":
		g.executePonLocked(client, callerIdx)
	case "chi":
		g.executeChiLocked(client, callerIdx, targetTiles)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 콜 타입: %s", callType)})
	}
}

func (g *MahjongGame) executePonLocked(client *Client, callerIdx int) {
	// 손패에서 lastDiscard와 같은 패 2장 찾기
	hand := g.hands[callerIdx]
	var keep []MahjongTile
	var matched []MahjongTile
	for _, t := range hand {
		if t.Type == g.lastDiscard.Type && t.Value == g.lastDiscard.Value {
			if len(matched) < 2 {
				matched = append(matched, t)
			} else {
				keep = append(keep, t)
			}
		} else {
			keep = append(keep, t)
		}
	}
	if len(matched) < 2 {
		client.SendJSON(ServerResponse{Type: "error", Message: "퐁에 필요한 패가 없습니다."})
		return
	}
	// meld 생성: 2장(손패) + 1장(버림패)
	meld := MahjongMeld{
		Type:  "pon",
		Tiles: []MahjongTile{matched[0], matched[1], g.lastDiscard},
	}
	g.melds[callerIdx] = append(g.melds[callerIdx], meld)
	g.hands[callerIdx] = keep
	// 버린 사람의 마지막 버림패 제거
	discarderDiscards := g.discards[g.lastDiscarderIdx]
	if len(discarderDiscards) > 0 {
		g.discards[g.lastDiscarderIdx] = discarderDiscards[:len(discarderDiscards)-1]
	}
	g.currentPlayerIdx = callerIdx
	g.state = "playing"
	if g.callTimerStop != nil {
		close(g.callTimerStop)
		g.callTimerStop = nil
	}
	g.callPassed = nil
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] 퐁! %s", client.UserID, g.tileDisplayName(g.lastDiscard)),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *MahjongGame) executeChiLocked(client *Client, callerIdx int, targetTiles []MahjongTile) {
	t := g.lastDiscard
	if t.Type == "honor" {
		client.SendJSON(ServerResponse{Type: "error", Message: "자패는 치할 수 없습니다."})
		return
	}
	hand := make([]MahjongTile, len(g.hands[callerIdx]))
	copy(hand, g.hands[callerIdx])

	var allThree []MahjongTile
	if len(targetTiles) == 2 {
		// 클라이언트가 targetTiles를 보낸 경우
		for _, need := range targetTiles {
			found := false
			for i, h := range hand {
				if h.Type == need.Type && h.Value == need.Value {
					hand = append(hand[:i], hand[i+1:]...)
					found = true
					break
				}
			}
			if !found {
				client.SendJSON(ServerResponse{Type: "error", Message: "치에 필요한 패가 손패에 없습니다."})
				return
			}
		}
		allThree = []MahjongTile{targetTiles[0], targetTiles[1], g.lastDiscard}
	} else {
		// targetTiles 없으면 손패에서 가능한 치 조합 자동 탐색
		v := t.Value
		possiblePairs := [][]int{{v - 2, v - 1}, {v - 1, v + 1}, {v + 1, v + 2}}
		for _, pair := range possiblePairs {
			if pair[0] < 1 || pair[1] > 9 {
				continue
			}
			var keep []MahjongTile
			var used []MahjongTile
			for _, h := range hand {
				if h.Type != t.Type {
					keep = append(keep, h)
					continue
				}
				if h.Value == pair[0] && (len(used) == 0 || used[0].Value != pair[0]) {
					used = append(used, h)
				} else if h.Value == pair[1] && (len(used) == 0 || used[0].Value != pair[1]) {
					used = append(used, h)
				} else if len(used) == 1 && (h.Value == pair[0] || h.Value == pair[1]) && h.Value != used[0].Value {
					used = append(used, h)
				} else {
					keep = append(keep, h)
				}
			}
			if len(used) == 2 {
				hand = keep
				if used[0].Value > used[1].Value {
					used[0], used[1] = used[1], used[0]
				}
				allThree = []MahjongTile{used[0], g.lastDiscard, used[1]}
				for i := 0; i < len(allThree)-1; i++ {
					for j := i + 1; j < len(allThree); j++ {
						if allThree[i].Value > allThree[j].Value {
							allThree[i], allThree[j] = allThree[j], allThree[i]
						}
					}
				}
				break
			}
		}
	}
	if len(allThree) != 3 {
		client.SendJSON(ServerResponse{Type: "error", Message: "치에 필요한 연속 패가 없습니다."})
		return
	}
	// 정렬 검증
	for i := 0; i < len(allThree)-1; i++ {
		for j := i + 1; j < len(allThree); j++ {
			if allThree[i].Value > allThree[j].Value {
				allThree[i], allThree[j] = allThree[j], allThree[i]
			}
		}
	}
	if allThree[1].Value-allThree[0].Value != 1 || allThree[2].Value-allThree[1].Value != 1 {
		client.SendJSON(ServerResponse{Type: "error", Message: "연속된 수패가 아닙니다."})
		return
	}
	meld := MahjongMeld{Type: "chi", Tiles: allThree}
	g.melds[callerIdx] = append(g.melds[callerIdx], meld)
	g.hands[callerIdx] = hand
	discarderDiscards := g.discards[g.lastDiscarderIdx]
	if len(discarderDiscards) > 0 {
		g.discards[g.lastDiscarderIdx] = discarderDiscards[:len(discarderDiscards)-1]
	}
	g.currentPlayerIdx = callerIdx
	g.state = "playing"
	if g.callTimerStop != nil {
		close(g.callTimerStop)
		g.callTimerStop = nil
	}
	g.callPassed = nil
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("[%s] 치! %s", client.UserID, g.tileDisplayName(g.lastDiscard)),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *MahjongGame) tileDisplayName(t MahjongTile) string {
	switch t.Type {
	case "man":
		return fmt.Sprintf("%d만", t.Value)
	case "pin":
		return fmt.Sprintf("%d삭", t.Value)
	case "sou":
		return fmt.Sprintf("%d통", t.Value)
	case "honor":
		names := map[int]string{1: "동", 2: "남", 3: "서", 4: "북", 5: "백", 6: "발", 7: "중"}
		if n, ok := names[t.Value]; ok {
			return n
		}
	}
	return "?"
}

func (g *MahjongGame) playerIndex(c *Client) int {
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] == c {
			return i
		}
	}
	return -1
}

func (g *MahjongGame) advanceTurnLocked() {
	// 다음 플레이어로 턴 이동 (nil 슬롯 건너뛰기)
	for attempt := 0; attempt < mahjongMaxPlayers; attempt++ {
		nextIdx := (g.currentPlayerIdx + 1) % mahjongMaxPlayers
		g.currentPlayerIdx = nextIdx
		if g.players[nextIdx] != nil {
			break
		}
	}

	// wall에서 1장 쯔모
	if len(g.wall) == 0 {
		g.sendStateToAllLocked()
		return
	}
	drawn := g.wall[0]
	g.wall = g.wall[1:]
	g.hands[g.currentPlayerIdx] = append(g.hands[g.currentPlayerIdx], drawn)
	sortMahjongHand(g.hands[g.currentPlayerIdx])

	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
}

func (g *MahjongGame) startTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	currentPlayer := g.players[g.currentPlayerIdx]
	if currentPlayer == nil {
		return
	}
	room := g.room
	data, _ := json.Marshal(TimerTickMessage{
		Type:      "timer_tick",
		RoomID:    g.room.ID,
		TurnUser:  currentPlayer.UserID,
		Remaining: mahjongTurnTimeLimit,
	})
	g.room.broadcastAll(data)
	go func() {
		remaining := mahjongTurnTimeLimit
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

func (g *MahjongGame) stopTurnTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *MahjongGame) handleTimeOver(timedOutPlayer *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.gameStarted || g.players[g.currentPlayerIdx] != timedOutPlayer {
		return
	}
	idx := g.playerIndex(timedOutPlayer)
	if idx < 0 || len(g.hands[idx]) != 14 {
		return
	}
	g.stopTurnTimerLocked()
	// 타임아웃: 무작위로 1장 버림
	discardIdx := rand.Intn(14)
	hand := g.hands[idx]
	discarded := hand[discardIdx]
	g.hands[idx] = append(hand[:discardIdx], hand[discardIdx+1:]...)
	sortMahjongHand(g.hands[idx])
	g.discards[idx] = append(g.discards[idx], discarded)

	tileStr := g.tileDisplayName(discarded)
	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: fmt.Sprintf("⏰ [%s] 시간 초과! %s 자동 버림", timedOutPlayer.UserID, tileStr),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.state = "call_window"
	g.lastDiscard = discarded
	g.lastDiscarderIdx = idx
	g.callPassed = make(map[*Client]bool)
	g.startCallTimerLocked()
	g.sendStateToAllLocked()
}

func (g *MahjongGame) buildWall() []MahjongTile {
	wall := make([]MahjongTile, 0, 136)
	// 수패: man, pin, sou 각 9종 x 4장 = 108
	for _, t := range []string{"man", "pin", "sou"} {
		for v := 1; v <= 9; v++ {
			for n := 0; n < 4; n++ {
				wall = append(wall, MahjongTile{Type: t, Value: v})
			}
		}
	}
	// 자패: honor 7종 x 4장 = 28
	for v := 1; v <= 7; v++ {
		for n := 0; n < 4; n++ {
			wall = append(wall, MahjongTile{Type: "honor", Value: v})
		}
	}
	rand.Shuffle(len(wall), func(i, j int) { wall[i], wall[j] = wall[j], wall[i] })
	return wall
}

func (g *MahjongGame) startRoundLocked() {
	activeCount := 0
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] != nil {
			activeCount++
		}
	}
	if activeCount < mahjongMaxPlayers {
		g.sendStateToAllLocked()
		return
	}

	g.wall = g.buildWall()
	g.state = "playing"
	for i := 0; i < mahjongMaxPlayers; i++ {
		g.hands[i] = nil
		g.discards[i] = nil
		g.melds[i] = nil
	}

	// 4명에게 각각 13장 분배 (13*4=52장 사용)
	cardIdx := 0
	for i := 0; i < mahjongMaxPlayers; i++ {
		for j := 0; j < mahjongTilesPerHand; j++ {
			g.hands[i] = append(g.hands[i], g.wall[cardIdx])
			cardIdx++
		}
		sortMahjongHand(g.hands[i])
	}
	g.wall = g.wall[cardIdx:]

	// 선(친)부터 턴 시작 → 쯔모 1장. 첫 번째 유효 플레이어를 찾음
	g.currentPlayerIdx = 0
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] != nil {
			g.currentPlayerIdx = i
			break
		}
	}
	if len(g.wall) > 0 {
		drawn := g.wall[0]
		g.wall = g.wall[1:]
		g.hands[g.currentPlayerIdx] = append(g.hands[g.currentPlayerIdx], drawn)
		sortMahjongHand(g.hands[g.currentPlayerIdx])
	}

	notice, _ := json.Marshal(ServerResponse{
		Type:    "game_notice",
		Message: "🀄 마작 시작! 14장이 되면 패를 버리세요.",
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)

	g.startTurnTimerLocked()
	g.sendStateToAllLocked()
	log.Printf("[MAHJONG] room:[%s] 라운드 시작", g.room.ID)
}

func (g *MahjongGame) buildMahjongDataForPlayer(viewerIdx int) MahjongData {
	players := make([]MahjongPlayerInfo, mahjongMaxPlayers)
	for i := 0; i < mahjongMaxPlayers; i++ {
		if g.players[i] != nil {
			discards := make([]MahjongTile, len(g.discards[i]))
			copy(discards, g.discards[i])
			melds := make([]MahjongMeld, len(g.melds[i]))
			copy(melds, g.melds[i])
			for j := range g.melds[i] {
				melds[j].Tiles = make([]MahjongTile, len(g.melds[i][j].Tiles))
				copy(melds[j].Tiles, g.melds[i][j].Tiles)
			}
			players[i] = MahjongPlayerInfo{
				UserID:    g.players[i].UserID,
				HandCount: len(g.hands[i]),
				Discards:  discards,
				Melds:     melds,
				IsTurn:    i == g.currentPlayerIdx,
			}
		} else {
			players[i] = MahjongPlayerInfo{UserID: ""}
		}
	}

	currentTurn := ""
	if g.currentPlayerIdx >= 0 && g.currentPlayerIdx < mahjongMaxPlayers && g.players[g.currentPlayerIdx] != nil {
		currentTurn = g.players[g.currentPlayerIdx].UserID
	}

	canTakeover := false
	if viewerIdx < 0 && !g.gameStarted {
		for i := 0; i < mahjongMaxPlayers; i++ {
			if g.players[i] == nil {
				canTakeover = true
				break
			}
		}
	}

	myHand := []MahjongTile{}
	if viewerIdx >= 0 {
		myHand = make([]MahjongTile, len(g.hands[viewerIdx]))
		copy(myHand, g.hands[viewerIdx])
	}

	callWindow := g.state == "call_window"
	var lastDiscard *MahjongTile
	lastDiscarderID := ""
	if callWindow {
		lastDiscard = &g.lastDiscard
		if g.players[g.lastDiscarderIdx] != nil {
			lastDiscarderID = g.players[g.lastDiscarderIdx].UserID
		}
	}

	return MahjongData{
		WallCount:       len(g.wall),
		Players:         players,
		CurrentTurn:     currentTurn,
		CanTakeover:     canTakeover,
		MyHand:          myHand,
		CallWindow:      callWindow,
		LastDiscard:     lastDiscard,
		LastDiscarderID: lastDiscarderID,
	}
}

func (g *MahjongGame) sendStateToAllLocked() {
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

func (g *MahjongGame) sendStateToPlayerLocked(client *Client, playerIdx int) {
	data := g.buildMahjongDataForPlayer(playerIdx)
	client.SendJSON(MahjongStateResponse{
		Type:   "mahjong_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

func (g *MahjongGame) sendStateToSpectatorLocked(client *Client) {
	data := g.buildMahjongDataForPlayer(-1)
	client.SendJSON(MahjongStateResponse{
		Type:   "mahjong_state",
		RoomID: g.room.ID,
		Data:   data,
	})
}

// sortMahjongHand는 리치 마작 순서(만→삭→통→자패)로 손패를 정렬합니다.
func sortMahjongHand(hand []MahjongTile) {
	suitOrder := map[string]int{"man": 1, "pin": 2, "sou": 3, "honor": 4}
	for i := 0; i < len(hand)-1; i++ {
		for j := i + 1; j < len(hand); j++ {
			t1, t2 := hand[i], hand[j]
			s1, s2 := suitOrder[t1.Type], suitOrder[t2.Type]
			if s1 == 0 {
				s1 = 4
			}
			if s2 == 0 {
				s2 = 4
			}
			if s1 > s2 || (s1 == s2 && t1.Value > t2.Value) {
				hand[i], hand[j] = hand[j], hand[i]
			}
		}
	}
}
