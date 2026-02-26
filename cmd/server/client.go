package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
)

// GameRecord는 특정 게임 또는 전체에 대한 승무패 전적을 저장합니다.
type GameRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Draws  int `json:"draws"`
}

// RecordUpdateResponse는 전적이 갱신될 때 해당 클라이언트에게 전송하는 개별 알림입니다.
type RecordUpdateResponse struct {
	Type    string                 `json:"type"`    // "record_update"
	Records map[string]*GameRecord `json:"records"` // "total", "omok", "blackjack" 등
}

// Client는 WebSocket 연결 하나(유저 한 명)를 표현합니다.
// IsBot이 true인 가상 클라이언트는 conn이 nil이며, SafeSend 시 BotProcess 콜백으로 메시지가 전달됩니다.
type Client struct {
	manager *RoomManager
	conn    *websocket.Conn
	send    chan []byte

	UserID   string
	UserUUID string // Supabase auth.users.id (auth 액션 이후 설정)
	Token    string // 유저 JWT (auth 액션 이후 설정, DB RLS 통과용)
	RoomID   string
	Records  map[string]*GameRecord // 게임별 + 총합 전적 ("total", "omok", "blackjack")

	IsBot      bool              // true면 가상 클라이언트(AI 봇)
	BotProcess func(msg []byte)  // 봇 전용: 수신 메시지 처리 콜백 (nil 가능)

	limiter *rate.Limiter // 메시지 속도 제한 (10msg/s, 버스트 10)

	mu      sync.Mutex // Records 갱신 시 경쟁 방지
	once    sync.Once  // send 채널을 단 한 번만 닫기 위한 가드
	sendMu  sync.Mutex // send 채널에 대한 쓰기 경쟁 방지
	closed  bool
}

func newClient(conn *websocket.Conn, manager *RoomManager) *Client {
	return &Client{
		conn:    conn,
		manager: manager,
		send:    make(chan []byte, 256),
		Records: map[string]*GameRecord{
			"total":         {},
			"omok":          {},
			"blackjack_pve": {},
			"blackjack":     {},
		},
		// 100ms마다 토큰 1개 보충, 최대 버스트 10개 (초당 최대 10 메시지)
		limiter: rate.NewLimiter(rate.Every(100*time.Millisecond), 10),
	}
}

// SafeSend는 send 채널이 닫혀있어도 패닉 없이 안전하게 메시지를 전송합니다.
// 봇(IsBot)인 경우 웹소켓 전송 없이 BotProcess로 비동기 전달합니다.
func (c *Client) SafeSend(data []byte) bool {
	if c.IsBot && c.BotProcess != nil {
		go c.BotProcess(data)
		return true
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		log.Printf("[SEND_DROP] User: %s, Room: %s - Buffer Full", c.UserID, c.RoomID)
		return false
	}
}

// SendJSON은 v를 JSON으로 직렬화하여 클라이언트에 전송합니다.
// 봇(IsBot)인 경우 BotProcess로 전달됩니다 (SafeSend 내부 처리).
func (c *Client) SendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[ERROR] JSON 직렬화 실패: %v", err)
		return
	}
	c.SafeSend(data)
}

// updateRecord는 GameRecord에 result를 반영합니다.
func updateRecord(r *GameRecord, result string) {
	switch result {
	case "win":
		r.Wins++
	case "lose":
		r.Losses++
	case "draw":
		r.Draws++
	}
}

// RecordResult는 플러그인이 결과를 보고할 때 코어에 위임하는 메서드입니다.
// game: "omok" | "blackjack" 등 게임 키 / result: "win" | "lose" | "draw"
// 게임별 전적과 "total" 전적을 동시에 갱신하고 record_update JSON을 해당 클라이언트에 전송합니다.
// 변동이 있으면 DB에도 비동기로 upsert합니다.
func (c *Client) RecordResult(gamePrefix, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if result != "win" && result != "lose" && result != "draw" {
		log.Printf("[WARN] RecordResult: 알 수 없는 result 값 [%s]", result)
		return
	}

	// total 갱신 (PVE 전적은 메인 랭킹에서 제외)
	if gamePrefix != "blackjack_pve" {
		if c.Records["total"] == nil {
			c.Records["total"] = &GameRecord{}
		}
		updateRecord(c.Records["total"], result)
	}
	// 개별 게임 갱신
	if c.Records[gamePrefix] == nil {
		c.Records[gamePrefix] = &GameRecord{}
	}
	updateRecord(c.Records[gamePrefix], result)

	total := c.Records["total"]
	c.SendJSON(RecordUpdateResponse{
		Type:    "record_update",
		Records: c.Records,
	})
	log.Printf("[RECORD] [%s] %s → %s (총 %dW/%dL/%dD)",
		c.UserID, gamePrefix, result, total.Wins, total.Losses, total.Draws)

	// DB 동기 upsert — UserUUID·Token이 설정된 경우에만 실행. 승패가 반드시 DB에 기록되도록 동기 호출.
	if db != nil && c.UserUUID != "" && c.Token != "" {
		uuid := c.UserUUID
		gameRec := *c.Records[gamePrefix]
		dbName, isPVE := gameDBName(gamePrefix)
		db.UpsertGameRecord(uuid, dbName, isPVE, gameRec.Wins, gameRec.Losses, gameRec.Draws, c.Token)
	}
}

// closeOnce는 send 채널을 정확히 한 번만 닫습니다.
// 봇(IsBot)은 send가 nil이므로 close를 건너뜁니다.
func (c *Client) closeOnce() {
	c.once.Do(func() {
		c.sendMu.Lock()
		c.closed = true
		if c.send != nil {
			close(c.send)
		}
		c.sendMu.Unlock()
	})
}

// writePump는 send 채널에서 메시지를 읽어 WebSocket으로 전송하는 전용 고루틴입니다.
// 고루틴 하나만 Write를 담당하여 gorilla/websocket의 동시 쓰기 제약을 지킵니다.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// 채널이 닫힌 경우 — 정상 종료 처리
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			log.Printf("[SEND] → [%s]: %s", c.UserID, string(msg))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("[ERROR] 쓰기 실패 [%s]: %v", c.UserID, err)
				return
			}
		case <-ticker.C:
			// Ping을 보내 연결이 살아있는지 확인합니다.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump는 WebSocket에서 메시지를 읽어 매니저에 전달하는 메인 루프입니다.
// 이 함수가 종료되면 클라이언트 정리(퇴장 처리 + 채널 닫기)가 실행됩니다.
func (c *Client) readPump() {
	defer func() {
		c.manager.RemoveClient(c)
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, rawMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ERROR] 클라이언트 [%s] 비정상 종료: %v", c.UserID, err)
			} else {
				log.Printf("[DISCONNECT] 클라이언트 종료: [%s]", c.UserID)
			}
			return
		}

		// Rate Limiting: 초당 메시지 수 초과 시 경고 후 드롭
		if !c.limiter.Allow() {
			c.SendJSON(ServerResponse{
				Type:    "error",
				Message: "메시지 전송 속도가 너무 빠릅니다.",
			})
			log.Printf("[RATELIMIT] [%s] 속도 제한 초과 — 메시지 드롭", c.UserID)
			continue
		}

		log.Printf("[RECV] ← [%s]: %s", c.UserID, string(rawMsg))
		c.manager.HandleMessage(c, rawMsg)
	}
}
