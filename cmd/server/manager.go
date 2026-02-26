package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

// ── 메시지 프로토콜 ────────────────────────────────────────────────────────────

// ClientRequest는 클라이언트로부터 수신하는 최상위 JSON 구조입니다.
// Payload는 action에 따라 내부 구조가 달라지므로 RawMessage로 받습니다.
type ClientRequest struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// ServerResponse는 클라이언트로 송신하는 표준 JSON 구조입니다.
// 게임 플러그인도 이 타입을 사용하여 공통된 응답 형식을 유지합니다.
type ServerResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	UserID  string `json:"userId,omitempty"`
	RoomID  string `json:"roomId,omitempty"`
}

// JoinPayload는 action: "join" 시 Payload 내부 구조입니다.
type JoinPayload struct {
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
}

// ChatPayload는 action: "chat" 시 Payload 내부 구조입니다.
type ChatPayload struct {
	Message string `json:"message"`
}

// ── 플러그인 레지스트리 (OCP) ───────────────────────────────────────────────────

// pluginRegistry는 접두사 → 플러그인 생성 함수 매핑입니다.
// 새 게임 추가 시 manager.go 수정 없이 plugin_*.go의 init()에서 등록합니다.
var pluginRegistry = make(map[string]PluginFactory)

// RegisterPlugin은 접두사와 플러그인 생성 함수를 레지스트리에 등록합니다.
// 각 plugin_*.go의 init() 함수에서 호출합니다.
func RegisterPlugin(prefix string, factory PluginFactory) {
	pluginRegistry[prefix] = factory
}

// ── Room ──────────────────────────────────────────────────────────────────────

// Room은 같은 방에 속한 클라이언트 집합과 게임 플러그인을 Thread-safe하게 관리합니다.
type Room struct {
	ID      string
	clients map[*Client]bool
	mu      sync.RWMutex
	Plugin  GamePlugin // 이 방에서 실행 중인 게임 플러그인 (nil 가능)
}

// newRoom은 플러그인 레지스트리에서 방 ID 접두사에 맞는 플러그인을 찾아 주입합니다.
// 새 게임 추가 시 manager.go 수정 없이 plugin_*.go의 init()에서 RegisterPlugin만 호출하면 됩니다.
func newRoom(id string) *Room {
	room := &Room{
		ID:      id,
		clients: make(map[*Client]bool),
	}

	// 가장 긴 접두사 우선 매칭 (blackjack_pve가 blackjack보다 먼저 매칭되도록)
	var bestPrefix string
	for prefix := range pluginRegistry {
		if strings.HasPrefix(id, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
		}
	}
	if bestPrefix != "" {
		room.Plugin = pluginRegistry[bestPrefix](room)
	}

	if room.Plugin != nil {
		log.Printf("[PLUGIN] room:[%s] — 플러그인 로드: %s", id, room.Plugin.Name())
	} else {
		log.Printf("[ROOM] room:[%s] — 채팅방 생성 (게임 플러그인 없음)", id)
	}
	return room
}

// broadcast는 방 안의 모든 클라이언트(exclude 제외)에게 메시지를 전송합니다.
func (r *Room) broadcast(msg []byte, exclude *Client) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for client := range r.clients {
		if client == exclude {
			continue
		}
		client.SafeSend(msg)
	}
}

func (r *Room) broadcastAll(msg []byte) {
	r.broadcast(msg, nil)
}

func (r *Room) count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

// ── RoomManager ───────────────────────────────────────────────────────────────

// RoomManager는 서버 전체의 방과 클라이언트를 Thread-safe하게 관리합니다.
// 게임 로직을 전혀 알지 못하며, 오직 GamePlugin 인터페이스를 통해서만 플러그인과 통신합니다.
type RoomManager struct {
	rooms map[string]*Room
	mu    sync.RWMutex
}

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*Room),
	}
}

// getOrCreateRoom은 roomID에 해당하는 방을 반환하며, 없으면 새로 생성합니다.
func (m *RoomManager) getOrCreateRoom(roomID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	if room, ok := m.rooms[roomID]; ok {
		return room
	}
	room := newRoom(roomID)
	m.rooms[roomID] = room
	log.Printf("[ROOM] 새 방 생성: [%s]", roomID)
	return room
}

