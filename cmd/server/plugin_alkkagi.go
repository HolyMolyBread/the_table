package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// AlkkagiStone은 알까기 돌 하나의 좌표/속도 데이터입니다.
type AlkkagiStone struct {
	ID     int     `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	VelX   float64 `json:"velX"`
	VelY   float64 `json:"velY"`
	Color  int     `json:"color"` // 1=흑, 2=백
	Angle  float64 `json:"angle,omitempty"`
}

// AlkkagiData는 alkkagi_state 응답의 data 필드입니다.
type AlkkagiData struct {
	CurrentTurn string         `json:"currentTurn"` // 현재 차례 유저 ID
	Players     [2]string      `json:"players"`     // [0]=흑, [1]=백
	Stones      []AlkkagiStone `json:"stones"`      // 각 알의 좌표/속도
}

// AlkkagiStateResponse는 알까기 게임 상태 응답입니다.
type AlkkagiStateResponse struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId"`
	Data   AlkkagiData `json:"data"`
}

// ── AlkkagiGame 플러그인 ───────────────────────────────────────────────────────

// AlkkagiGame은 클라이언트 주도권(Client Authority) 방식의 알까기 게임 플러그인입니다.
// 서버는 물리 연산을 하지 않고, 클라이언트가 보낸 sync/flick 메시지를 검증 후 중계합니다.
type AlkkagiGame struct {
	room        *Room
	players     [2]*Client // [0]=흑, [1]=백
	currentTurn int
	stones      []AlkkagiStone
	mu          sync.Mutex
}

func NewAlkkagiGame(room *Room) *AlkkagiGame {
	return &AlkkagiGame{
		room:        room,
		stones:      makeInitialStones(),
		currentTurn: 0,
	}
}

func init() { RegisterPlugin("alkkagi", func(room *Room) GamePlugin { return NewAlkkagiGame(room) }) }

func (g *AlkkagiGame) Name() string { return "알까기 (Alkkagi)" }

func makeInitialStones() []AlkkagiStone {
	stones := make([]AlkkagiStone, 0, 8)
	// 흑돌 4개 (아래쪽)
	for i := 0; i < 4; i++ {
		stones = append(stones, AlkkagiStone{
			ID:    i,
			X:     200 + float64(i)*60,
			Y:     350,
			Color: 1,
		})
	}
	// 백돌 4개 (위쪽)
	for i := 0; i < 4; i++ {
		stones = append(stones, AlkkagiStone{
			ID:    4 + i,
			X:     200 + float64(i)*60,
			Y:     50,
			Color: 2,
		})
	}
	return stones
}

// OnJoin은 플레이어 입장 시 호출됩니다.
func (g *AlkkagiGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] == client {
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
		Message: fmt.Sprintf("알까기 [%s]님이 입장했습니다. (%d/2)", client.UserID, slot+1),
		RoomID:  g.room.ID,
	})
	g.room.broadcastAll(notice)
	g.sendStateToAllLocked()
}

// OnLeave는 플레이어 퇴장 시 호출됩니다.
func (g *AlkkagiGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] == client {
			g.players[i] = nil
			break
		}
	}
	if remainingCount == 0 {
		log.Printf("[alkkagi] 방 [%s] 비어서 초기화", g.room.ID)
		g.stones = makeInitialStones()
		g.currentTurn = 0
	}
}

// HandleAction은 game_action 메시지를 처리합니다.
func (g *AlkkagiGame) HandleAction(client *Client, action string, payload json.RawMessage) {
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
	case "sync":
		var p struct {
			Stones []AlkkagiStone `json:"stones"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "sync 페이로드 파싱 오류"})
			return
		}
		if len(p.Stones) > 16 {
			client.SendJSON(ServerResponse{Type: "error", Message: "stones 개수 오류 (0~16)"})
			return
		}
		for i := range p.Stones {
			if p.Stones[i].ID < 0 || p.Stones[i].ID >= 16 {
				client.SendJSON(ServerResponse{Type: "error", Message: "stone id 범위 오류"})
				return
			}
		}
		g.stones = p.Stones
		g.broadcastStateLocked()

	case "flick":
		// 턴 검증: 요청한 client가 현재 차례인지 확인
		if g.currentTurn < 0 || g.currentTurn >= 2 || g.players[g.currentTurn] == nil || g.players[g.currentTurn].UserID != client.UserID {
			client.SendJSON(ServerResponse{Type: "error", Message: "내 차례가 아닙니다."})
			return
		}
		var p struct {
			ID     int     `json:"id"`
			ForceX float64 `json:"forceX"`
			ForceY float64 `json:"forceY"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "flick 페이로드 파싱 오류"})
			return
		}
		if p.ID < 0 || p.ID >= 16 {
			client.SendJSON(ServerResponse{Type: "error", Message: "stone id 범위 오류"})
			return
		}
		// 힘 크기 제한 (과도한 값 방지)
		if p.ForceX < -2 || p.ForceX > 2 || p.ForceY < -2 || p.ForceY > 2 {
			client.SendJSON(ServerResponse{Type: "error", Message: "force 범위 오류 (-2 ~ 2)"})
			return
		}
		// flick 액션을 그대로 브로드캐스트 (클라이언트가 물리 연산)
		msg, _ := json.Marshal(map[string]any{
			"type":   "alkkagi_flick",
			"roomId": g.room.ID,
			"data": map[string]any{
				"userId": client.UserID,
				"id":     p.ID,
				"forceX": p.ForceX,
				"forceY": p.ForceY,
			},
		})
		g.room.broadcastAll(msg)
		g.currentTurn = 1 - g.currentTurn

	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 cmd: %s", base.Cmd)})
	}
}

func (g *AlkkagiGame) sendStateToAllLocked() {
	msg, _ := json.Marshal(AlkkagiStateResponse{
		Type:   "alkkagi_state",
		RoomID: g.room.ID,
		Data: AlkkagiData{
			CurrentTurn: g.turnUserIDLocked(),
			Players:     g.playersUserIDsLocked(),
			Stones:      g.stones,
		},
	})
	g.room.broadcastAll(msg)
}

func (g *AlkkagiGame) broadcastStateLocked() {
	g.sendStateToAllLocked()
}

func (g *AlkkagiGame) sendStateToClientLocked(client *Client) {
	msg, _ := json.Marshal(AlkkagiStateResponse{
		Type:   "alkkagi_state",
		RoomID: g.room.ID,
		Data: AlkkagiData{
			CurrentTurn: g.turnUserIDLocked(),
			Players:     g.playersUserIDsLocked(),
			Stones:      g.stones,
		},
	})
	client.SafeSend(msg)
}

func (g *AlkkagiGame) turnUserIDLocked() string {
	if g.currentTurn >= 0 && g.currentTurn < 2 && g.players[g.currentTurn] != nil {
		return g.players[g.currentTurn].UserID
	}
	return ""
}

func (g *AlkkagiGame) playersUserIDsLocked() [2]string {
	var out [2]string
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			out[i] = g.players[i].UserID
		}
	}
	return out
}
