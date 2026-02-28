package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	alkkagiStonesPerPlayer = 7
	alkkagiPlacementTime   = 20
	alkkagiBoardSize       = 15
	alkkagiCellPx          = 28
)

// ── 응답 타입 ─────────────────────────────────────────────────────────────────

// AlkkagiStone은 알까기 돌 하나의 좌표/속도/역할 데이터입니다.
type AlkkagiStone struct {
	ID    int     `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	VelX  float64 `json:"velX"`
	VelY  float64 `json:"velY"`
	Color int     `json:"color"` // 1=흑, 2=백
	Role  string  `json:"role"`  // K, R, P, H, E (장기: 궁, 차, 포, 마, 상)
	Angle float64 `json:"angle,omitempty"`
}

// AlkkagiData는 alkkagi_state 응답의 data 필드입니다.
type AlkkagiData struct {
	Mode               string         `json:"mode"`               // "original" | "janggi" | "chess"
	Phase              string         `json:"phase"`              // "ready" | "placement" | "playing"
	CurrentTurn        string         `json:"currentTurn"`        // 현재 차례 유저 ID (playing 시)
	Players            [2]string      `json:"players"`            // [0]=한(漢), [1]=초(楚)
	Teams              [2]string      `json:"teams"`              // [0]="한", [1]="초"
	Stones             []AlkkagiStone `json:"stones"`             // 각 알의 좌표/속도
	PlacementRemaining int            `json:"placementRemaining"` // 배치 페이즈 남은 초 (0이면 무관)
	NextRoleBlack      string         `json:"nextRoleBlack"`       // 한(1)의 다음 배치 역할
	NextRoleWhite      string         `json:"nextRoleWhite"`       // 초(2)의 다음 배치 역할
}

// AlkkagiStateResponse는 알까기 게임 상태 응답입니다.
type AlkkagiStateResponse struct {
	Type   string      `json:"type"`
	RoomID string      `json:"roomId"`
	Data   AlkkagiData `json:"data"`
}

// ── AlkkagiGame 플러그인 ───────────────────────────────────────────────────────

type AlkkagiGame struct {
	room         *Room
	mode         string // "original" | "janggi" | "chess"
	players      [2]*Client // [0]=흑, [1]=백 (랜덤 배정 후)
	currentTurn  int
	stones       []AlkkagiStone
	phase        string // "ready" | "placement" | "playing"
	startReady   map[*Client]bool
	placementCnt [2]int  // 각 색상별 배치된 돌 개수
	stopTick     chan struct{}
	mu           sync.Mutex
}

func NewAlkkagiGame(room *Room) *AlkkagiGame {
	g := &AlkkagiGame{
		room:       room,
		phase:      "ready",
		startReady: make(map[*Client]bool),
	}
	// room.ID 접두사로 모드 설정: alkkagi_chess_... -> "chess", alkkagi_janggi_... -> "janggi", alkkagi_original_... -> "original"
	id := room.ID
	if strings.Contains(id, "chess") {
		g.mode = "chess"
	} else if strings.Contains(id, "janggi") {
		g.mode = "janggi"
	} else if strings.Contains(id, "original") {
		g.mode = "original"
	} else {
		g.mode = "janggi" // alkkagi_XXX 하위 호환
	}
	return g
}

func init() {
	factory := func(room *Room) GamePlugin { return NewAlkkagiGame(room) }
	RegisterPlugin("alkkagi_janggi", factory)
	RegisterPlugin("alkkagi_chess", factory)
	RegisterPlugin("alkkagi_original", factory)
	RegisterPlugin("alkkagi", factory) // 하위 호환: alkkagi_XXX → janggi 모드
}

func (g *AlkkagiGame) Name() string { return "알까기 (Alkkagi)" }

// cellToPx converts grid (col, row) to pixel center.
func cellToPx(col, row int) (x, y float64) {
	x = (float64(col) + 0.5) * float64(alkkagiCellPx)
	y = (float64(row) + 0.5) * float64(alkkagiCellPx)
	return x, y
}

// alkkagiJanggiRoles: 궁(K), 차(R), 포(P), 마(H), 상(E), 사/졸(S)×2
var alkkagiJanggiRoles = [7]string{"K", "R", "P", "H", "E", "S", "S"}

// alkkagiChessRoles: King(K), Queen(Q), Rook(R), Bishop(B), Knight(N), Pawn(P)×2
var alkkagiChessRoles = [7]string{"K", "Q", "R", "B", "N", "P", "P"}

// makeOriginalStones returns 5 plain stones per side with no role.
func makeOriginalStones() []AlkkagiStone {
	stones := make([]AlkkagiStone, 0, 10)
	hanCells := [][2]int{{2, 11}, {6, 11}, {10, 11}, {3, 13}, {9, 13}}
	for i, c := range hanCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: i, X: x, Y: y, Color: 1, Role: ""})
	}
	choCells := [][2]int{{2, 3}, {6, 3}, {10, 3}, {3, 1}, {9, 1}}
	for i, c := range choCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: 5 + i, X: x, Y: y, Color: 2, Role: ""})
	}
	return stones
}

// makeJanggiStones returns 5 한(漢) + 5 초(楚) stones with 장기 roles.
// Color 1=한(빨강), Color 2=초(초록/파랑)
func makeJanggiStones() []AlkkagiStone {
	stones := make([]AlkkagiStone, 0, 10)
	hanCells := [][2]int{{2, 11}, {6, 11}, {10, 11}, {3, 13}, {9, 13}}
	for i, c := range hanCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: i, X: x, Y: y, Color: 1, Role: alkkagiJanggiRoles[i]})
	}
	choCells := [][2]int{{2, 3}, {6, 3}, {10, 3}, {3, 1}, {9, 1}}
	for i, c := range choCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: 5 + i, X: x, Y: y, Color: 2, Role: alkkagiJanggiRoles[i]})
	}
	return stones
}

// makeChessStones returns 5 King/Queen/Rook/Bishop/Knight per side.
func makeChessStones() []AlkkagiStone {
	stones := make([]AlkkagiStone, 0, 10)
	hanCells := [][2]int{{2, 11}, {6, 11}, {10, 11}, {3, 13}, {9, 13}}
	for i, c := range hanCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: i, X: x, Y: y, Color: 1, Role: alkkagiChessRoles[i]})
	}
	choCells := [][2]int{{2, 3}, {6, 3}, {10, 3}, {3, 1}, {9, 1}}
	for i, c := range choCells {
		x, y := cellToPx(c[0], c[1])
		stones = append(stones, AlkkagiStone{ID: 5 + i, X: x, Y: y, Color: 2, Role: alkkagiChessRoles[i]})
	}
	return stones
}

func (g *AlkkagiGame) OnJoin(client *Client) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			g.players[i] = client
			if g.startReady[g.players[i]] {
				// keep ready state
			}
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

	readyCount := 0
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total,
	})
	g.room.broadcastAll(upd)
	g.sendStateToAllLocked()
}

func (g *AlkkagiGame) OnLeave(client *Client, remainingCount int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < 2; i++ {
		if g.players[i] == client {
			g.players[i] = nil
			delete(g.startReady, client)
			break
		}
	}
	if remainingCount == 0 {
		log.Printf("[alkkagi] 방 [%s] 비어서 초기화", g.room.ID)
		g.phase = "ready"
		g.stones = nil
		g.currentTurn = 0
		g.placementCnt = [2]int{0, 0}
		g.stopPlacementTimerLocked()
	}
}

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
	case "ready":
		g.handleReadyLocked(client)
	case "place":
		g.handlePlaceLocked(client, payload)
	case "sync":
		g.handleSyncLocked(client, payload)
	case "flick":
		g.handleFlickLocked(client, payload)
	default:
		client.SendJSON(ServerResponse{Type: "error", Message: fmt.Sprintf("알 수 없는 cmd: %s", base.Cmd)})
	}
}

func (g *AlkkagiGame) handleReadyLocked(client *Client) {
	if g.phase != "ready" {
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
	g.startReady[client] = true

	readyCount := 0
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
			if g.startReady[g.players[i]] {
				readyCount++
			}
		}
	}
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: readyCount, TotalCount: total,
	})
	g.room.broadcastAll(upd)

	if readyCount >= 2 && total >= 2 {
		g.startPlacementPhaseLocked()
	} else {
		g.sendStateToAllLocked()
	}
}

func (g *AlkkagiGame) startPlacementPhaseLocked() {
	g.startReady = make(map[*Client]bool)
	g.phase = "placement"
	g.placementCnt = [2]int{0, 0}
	g.stones = make([]AlkkagiStone, 0, alkkagiStonesPerPlayer*2)
	// 모드는 NewAlkkagiGame에서 room.ID 접두사로 이미 설정됨

	// 랜덤 배정: 0/1 순서를 섞음
	order := []int{0, 1}
	if rand.Intn(2) == 1 {
		order[0], order[1] = order[1], order[0]
	}
	// order[0]이 흑(선공), order[1]이 백
	shuffled := [2]*Client{g.players[order[0]], g.players[order[1]]}
	g.players = shuffled
	g.currentTurn = 0

	g.sendStateWithRemainingLocked(alkkagiPlacementTime)
	g.startPlacementTimerLocked()
}

func (g *AlkkagiGame) startPlacementTimerLocked() {
	g.stopPlacementTimerLocked()
	stopCh := make(chan struct{})
	g.stopTick = stopCh
	room := g.room

	remaining := alkkagiPlacementTime
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for remaining > 0 {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				remaining--
				room.Plugin.(*AlkkagiGame).mu.Lock()
				room.Plugin.(*AlkkagiGame).phase = "placement"
				room.Plugin.(*AlkkagiGame).sendStateWithRemainingLocked(remaining)
				room.Plugin.(*AlkkagiGame).mu.Unlock()
			}
		}
		room.Plugin.(*AlkkagiGame).placementTimeout()
	}()
}

func (g *AlkkagiGame) stopPlacementTimerLocked() {
	if g.stopTick != nil {
		close(g.stopTick)
		g.stopTick = nil
	}
}

func (g *AlkkagiGame) getRoleForPlacementLocked(slotIdx int) string {
	switch g.mode {
	case "janggi":
		return alkkagiJanggiRoles[slotIdx]
	case "chess":
		return alkkagiChessRoles[slotIdx]
	default:
		return ""
	}
}

// getNextRolesLocked returns (nextRoleBlack, nextRoleWhite) for placement phase.
func (g *AlkkagiGame) getNextRolesLocked() (black, white string) {
	if g.placementCnt[0] < alkkagiStonesPerPlayer {
		black = g.getRoleForPlacementLocked(g.placementCnt[0])
	}
	if g.placementCnt[1] < alkkagiStonesPerPlayer {
		white = g.getRoleForPlacementLocked(g.placementCnt[1])
	}
	return black, white
}

func (g *AlkkagiGame) sendStateWithRemainingLocked(remaining int) {
	nb, nw := g.getNextRolesLocked()
	msg, _ := json.Marshal(AlkkagiStateResponse{
		Type:   "alkkagi_state",
		RoomID: g.room.ID,
		Data: AlkkagiData{
			Mode:               g.mode,
			Phase:              g.phase,
			CurrentTurn:        g.turnUserIDLocked(),
			Players:            g.playersUserIDsLocked(),
			Teams:              g.teamsLocked(),
			Stones:             g.stones,
			PlacementRemaining: remaining,
			NextRoleBlack:      nb,
			NextRoleWhite:      nw,
		},
	})
	g.room.broadcastAll(msg)
}

func (g *AlkkagiGame) placementTimeout() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "placement" {
		return
	}
	g.stopPlacementTimerLocked()
	placed := make(map[[2]int]bool)
	for _, s := range g.stones {
		col := int(s.X/float64(alkkagiCellPx) + 0.5)
		row := int(s.Y/float64(alkkagiCellPx) + 0.5)
		placed[[2]int{col, row}] = true
	}
	blackDefaults := [][2]int{{2, 11}, {6, 11}, {10, 11}, {3, 13}, {9, 13}, {4, 12}, {8, 12}}
	whiteDefaults := [][2]int{{2, 3}, {6, 3}, {10, 3}, {3, 1}, {9, 1}, {4, 2}, {8, 2}}
	findEmpty := func(minRow, maxRow int) (int, int) {
		for cc := 0; cc < alkkagiBoardSize; cc++ {
			for rr := minRow; rr <= maxRow; rr++ {
				if !placed[[2]int{cc, rr}] {
					return cc, rr
				}
			}
		}
		return 0, minRow
	}
	for i := g.placementCnt[0]; i < alkkagiStonesPerPlayer; i++ {
		c := blackDefaults[i]
		col, row := c[0], c[1]
		if placed[[2]int{col, row}] {
			col, row = findEmpty(10, 14)
		}
		placed[[2]int{col, row}] = true
		x, y := cellToPx(col, row)
		g.stones = append(g.stones, AlkkagiStone{ID: i, X: x, Y: y, Color: 1, Role: g.getRoleForPlacementLocked(i)})
	}
	for i := g.placementCnt[1]; i < alkkagiStonesPerPlayer; i++ {
		c := whiteDefaults[i]
		col, row := c[0], c[1]
		if placed[[2]int{col, row}] {
			col, row = findEmpty(0, 4)
		}
		placed[[2]int{col, row}] = true
		x, y := cellToPx(col, row)
		g.stones = append(g.stones, AlkkagiStone{ID: 5 + i, X: x, Y: y, Color: 2, Role: g.getRoleForPlacementLocked(i)})
	}
	g.phase = "playing"
	g.sendStateToAllLocked()
}

func (g *AlkkagiGame) handlePlaceLocked(client *Client, payload json.RawMessage) {
	if g.phase != "placement" {
		client.SendJSON(ServerResponse{Type: "error", Message: "배치 페이즈가 아닙니다."})
		return
	}
	var p struct {
		Col int `json:"col"`
		Row int `json:"row"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "place 페이로드 파싱 오류"})
		return
	}
	if p.Col < 0 || p.Col >= alkkagiBoardSize || p.Row < 0 || p.Row >= alkkagiBoardSize {
		client.SendJSON(ServerResponse{Type: "error", Message: "격자 범위 오류 (0~14)"})
		return
	}

	color := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil && g.players[i].UserID == client.UserID {
			color = i + 1
			break
		}
	}
	if color == 0 {
		client.SendJSON(ServerResponse{Type: "error", Message: "플레이어가 아닙니다."})
		return
	}

	// color 1: row 10~14 (bottom), color 2: row 0~4 (top)
	if color == 1 && (p.Row < 10 || p.Row > 14) {
		msg := "한(漢)은 아래쪽 5줄(10~14행)에만 배치할 수 있습니다."
		if g.mode == "chess" {
			msg = "White는 아래쪽 5줄(10~14행)에만 배치할 수 있습니다."
		} else if g.mode == "original" {
			msg = "Black은 아래쪽 5줄(10~14행)에만 배치할 수 있습니다."
		}
		client.SendJSON(ServerResponse{Type: "error", Message: msg})
		return
	}
	if color == 2 && (p.Row < 0 || p.Row > 4) {
		msg := "초(楚)는 위쪽 5줄(0~4행)에만 배치할 수 있습니다."
		if g.mode == "chess" {
			msg = "Black은 위쪽 5줄(0~4행)에만 배치할 수 있습니다."
		} else if g.mode == "original" {
			msg = "White는 위쪽 5줄(0~4행)에만 배치할 수 있습니다."
		}
		client.SendJSON(ServerResponse{Type: "error", Message: msg})
		return
	}

	if g.placementCnt[color-1] >= alkkagiStonesPerPlayer {
		client.SendJSON(ServerResponse{Type: "error", Message: "이미 7개를 모두 배치했습니다."})
		return
	}

	for _, s := range g.stones {
		sc := int(s.X / float64(alkkagiCellPx))
		sr := int(s.Y / float64(alkkagiCellPx))
		if sc == p.Col && sr == p.Row {
			client.SendJSON(ServerResponse{Type: "error", Message: "이미 돌이 있는 칸입니다."})
			return
		}
	}

	x, y := cellToPx(p.Col, p.Row)
	id := (color-1)*alkkagiStonesPerPlayer + g.placementCnt[color-1]
	role := g.getRoleForPlacementLocked(g.placementCnt[color-1])
	g.stones = append(g.stones, AlkkagiStone{ID: id, X: x, Y: y, Color: color, Role: role})
	g.placementCnt[color-1]++

	g.sendStateToAllLocked()
}