// JoinRoom은 클라이언트를 지정한 방에 입장시키고, 방 전체에 브로드캐스트합니다.
// 이미 다른 방에 있다면 먼저 퇴장 처리합니다.
// 입장 완료 후 플러그인의 OnJoin 훅을 호출합니다.
func (m *RoomManager) JoinRoom(roomID, userID string, client *Client) {
	if client.RoomID != "" && client.RoomID != roomID {
		m.leaveRoom(client)
	}

	isPVE := strings.Contains(roomID, "_pve_")
	room := m.getOrCreateRoom(roomID)

	// PVE 방: 이미 사람(Human) 1명이 있으면 다른 사람 입장 차단
	if isPVE && !client.IsBot {
		room.mu.Lock()
		humanCount := 0
		for c := range room.clients {
			if !c.IsBot {
				humanCount++
			}
		}
		room.mu.Unlock()
		if humanCount >= 1 {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "PVE 모드는 혼자서만 플레이할 수 있습니다.",
			})
			return
		}
	}

	room.mu.Lock()
	room.clients[client] = true
	room.mu.Unlock()

	client.UserID = userID
	client.RoomID = roomID

	log.Printf("[JOIN] [%s] → room:[%s] (현재 %d명)", userID, roomID, room.count())

	// 코어 입장 알림 먼저 브로드캐스트
	resp := ServerResponse{
		Type:    "join",
		Message: fmt.Sprintf("[%s] 님이 입장했습니다", userID),
		UserID:  userID,
		RoomID:  roomID,
	}
	data, _ := json.Marshal(resp)
	room.broadcastAll(data)

	// 이후 플러그인 훅 호출 — 게임 상태 안내 등 플러그인 고유 처리
	// ※ DB 연동(전적 복구)은 auth 액션 처리 시점으로 이동됨
	if room.Plugin != nil {
		room.Plugin.OnJoin(client)
	}
	// PVE 방: human 입장 직후 빈 자리에 봇 자동 소환
	if isPVE && !client.IsBot {
		parts := strings.Split(roomID, "_pve_")
		gamePrefix := "blackjack"
		if len(parts) >= 1 && parts[0] != "" {
			gamePrefix = parts[0]
		}
		SpawnBotsForPVE(m, room, gamePrefix)
	}
}

// leaveRoom은 클라이언트를 현재 방에서 퇴장시키고, 남은 인원에게 브로드캐스트합니다.
// 빈 방이 되면 자동으로 삭제합니다.
// 클라이언트 제거 후 플러그인의 OnLeave 훅을 호출합니다.
func (m *RoomManager) leaveRoom(client *Client) {
	if client.RoomID == "" {
		return
	}

	m.mu.RLock()
	room, ok := m.rooms[client.RoomID]
	m.mu.RUnlock()
	if !ok {
		client.RoomID = ""
		return
	}

	room.mu.Lock()
	delete(room.clients, client)
	remaining := len(room.clients)
	room.mu.Unlock()

	userID := client.UserID
	roomID := client.RoomID

	// 플러그인 훅 호출 — client.RoomID가 아직 유효한 상태에서 호출
	// 플러그인은 이 시점에 게임 상태 정리 및 남은 플레이어 알림을 처리할 수 있습니다.
	if room.Plugin != nil {
		room.Plugin.OnLeave(client, remaining)
	}

	client.RoomID = ""

	log.Printf("[LEAVE] [%s] ← room:[%s] (남은 %d명)", userID, roomID, remaining)

	resp := ServerResponse{
		Type:    "leave",
		Message: fmt.Sprintf("[%s] 님이 퇴장했습니다", userID),
		UserID:  userID,
		RoomID:  roomID,
	}
	data, _ := json.Marshal(resp)
	room.broadcastAll(data)

	// 빈 방이 되면 메모리에서 제거합니다.
	if remaining == 0 {
		m.mu.Lock()
		delete(m.rooms, roomID)
		m.mu.Unlock()
		log.Printf("[ROOM] 빈 방 삭제: [%s]", roomID)
	}
}

// RemoveClient는 연결이 끊긴 클라이언트를 방에서 퇴장시키고 send 채널을 닫습니다.
// readPump의 defer에서 호출됩니다.
func (m *RoomManager) RemoveClient(client *Client) {
	m.leaveRoom(client)
	client.closeOnce()
}

