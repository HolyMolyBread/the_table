package main

import "encoding/json"

// GamePlugin은 게임 룰 플러그인이 구현해야 하는 인터페이스입니다.
//
// 코어 서버(manager.go)는 이 인터페이스만 알고 있으며,
// 구체적인 게임 로직은 플러그인 파일(plugin_*.go)에 완전히 캡슐화됩니다.
// 새로운 게임을 추가할 때는 이 인터페이스를 구현하고 newRoom에 주입하기만 하면 됩니다.
type GamePlugin interface {
	// Name은 게임의 식별 이름을 반환합니다.
	Name() string

	// OnJoin은 플레이어가 방에 입장한 직후 호출되는 훅(Hook)입니다.
	OnJoin(client *Client)

	// OnLeave는 플레이어가 방에서 퇴장하기 직전에 호출되는 훅입니다.
	OnLeave(client *Client)

	// HandleAction은 action: "game_action" 메시지가 도착했을 때
	// 코어 매니저로부터 페이로드를 위임받아 처리합니다.
	HandleAction(client *Client, action string, payload json.RawMessage)
}

// ── 공유 응답 타입 ─────────────────────────────────────────────────────────────

// GameResultResponse는 게임 결과를 방 전체에 브로드캐스트할 때 사용합니다.
// Data 필드는 게임마다 다른 구조를 가질 수 있도록 any 타입으로 유연하게 정의합니다.
type GameResultResponse struct {
	Type           string `json:"type"`
	Message        string `json:"message"`
	RoomID         string `json:"roomId,omitempty"`
	Data           any    `json:"data,omitempty"`
	RematchEnabled bool   `json:"rematchEnabled,omitempty"` // Phase 4.3: 리매치 가능 여부
}

// RematchUpdateMessage는 리매치 레디 상태를 방 전체에 알릴 때 사용합니다.
type RematchUpdateMessage struct {
	Type       string `json:"type"`       // "rematch_update"
	RoomID     string `json:"roomId"`
	ReadyCount int    `json:"readyCount"` // 현재 레디한 플레이어 수
	TotalCount int    `json:"totalCount"` // 방 전체 인원 수 (다인용)
}