func (g *AlkkagiGame) handleSyncLocked(client *Client, payload json.RawMessage) {
	var p struct {
		Stones []AlkkagiStone `json:"stones"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		client.SendJSON(ServerResponse{Type: "error", Message: "sync 페이로드 파싱 오류"})
		return
	}
	if len(p.Stones) > alkkagiStonesPerPlayer*2+4 {
		client.SendJSON(ServerResponse{Type: "error", Message: "stones 개수 오류"})
		return
	}
	roleByID := make(map[int]string)
	for _, s := range g.stones {
		if s.Role != "" {
			roleByID[s.ID] = s.Role
		}
	}
	// original 모드: Role 비움. janggi/chess: 기존 Role 유지, 없으면 기본값
	for i := range p.Stones {
		if g.mode == "original" {
			p.Stones[i].Role = ""
		} else if p.Stones[i].Role == "" {
			if r, ok := roleByID[p.Stones[i].ID]; ok {
				p.Stones[i].Role = r
			} else {
				if g.mode == "chess" {
					p.Stones[i].Role = "N"
				} else {
					p.Stones[i].Role = "E"
				}
			}
		}
	}
	g.stones = p.Stones
	g.broadcastStateLocked()

	// 승패 판정: 특정 색상 돌이 0개면 해당 유저 패배
	blackCount, whiteCount := 0, 0
	for _, s := range g.stones {
		if s.Color == 1 {
			blackCount++
		} else if s.Color == 2 {
			whiteCount++
		}
	}
	if blackCount == 0 {
		msg := "초(楚) 승리! 한(漢) 기물이 모두 밀려났습니다."
		if g.mode == "chess" {
			msg = "Black 승리! White 기물이 모두 밀려났습니다."
		} else if g.mode == "original" {
			msg = "White 승리! Black 기물이 모두 밀려났습니다."
		}
		g.endMatchLocked(1, msg)
		return
	}
	if whiteCount == 0 {
		msg := "한(漢) 승리! 초(楚) 기물이 모두 밀려났습니다."
		if g.mode == "chess" {
			msg = "White 승리! Black 기물이 모두 밀려났습니다."
		} else if g.mode == "original" {
			msg = "Black 승리! White 기물이 모두 밀려났습니다."
		}
		g.endMatchLocked(0, msg)
		return
	}
}

func (g *AlkkagiGame) endMatchLocked(winnerSlot int, msg string) {
	g.phase = "ready"
	g.stopPlacementTimerLocked()
	if g.players[winnerSlot] != nil {
		g.players[winnerSlot].RecordResult("alkkagi", "win")
	}
	loserSlot := 1 - winnerSlot
	if g.players[loserSlot] != nil {
		g.players[loserSlot].RecordResult("alkkagi", "lose")
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
	g.stones = nil
	g.placementCnt = [2]int{0, 0}
	g.startReady = make(map[*Client]bool)
	total := 0
	for i := 0; i < 2; i++ {
		if g.players[i] != nil {
			total++
		}
	}
	g.sendStateToAllLocked()
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type: "ready_update", RoomID: g.room.ID, ReadyCount: 0, TotalCount: total,
	})
	g.room.broadcastAll(upd)
}

func (g *AlkkagiGame) handleFlickLocked(client *Client, payload json.RawMessage) {
	if g.phase != "playing" {
		client.SendJSON(ServerResponse{Type: "error", Message: "대전 중이 아닙니다."})
		return
	}
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
	if p.ID < 0 || p.ID >= alkkagiStonesPerPlayer*2 {
		client.SendJSON(ServerResponse{Type: "error", Message: "stone id 범위 오류"})
		return
	}
	if p.ForceX < -2 || p.ForceX > 2 || p.ForceY < -2 || p.ForceY > 2 {
		client.SendJSON(ServerResponse{Type: "error", Message: "force 범위 오류 (-2 ~ 2)"})
		return
	}
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
	g.broadcastStateLocked()
}

func (g *AlkkagiGame) sendStateToAllLocked() {
	nb, nw := g.getNextRolesLocked()
	msg, _ := json.Marshal(AlkkagiStateResponse{
		Type:   "alkkagi_state",
		RoomID: g.room.ID,
		Data: AlkkagiData{
			Mode:               g.mode,
			Phase:              g.phase,
			CurrentTurn:        g.turnUserIDLocked(),
			Players:            g.playersUserIDsLocked(),
			Teams:              g.teamsLocked(),
			Stones:             g.stones,
			PlacementRemaining: 0,
			NextRoleBlack:      nb,
			NextRoleWhite:      nw,
		},
	})
	g.room.broadcastAll(msg)
}

func (g *AlkkagiGame) broadcastStateLocked() {
	g.sendStateToAllLocked()
}

func (g *AlkkagiGame) sendStateToClientLocked(client *Client) {
	nb, nw := g.getNextRolesLocked()
	msg, _ := json.Marshal(AlkkagiStateResponse{
		Type:   "alkkagi_state",
		RoomID: g.room.ID,
		Data: AlkkagiData{
			Mode:               g.mode,
			Phase:              g.phase,
			CurrentTurn:        g.turnUserIDLocked(),
			Players:            g.playersUserIDsLocked(),
			Teams:              g.teamsLocked(),
			Stones:             g.stones,
			PlacementRemaining: 0,
			NextRoleBlack:      nb,
			NextRoleWhite:      nw,
		},
	})
	client.SafeSend(msg)
}

func (g *AlkkagiGame) teamsLocked() [2]string {
	switch g.mode {
	case "original":
		return [2]string{"Black", "White"}
	case "chess":
		return [2]string{"White", "Black"}
	default:
		return [2]string{"한", "초"}
	}
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