// BroadcastToRoom은 특정 방의 클라이언트 전체(exclude 제외)에게 메시지를 전송합니다.
func (m *RoomManager) BroadcastToRoom(roomID string, msg []byte, exclude *Client) {
	m.mu.RLock()
	room, ok := m.rooms[roomID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	room.broadcast(msg, exclude)
}

// broadcastRoomUpdate는 방 인원 수(pCount) 변경 시 프론트엔드의 ready_count UI를 갱신하기 위해 ready_update를 브로드캐스트합니다.
func (m *RoomManager) broadcastRoomUpdate(room *Room) {
	pCount := room.count()
	upd, _ := json.Marshal(ReadyUpdateMessage{
		Type:       "ready_update",
		RoomID:     room.ID,
		ReadyCount: 0,
		TotalCount: pCount,
	})
	room.broadcastAll(upd)
}

// ── 메시지 핸들러 ─────────────────────────────────────────────────────────────

// HandleMessage는 클라이언트로부터 수신한 원시 JSON을 파싱하고
// action에 따라 적절한 처리를 수행합니다.
//
// 코어가 처리하는 action: auth, join, chat, leave
// 플러그인으로 위임하는 action: game_action → room.Plugin.HandleAction()
func (m *RoomManager) HandleMessage(client *Client, rawMsg []byte) {
	var req ClientRequest
	if err := json.Unmarshal(rawMsg, &req); err != nil {
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("잘못된 JSON 형식입니다: %v", err),
		})
		return
	}

	switch req.Action {

	// ── Auth: JWT 토큰 검증 → UserID/UserUUID 설정 → 전적 복구 ─────────────
	case "auth":
		var p struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.Token == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "auth payload에 token이 필요합니다"})
			return
		}
		if db == nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "서버 DB 미연결 — 인증 불가"})
			return
		}
		uuid, email, err := db.ValidateAuthToken(p.Token)
		if err != nil {
			log.Printf("[AUTH] 토큰 검증 실패: %v", err)
			client.SendJSON(ServerResponse{Type: "error", Message: "인증 실패: 토큰이 유효하지 않습니다"})
			return
		}
		client.UserUUID = uuid
		client.Token = p.Token

		// 닉네임 조회: profiles에 있으면 UserID=닉네임, 없으면 이메일 ID부분(골뱅이 앞) 임시 설정
		nickname := db.GetProfileByUUID(uuid)
		if nickname != "" {
			client.UserID = nickname
		} else {
			if idx := strings.Index(email, "@"); idx > 0 {
				client.UserID = email[:idx]
			} else {
				client.UserID = email
			}
		}
		log.Printf("[AUTH] 인증 완료: [%s] uuid=[%s]", client.UserID, uuid[:8])

		// 인증 성공 응답 (클라이언트는 이 응답을 받고 pendingJoin 처리)
		client.SendJSON(struct {
			Type    string `json:"type"`
			UserID  string `json:"userId"`
			Message string `json:"message"`
		}{"auth_ok", client.UserID, "인증 성공: " + client.UserID})

		// 전적 복구 (비동기)
		go func() {
			loaded := db.LoadUserRecords(uuid, p.Token)
			if loaded != nil {
				client.Records = loaded
				client.SendJSON(RecordUpdateResponse{Type: "record_update", Records: client.Records})
				log.Printf("[AUTH] [%s] 전적 복구 완료", client.UserID)
			}
		}()

	case "join":
		// 인증 가드: auth 액션을 먼저 통과해야 입장 가능
		if client.UserUUID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "먼저 인증이 필요합니다 (action: auth)"})
			return
		}
		var p JoinPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.RoomID == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "join payload에 roomId가 필요합니다",
			})
			return
		}
		// UserID는 auth 시점에 이미 설정됨 (이메일). payload의 userId는 무시.
		m.JoinRoom(p.RoomID, client.UserID, client)

	case "chat":
		if client.RoomID == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "먼저 방에 입장하세요 (action: join)",
			})
			return
		}
		var p ChatPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.Message == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "chat payload에 message가 필요합니다",
			})
			return
		}
		resp := ServerResponse{
			Type:    "chat",
			Message: p.Message,
			UserID:  client.UserID,
			RoomID:  client.RoomID,
		}
		data, _ := json.Marshal(resp)
		m.BroadcastToRoom(client.RoomID, data, nil)
		log.Printf("[CHAT] room:[%s] [%s]: %s", client.RoomID, client.UserID, p.Message)

	case "leave":
		if client.RoomID == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "현재 방에 있지 않습니다",
			})
			return
		}
		m.leaveRoom(client)

	case "check_nickname":
		if client.UserUUID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "먼저 인증이 필요합니다"})
			return
		}
		if db == nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "서버 DB 미연결"})
			return
		}
		var p struct {
			Nickname string `json:"nickname"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.Nickname == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "nickname이 필요합니다"})
			return
		}
		available := db.CheckNicknameUnique(p.Nickname, client.UserUUID)
		client.SendJSON(struct {
			Type      string `json:"type"`
			Available bool   `json:"available"`
		}{"nickname_check", available})

	case "set_nickname":
		if client.UserUUID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "먼저 인증이 필요합니다"})
			return
		}
		if db == nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "서버 DB 미연결"})
			return
		}
		var p struct {
			Nickname string `json:"nickname"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.Nickname == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "nickname이 필요합니다"})
			return
		}
		// 본인 현재 닉네임과 같으면 중복 검사 생략
		if p.Nickname != client.UserID && !db.CheckNicknameUnique(p.Nickname, client.UserUUID) {
			client.SendJSON(ServerResponse{Type: "error", Message: "이미 사용 중인 닉네임입니다"})
			return
		}
		if err := db.UpsertProfile(client.UserUUID, p.Nickname); err != nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "닉네임 저장 실패: " + err.Error()})
			return
		}
		client.UserID = p.Nickname
		client.SendJSON(struct {
			Type   string `json:"type"`
			UserID string `json:"userId"`
		}{"auth_ok", client.UserID})

	case "get_profile":
		if client.UserUUID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "먼저 인증이 필요합니다 (action: auth)"})
			return
		}
		if db == nil {
			client.SendJSON(RecordUpdateResponse{Type: "record_update", Records: client.Records})
			return
		}
		go func() {
			loaded := db.LoadUserRecords(client.UserUUID, client.Token)
			if loaded != nil {
				client.Records = loaded
				client.SendJSON(RecordUpdateResponse{Type: "record_update", Records: client.Records})
			}
		}()

	case "get_user_record":
		if client.RoomID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "방에 입장한 상태에서만 상대 전적을 조회할 수 있습니다"})
			return
		}
		var p struct {
			UserID string `json:"userId"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil || p.UserID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "userId가 필요합니다"})
			return
		}
		m.mu.RLock()
		room, ok := m.rooms[client.RoomID]
		m.mu.RUnlock()
		if !ok {
			client.SendJSON(ServerResponse{Type: "error", Message: "방을 찾을 수 없습니다"})
			return
		}
		room.mu.RLock()
		var target *Client
		for c := range room.clients {
			if c.UserID == p.UserID {
				target = c
				break
			}
		}
		room.mu.RUnlock()
		if target == nil {
			client.SendJSON(ServerResponse{Type: "error", Message: "해당 유저를 찾을 수 없습니다"})
			return
		}
		client.SendJSON(struct {
			Type    string                 `json:"type"`
			UserID  string                 `json:"userId"`
			Records map[string]*GameRecord `json:"records"`
		}{"opponent_record", p.UserID, target.Records})

	case "add_bot":
		if client.RoomID == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "먼저 방에 입장하세요 (action: join)",
			})
			return
		}
		m.mu.RLock()
		room, ok := m.rooms[client.RoomID]
		m.mu.RUnlock()
		if !ok || room.Plugin == nil {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "이 방에 활성화된 게임 플러그인이 없습니다",
			})
			return
		}
		// 오목, 4목, 틱택토, 인디언 포커, 홀덤, 세븐 포커만 AI 봇 지원 (blackjack 제외)
		prefix := ""
		if strings.HasPrefix(client.RoomID, "omok") {
			prefix = "omok"
		} else if strings.HasPrefix(client.RoomID, "connect4") {
			prefix = "connect4"
		} else if strings.HasPrefix(client.RoomID, "tictactoe") {
			prefix = "tictactoe"
		} else if strings.HasPrefix(client.RoomID, "indian") {
			prefix = "indian"
		} else if strings.HasPrefix(client.RoomID, "holdem") {
			prefix = "holdem"
		} else if strings.HasPrefix(client.RoomID, "sevenpoker") {
			prefix = "sevenpoker"
		} else if strings.HasPrefix(client.RoomID, "thief") {
			prefix = "thief"
		} else if strings.HasPrefix(client.RoomID, "onecard") {
			prefix = "onecard"
		} else if strings.HasPrefix(client.RoomID, "mahjong") {
			prefix = "mahjong"
		}
		if prefix == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "이 게임은 AI 봇을 지원하지 않습니다",
			})
			return
		}
		SpawnBot(m, room, prefix)

	case "game_action":
		// 코어는 게임 로직을 전혀 모릅니다.
		// 방에 연결된 플러그인에게 페이로드를 그대로 위임(토스)합니다.
		if client.UserUUID == "" {
			client.SendJSON(ServerResponse{Type: "error", Message: "먼저 인증이 필요합니다 (action: auth)"})
			return
		}
		if client.RoomID == "" {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "먼저 방에 입장하세요 (action: join)",
			})
			return
		}
		m.mu.RLock()
		room, ok := m.rooms[client.RoomID]
		m.mu.RUnlock()
		if !ok || room.Plugin == nil {
			client.SendJSON(ServerResponse{
				Type:    "error",
				Message: "이 방에 활성화된 게임 플러그인이 없습니다",
			})
			return
		}
		room.Plugin.HandleAction(client, req.Action, req.Payload)

	default:
		client.SendJSON(ServerResponse{
			Type:    "error",
			Message: fmt.Sprintf("알 수 없는 action: [%s]", req.Action),
		})
	}
}
