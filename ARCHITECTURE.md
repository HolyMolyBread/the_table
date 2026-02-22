# The Table — 아키텍처 설계 문서 (SSOT)

> **이 문서는 프로젝트의 단일 진실 공급원(Single Source of Truth)입니다.**
> AI(Cursor)는 코드를 작성하거나 리팩토링하기 전에 반드시 이 문서를 먼저 읽고,
> 작업 완료 후 변경 사항이 있으면 즉시 이 파일을 최신화해야 합니다.

---

## 목차

1. [프로젝트 개요](#1-프로젝트-개요)
2. [기술 스택](#2-기술-스택)
3. [디렉터리 구조](#3-디렉터리-구조)
4. [핵심 아키텍처 원칙](#4-핵심-아키텍처-원칙)
5. [코어 시스템 명세](#5-코어-시스템-명세)
6. [JSON 통신 프로토콜](#6-json-통신-프로토콜)
7. [GamePlugin 인터페이스 규격](#7-gameplugin-인터페이스-규격)
8. [플러그인 구현 체크리스트](#8-플러그인-구현-체크리스트)
9. [PVE AI Bot 설계 방향](#9-pve-ai-bot-설계-방향)
10. [구현 게임 로드맵](#10-구현-게임-로드맵)
11. [개발 이력 (Phase 로그)](#11-개발-이력-phase-로그)
12. [AI 컨텍스트 유지 정책](#12-ai-컨텍스트-유지-정책)

> ✅ **보안 점검 완료 (Phase 6.2)**: [`SECURITY_AUDIT.md`](./SECURITY_AUDIT.md) 참조.
> Critical 0건, Warning 0건 — 모든 보안 취약점이 해결되었습니다. **배포 가능 상태입니다.**

---

## 1. 프로젝트 개요

**The Table**은 블랙잭, 텍사스 홀덤, 마작 등 다양한 보드게임 룰을 플러그인 형태로 교체할 수 있는 **범용 멀티플레이어 보드게임 엔진 플랫폼**이다.

단일 엔진 위에서 수십 종의 게임을 실시간으로 운영하는 것이 최종 목표이며, 모든 설계 결정은 이 목표로부터 역산된다.

---

## 2. 기술 스택

| 영역 | 기술 | 비고 |
|---|---|---|
| **백엔드** | Go (Golang) 1.22 | 고루틴 기반 고성능 동시성 |
| **실시간 통신** | gorilla/websocket v1.5.3 | JSON 텍스트 프레임 |
| **인프라** (예정) | AWS EC2 / Oracle Cloud Linux | Docker 컨테이너화 예정 |
| **데이터베이스** (예정) | Supabase (PostgreSQL) | 영구 유저 데이터 |
| **캐시/세션** (예정) | Upstash (Redis) | 서버 간 상태 공유 |
| **클라이언트** | 브라우저 HTML/JS | 테스트 콘솔 (`index.html`) |

---

## 3. 디렉터리 구조

```
the-table/
├── cmd/
│   └── server/
│       ├── main.go              # 서버 진입점 — HTTP 라우팅, RoomManager 초기화
│       ├── client.go            # Client 구조체 — 연결, 전적(Records), readPump/writePump
│       ├── game.go              # GamePlugin 인터페이스 + 공유 응답 타입 (SSOT)
│       ├── db.go                # DBClient — Supabase REST API (net/http 경량 구현)
│       ├── manager.go           # RoomManager, Room — 코어 룸/세션 관리 + Plugin Factory
│       ├── bot.go               # SpawnBot — Dummy Client 기반 AI 봇 (오목/4목/틱택토)
│       ├── poker_utils.go       # Card, NewShuffledDeck, EvaluateHand, HandRankName — 포커 공통 유틸
│       ├── plugin_gomoku.go     # [플러그인] 1:1 PVP 오목 (접두사: omok) — 리매치 지원
│       ├── plugin_blackjack.go  # [플러그인] PVE 블랙잭 (접두사: blackjack) — 관전 지원
│       ├── plugin_tictactoe.go # [플러그인] 1:1 PVP 틱택토 (접두사: tictactoe)
│       ├── plugin_connect4.go  # [플러그인] 1:1 PVP 4목 — 중력 낙하, 6×7 보드 (접두사: connect4)
│       └── plugin_indian.go    # [플러그인] 1:1 PVP 인디언 포커 — 하트 서바이벌, 개별 상태 전송 (접두사: indian)
├── index.html               # 브라우저 기반 클라이언트 (로비 + 인게임 UI)
├── .env                     # 환경 변수 (SUPABASE_URL, SUPABASE_ANON_KEY, ALLOWED_ORIGINS) — Git 미포함
├── .gitignore               # .env, temp_sql/, 빌드 산출물 제외
├── .dockerignore            # .env, temp_sql/, SECURITY_AUDIT.md 등 이미지 제외 목록
├── Dockerfile               # 멀티스테이지 빌드 (golang:1.24-alpine → alpine:3.20)
├── temp_sql/                # DB 스키마 SQL 파일 모음 — Git 미포함
│   ├── 001_initial_schema.sql   # profiles, game_records 테이블 + RLS + 뷰
│   ├── 002_fix_schema.sql       # updated_at 컬럼 안전 추가 + 뷰/트리거 재생성
│   ├── 003_auth_rls.sql         # Phase 6.1: auth.uid() 기반 RLS 강화
│   └── 004_profiles.sql         # Phase 7.4: profiles auth.users 연동, 닉네임(username) UNIQUE, RLS 정책
├── go.mod
├── go.sum
├── README.md
├── SECURITY_AUDIT.md        # 배포 전 보안 점검 보고서 (Phase 6.2 기준 전체 해결 완료)
└── ARCHITECTURE.md          # ← 이 파일
```

> **파일 추가 규칙**: 새 게임 플러그인은 반드시 `plugin_<게임명>.go` 형식으로 추가한다.
> 코어 파일(`main.go`, `client.go`, `game.go`, `manager.go`)은 플러그인 추가 시 수정하지 않는 것을 원칙으로 한다.

---

## 4. 핵심 아키텍처 원칙

### 4.1 DB-Core 분리 원칙 (Phase 5~)

Supabase DB 연동은 **코어의 동작을 블로킹하지 않는다**. 모든 DB I/O는 고루틴(비동기)으로 실행되며, DB가 없어도(`db == nil`) 서버는 인메모리 모드로 정상 동작한다.

```
인증(auth 액션)                    RecordResult 호출
    │                                    │
    ├─[즉시] ValidateAuthToken 호출       ├─[즉시] 인메모리 Records 갱신 + record_update 전송
    ├─[즉시] client.UserID, UserUUID 설정 └─[go] UpsertGameRecord (DB, 비동기)
    ├─[즉시] auth_ok 응답 전송
    └─[go] LoadUserRecords
           → record_update 전송 (전적 복구)

join 액션 (auth 이후에만 허용)
    └─[즉시] 코어 입장 브로드캐스트 → Plugin.OnJoin()
```

### 4.1.1 인증(Auth) 흐름 (Phase 6.1~)

```
클라이언트                          서버 (manager.go)           Supabase
    │                                   │                        │
    │─── WS 연결 ───────────────────────►│                        │
    │◄── 연결 수립 ──────────────────────│                        │
    │                                   │                        │
    │─── {action:"auth",                │                        │
    │     payload:{token:"JWT"}} ───────►│                        │
    │                                   │─── GET /auth/v1/user ──►│
    │                                   │◄── {id, email} ─────────│
    │                                   │ client.UserUUID = id     │
    │                                   │ client.UserID  = email   │
    │◄── {type:"auth_ok"} ──────────────│                        │
    │◄── {type:"record_update"} ────────│ (비동기: LoadUserRecords) │
    │                                   │                        │
    │─── {action:"join", ...} ──────────►│ (UserUUID 검증 통과)     │
    │◄── {type:"join"} ─────────────────│                        │
```

**가드 조건**: `client.UserUUID == ""` 상태에서 `join` 또는 `game_action`을 시도하면 에러를 반환하고 요청을 차단한다.

### 4.2 Record-Game 분리 원칙

코어(Core)와 게임(Plugin)의 책임은 엄격하게 분리된다.

```
┌─────────────────────────────────────────────────────┐
│                    코어 시스템 (Core)                  │
│                                                     │
│  · WebSocket 연결 수립 및 유지          (client.go)  │
│  · 방(Room) 생성 / 입장 / 퇴장          (manager.go) │
│  · 유저 전적(Records) 중앙 통제          (client.go)  │
│  · join / chat / leave / game_action  (manager.go) │
│                                                     │
│            ↕  GamePlugin 인터페이스만 노출            │
│                                                     │
│  ┌───────────────────────────────────────────────┐  │
│  │               게임 플러그인 (Plugin)             │  │
│  │                                               │  │
│  │  · 게임 규칙 및 상태 관리                        │  │
│  │  · 승패 판정                                   │  │
│  │  · 전적 변동 → RecordResult() 위임              │  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### 4.2 전적 시스템 위임 원칙

> **플러그인은 `client.Records`를 절대 직접 수정해서는 안 된다.**

승패가 결정되면 반드시 코어의 `RecordResult(game, result string)` 메서드를 호출한다.
이 메서드는 게임별 전적 + `"total"` 전적을 동시에 갱신하고, `record_update` JSON을 해당 클라이언트에 개별 전송한다.

```go
// ✅ 올바른 방법 — 코어에 위임
client.RecordResult("omok", "win")   // win | lose | draw
client.RecordResult("blackjack", "lose")

// ❌ 금지 — 플러그인이 직접 수정
client.Records["omok"].Wins++
```

### 4.3 Plugin Factory 패턴

코어(`manager.go`)는 구체적인 플러그인 타입을 알지 못한다.
`newRoom()` 함수에서 **방 ID 접두사(prefix)** 에 따라 적합한 플러그인을 자동으로 주입한다.

```go
// manager.go — newRoom() 내부 (Plugin Factory)
switch {
case strings.HasPrefix(id, "omok"):  room.Plugin = NewGomokuGame(room)
case strings.HasPrefix(id, "dice"):  room.Plugin = NewDiceGame(room)
default:                              // nil — 채팅방
}
```

**방 ID 접두사 규칙표**

| 접두사 | 게임 | 플러그인 |
|---|---|---|
| `omok` | 1:1 PVP 오목 (리매치 지원) | `NewGomokuGame()` |
| `blackjack` | 블랙잭 PVE (관전 지원) | `NewBlackjackGame()` |
| `tictactoe` | 1:1 PVP 틱택토 | `NewTicTacToeGame()` |
| `connect4` | 1:1 PVP 4목 (중력 낙하) | `NewConnect4Game()` |
| `indian` | 1:1 PVP 인디언 포커 (하트 서바이벌) | `NewIndianGame()` |
| 그 외 | — (채팅방) | `nil` |

새 게임 추가 시 이 switch 문에 case를 1개 추가하는 것이 전부다.

### 4.4 동시성 안전 원칙

| 대상 | 보호 수단 |
|---|---|
| `RoomManager.rooms` 맵 | `sync.RWMutex` (manager.go) |
| `Room.clients` 맵 | `sync.RWMutex` (manager.go) |
| `Client.send` 채널 | `sync.Mutex` + `sync.Once` (client.go) |
| `DiceGame.pendingRolls` 맵 | `sync.Mutex` (plugin_dice.go) |
| `GomokuGame` 내부 상태 전체 | `sync.Mutex` (plugin_gomoku.go) |

WebSocket 쓰기는 `writePump` 고루틴이 단독으로 담당한다 (gorilla/websocket 동시 쓰기 제약 준수).

---

## 5. 코어 시스템 명세

### 5.1 `Client` 구조체 (`client.go`)

```go
type Client struct {
    manager *RoomManager
    conn    *websocket.Conn
    send    chan []byte    // 버퍼 256

    UserID   string
    UserUUID string                 // Supabase profiles.id (DB 연동 시 비동기 설정)
    RoomID   string
    Records  map[string]*GameRecord // "total", "omok", "blackjack" 등

    once   sync.Once     // send 채널 단일 닫기
    sendMu sync.Mutex    // send 채널 쓰기 보호
    closed bool
}

type GameRecord struct {
    Wins   int `json:"wins"`
    Losses int `json:"losses"`
    Draws  int `json:"draws"`
}
```

**주요 메서드**

| 메서드 | 설명 |
|---|---|
| `SafeSend(data []byte) bool` | 채널이 닫혀도 패닉 없이 안전 전송 |
| `SendJSON(v any)` | 구조체를 JSON으로 직렬화 후 전송 |
| `RecordResult(game, result string)` | 전적 갱신 + record_update 개별 전송 (플러그인 위임 메서드) |
| `writePump()` | 전송 전담 고루틴 (Ping 포함) |
| `readPump()` | 수신 루프 — 종료 시 RemoveClient 자동 호출 |

### 5.2 `DBClient` 구조체 (`db.go`)

Go 기본 `net/http`로 Supabase PostgREST API를 호출하는 경량 클라이언트. 무거운 서드파티 SDK를 사용하지 않는다.

| 메서드 | 설명 |
|---|---|
| `GetOrCreateProfile(username)` | profiles 테이블 조회/생성 → UUID 반환 |
| `LoadUserRecords(userUUID)` | game_records 조회 → `map[string]*GameRecord` 반환 |
| `UpsertGameRecord(uuid, gameName, isPVE, w, l, d)` | 전적 upsert (merge-duplicates) |

**DB 게임 이름 매핑 규칙**

| 인메모리 키 | DB game_name | is_pve |
|---|---|---|
| `omok` | `omok_pvp` | false |
| `blackjack` | `blackjack_pve` | true |

**필요한 Supabase 테이블 스키마**

```sql
-- profiles: 유저 프로필 (username → UUID)
create table profiles (
  id uuid primary key default gen_random_uuid(),
  username text unique not null,
  created_at timestamptz default now()
);

-- game_records: 게임별 전적 (유저당 게임당 1행)
create table game_records (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references profiles(id) on delete cascade,
  game_name text not null,      -- "omok_pvp", "blackjack_pve" 등
  is_pve boolean default false,
  wins int default 0,
  losses int default 0,
  draws int default 0,
  unique (user_id, game_name)   -- upsert merge-duplicates 동작의 전제 조건
);
```

### 5.3 `Room` 구조체 (`manager.go`)

```go
type Room struct {
    ID      string
    clients map[*Client]bool
    mu      sync.RWMutex
    Plugin  GamePlugin    // 주입된 게임 플러그인
}
```

### 5.4 `RoomManager` 구조체 (`manager.go`)

| 메서드 | 설명 |
|---|---|
| `JoinRoom(roomID, userID, client)` | 입장 처리 → Plugin.OnJoin() 호출 |
| `leaveRoom(client)` | 퇴장 처리 → Plugin.OnLeave() 호출 → 빈 방 삭제 |
| `RemoveClient(client)` | leaveRoom + closeOnce (readPump defer) |
| `BroadcastToRoom(roomID, msg, exclude)` | 특정 방 브로드캐스트 |
| `HandleMessage(client, rawMsg)` | action 분기 라우터 |

---

## 6. JSON 통신 프로토콜

### 6.1 클라이언트 → 서버 (ClientRequest)

```json
{
  "action": "<action명>",
  "payload": { }
}
```

| action | payload 구조 | 처리 주체 |
|---|---|---|
| `join` | `{"roomId": "room_1", "userId": "player_A"}` | 코어 |
| `chat` | `{"message": "안녕하세요"}` | 코어 |
| `leave` | `{}` | 코어 |
| `game_action` | `{"cmd": "<명령>", ...}` | 플러그인으로 위임 |
| `game_action` (오목) | `{"cmd": "place", "x": 0, "y": 0}` | GomokuGame |

### 6.2 서버 → 클라이언트 (ServerResponse 계열)

**공통 응답 (`ServerResponse`)**

```json
{
  "type": "<타입>",
  "message": "...",
  "userId": "...",
  "roomId": "..."
}
```

| type | 설명 | 대상 |
|---|---|---|
| `system` | 서버 시스템 알림 | 개별 |
| `join` | 유저 입장 알림 | 방 전체 |
| `leave` | 유저 퇴장 알림 | 방 전체 |
| `chat` | 채팅 메시지 | 방 전체 |
| `error` | 오류 응답 | 개별 |
| `game_notice` | 게임 진행 공지 (플러그인 발행) | 방 전체 |
| `game_result` | 게임 최종 판정 결과 (플러그인 발행) | 방 전체 |
| `board_update` | 오목 보드 상태 갱신 (GomokuGame 발행) | 방 전체 |

**전적 갱신 응답 (`RecordUpdateResponse`) — 개별 전송 전용**

```json
{
  "type": "record_update",
  "records": {
    "total":     { "wins": 5, "losses": 2, "draws": 1 },
    "omok":      { "wins": 3, "losses": 1, "draws": 0 },
    "blackjack": { "wins": 2, "losses": 1, "draws": 1 }
  }
}
```

**게임 결과 응답 (`GameResultResponse`) — 방 전체 브로드캐스트**

```json
{
  "type": "game_result",
  "message": "🏆 [player_A]의 5목 달성! 승리!",
  "roomId": "omok_ABC123",
  "rematchEnabled": true
}
```

---

## 7. GamePlugin 인터페이스 규격

> 이 인터페이스는 `game.go`에 정의되어 있으며, 플러그인 구현의 계약(Contract)이다.
> 인터페이스를 변경할 경우 모든 기존 플러그인 파일을 동시에 업데이트해야 한다.

```go
type GamePlugin interface {
    // 게임의 식별 이름 반환
    Name() string

    // 플레이어 입장 직후 훅 — 게임 현황 안내, 대기 상태 업데이트 등
    // 코어가 join 브로드캐스트를 완료한 뒤 호출됨
    OnJoin(client *Client)

    // 플레이어 퇴장 직전 훅 — 게임 초기화, 진행 중 라운드 처리 등
    // 클라이언트가 room.clients에서 제거된 뒤, client.RoomID가 클리어되기 전에 호출됨
    OnLeave(client *Client)

    // game_action 메시지를 코어로부터 위임받아 처리
    // payload: {"cmd": "roll"} 등 게임별 명령 구조
    HandleAction(client *Client, action string, payload json.RawMessage)
}
```

### 7.1 훅 호출 순서 (중요)

**입장 시:**
```
room.clients[client] = true
client.UserID, client.RoomID 설정
→ 코어: join 브로드캐스트
→ Plugin.OnJoin(client)          ← 플러그인 훅
```

**퇴장 시:**
```
delete(room.clients, client)
→ Plugin.OnLeave(client)          ← client.RoomID 아직 유효
→ client.RoomID = ""
→ 코어: leave 브로드캐스트
→ 빈 방 삭제 (해당 시)
```

---

## 8. 플러그인 구현 체크리스트

새 게임 플러그인(`plugin_<게임명>.go`)을 작성할 때 반드시 확인한다.

- [ ] `GamePlugin` 인터페이스 4개 메서드 전부 구현
- [ ] `Name()` — 유니크한 게임 이름 반환
- [ ] `OnJoin` — 인원 카운트 확인 후 게임 시작 가능 여부 안내
- [ ] `OnLeave` — 진행 중 게임 상태 정리, 필요 시 몰수패/초기화 처리
- [ ] `HandleAction` — `cmd` 필드 기반 내부 분기 처리
- [ ] 전적 변경 시 `client.Records` 직접 수정 금지 → `client.RecordResult()` 사용
- [ ] 내부 상태 맵/슬라이스에 `sync.Mutex` 적용 (동시성 안전)
- [ ] `newRoom()` 함수에서 플러그인 주입 코드 1줄 추가
- [ ] 이 ARCHITECTURE.md의 로드맵 섹션 업데이트

---

## 9. PVE AI Bot 설계 방향

> **현재 상태: 미구현 (설계 단계)**
> PVE 모드는 향후 블랙잭, 텍사스 홀덤 등 딜러가 필요한 카드 게임 구현 시 필수적이다.

### 9.1 설계 목표

플러그인이 "상대가 사람인지 AI인지 알 필요 없도록" 만드는 것이 핵심이다.
AI Bot도 `Client` 인터페이스를 동일하게 구현하여 플러그인과 투명하게 상호작용한다.

### 9.2 구현 예정 구조

```
AIBot (가상 클라이언트)
  └── 실제 WebSocket 연결 없이 send 채널만 보유
  └── RoomManager.JoinRoom()으로 방에 입장
  └── 별도 고루틴에서 게임 로직에 따라 HandleAction() 자동 호출
  └── RecordResult()로 전적 동기화 (실제 전송은 no-op 처리)
```

### 9.3 코어 변경 최소화 원칙

AI Bot 도입 시 `game.go`의 `GamePlugin` 인터페이스와 `manager.go`의 코어는 **변경하지 않는다**.
`Client` 구조체를 인터페이스로 추상화하거나, AIBot 전용 `NullSend` 구현을 추가하는 방식으로 처리한다.

---

## 10. 구현 게임 로드맵

### 🃏 카드 게임

| 게임 | 파일 (예정) | 상태 | 비고 |
|---|---|---|---|
| 블랙잭 | `plugin_blackjack.go` | **완료** | PVE 딜러 AI. 고정 참가비 100점. **관전 모드** 지원(2번째 이후 입장자 자동 관전). 접두사: `blackjack` |
| 텍사스 홀덤 | `plugin_holdem.go` | **완료** | 별(⭐)×10 서바이벌, 체크(⭐-1)/폴드, 족보 판정, 절반 파산 시 생존자 승리. 접두사: `holdem` |
| 세븐 포커 | `plugin_sevenpoker.go` | **완료** | 3~7구 분배, 히든 카드, 별 서바이벌. 접두사: `sevenpoker` |
| 인디언 포커 | `plugin_indian.go` | **완료** | 하트 서바이벌(❤️×10), 개별 상태 전송(뒷면/앞면), 선공/포기 -1, 승부/패 -2+2, 접두사: `indian` |
| 도둑잡기 | `plugin_thief.go` | 미구현 | |
| 원카드 | `plugin_onecard.go` | 미구현 | UNO 유사 |

### 🀄 타일/그리드 게임

| 게임 | 파일 (예정) | 상태 | 비고 |
|---|---|---|---|
| 마작 | `plugin_mahjong.go` | 미구현 | 4인, 복잡도 최고 |
| 오목 | `plugin_gomoku.go` | **완료** | 15×15, 렌주룰, 15초 타이머, **리매치(흑/백 교체)** 지원. 접두사: `omok` |
| 바둑 | `plugin_baduk.go` | 미구현 | 19×19, 규칙 복잡 |
| 4목 (Connect 4) | `plugin_connect4.go` | **완료** | 6×7, 중력 낙하, 가로·세로·대각선 4목 승리, 무승부 판정. 승무패 전적 기록. 접두사: `connect4` |
| 틱택토 | `plugin_tictactoe.go` | **완료** | 3×3, 가로·세로·대각선 3목 승리, 무승부 판정. 승무패 전적 기록. 접두사: `tictactoe` |

### 🎯 물리/액션 게임

| 게임 | 파일 (예정) | 상태 | 비고 |
|---|---|---|---|
| 알까기 | `plugin_marbles.go` | 미구현 | 물리 엔진 필요 여부 검토 |

### 📈 특수 시뮬레이터

| 게임 | 파일 (예정) | 상태 | 비고 |
|---|---|---|---|
| 트렌드 마켓 | `plugin_market.go` | 미구현 | 별도 규격 문서 필요 |

### 🌐 확장 목표

대중적인 보드게임 및 미니게임을 지속적으로 추가할 예정이다.
커뮤니티 요청 및 구현 난이도를 고려하여 우선순위를 결정한다.

---

## 11. 개발 이력 (Phase 로그)

| Phase | 내용 | 완료 시점 |
|---|---|---|
| **Phase 1.5** | JSON WebSocket 통신 코어 구축 (echo 서버) | 초기 |
| **Phase 2.0** | Room Manager + 브로드캐스팅 시스템 | — |
| **Phase 2.1** | UserID/RoomID 입력 UI + Join/Leave 버튼 | — |
| **Phase 2.2** | Score 필드 + 단순 roll_dice (비플러그인) | — |
| **Phase 2.3** | game_result / score_update UI 처리 | — |
| **Phase 3.0** | **GamePlugin 인터페이스 + DiceGame 플러그인 아키텍처** | 완료 |
| **Phase 3.1** | **Plugin Factory 패턴 + GomokuGame (오목) 플러그인 + 시각적 보드 UI** | 완료 |
| **Phase 3.2** | **UI 레이아웃 개선 + 렌주룰/15초 타이머 + 직전 수 마커** | 완료 |
| **Phase 3.3** | **렌주룰 패턴매칭 전면 리팩토링 + 리입장 버그 수정 + 토스트/모달 알림** | 완료 |
| **Phase 4**   | **블랙잭 PVE 플러그인 (딜러 AI + 고정 참가비 + 수동 재시작)** | 완료 |
| **Phase 4.1** | **클라이언트 UI 전면 개편 (로비, 인게임 채팅, 디버그 패널 분리)** | 완료 |
| **Phase 4.2** | **비밀방 코드(4자리) 매칭 + 꽉 찬 방 예외 처리** | 완료 |
| **Phase 4.3** | **주사위 폐기, 오목 리매치, 블랙잭 관전 모드, CSS 오버플로우 수정** | 완료 |
| **Phase 4.3.1** | **UI 폴리싱(6자리 코드·룰 버튼), 오목 관전 모드, 방 해산 로직** | 완료 |
| **Phase 4.4** | **점수제 폐기 → 승무패 전적(W/L/D) 시스템 전면 도입** | 완료 |
| **Phase 5** | **Supabase REST API 연동 — 유저 프로필 + 전적 영구 저장 (db.go)** | 완료 |
| **Phase 5.1** | **보안 강화 (.gitignore) + SQL 관리 자동화 (temp_sql/)** | 완료 |
| **Phase 5.2** | **모바일 반응형 UI, IME 입력 버그 수정, 블랙잭 룰 최신화, SQL 마이그레이션** | 완료 |
| **Phase 6.1** | **Supabase Auth 연동 및 인증/보안 시스템 구축** — JWT 로그인 UI, `ValidateAuthToken`, auth 액션 핸들러, join/game_action 인증 가드, RLS 강화(003_auth_rls.sql) | 완료 |
| **Phase 6.1.1** | **로그인 전후 UI 분리 및 UX 개선** — `#game-cards` 로그인 전 숨김, 로그인 성공 시 표시, Enter 키 로그인 지원(`onAuthKeyDown`) | 완료 |
| **Phase 7.0** | **틱택토 PVP 플러그인 추가** — 3×3 보드, 승무패 판정, 전적 기록, 관전 모드 지원 | 완료 |
| **Phase 7.1** | **4목(Connect 4) PVP 플러그인 추가** — 6×7 보드, 중력 낙하 로직, 4목 승패 판정, 전적 기록, 파란 보드 UI | 완료 |
| **Phase 7.2** | **인디언 포커 PVP 플러그인 추가** — 하트 서바이벌(❤️×10), 개별 상태 전송(뒷면/앞면 분리), 선후공 교체, 포기/승부 하트 증감, 전적 기록 | 완료 |
| **Phase 7.3** | **게임 UI 폴리싱 및 타이머 적용** — 채팅창 토글, 오목 텍스트 통일, 전적 모달 신규 게임 추가, 인디언포커 결과 오버레이, 틱택토/4목 15초 타이머, 인디언포커 30초 타이머(시간초과 시 포기) | 완료 |
| **Phase 7.4** | **닉네임 및 상대 전적 시스템** — Supabase profiles 연동 닉네임 중복검사/변경, 인게임 상대 닉네임 클릭 시 전적 열람 기능 추가 | 완료 |
| **Phase 7.5** | **포커류 디테일 강화** — 15초 자동 폴드 타이머, 쇼다운 결과/족보 오버레이 UI, 로비 카테고리 정렬, 아이콘 개편 | 완료 |
| **Phase 7.6** | **다인용 공통 리매치 시스템** — 홀덤/세븐포커 리매치 로직 추가, N명 동적 레디 카운트(ready/total) UI 적용 및 별 초기화 로직 구현 | 완료 |
| **Phase 4.8** | **범용 AI(PVE) 프레임워크 (수동 소환)** — Dummy Client 기반 봇 아키텍처 구축(bot.go), 인게임 봇 추가 버튼 구현, 틱택토/4목/오목 랜덤 착수 봇 적용, 인디언 포커/텍사스 홀덤/세븐 포커 봇 로직 추가 | 완료 |
| **Phase 7.7** | **전역 코드 리팩토링** — 포커 공통 로직(poker_utils.go) 분리, 프론트/백엔드 중복 코드 제거 및 최적화 | 완료 |
| **Phase 4.x** | AI Bot (PVE) 범용 구조 설계 | 예정 |
| **Phase 6.2** | **최종 보안 패치 + Docker 배포 준비** — 로그인/회원가입 UI 분리, CORS 화이트리스트(`ALLOWED_ORIGINS`), Rate Limiting(`golang.org/x/time/rate`), 정적파일 단일 서빙(`indexHandler`), `Dockerfile` 멀티스테이지 빌드, `.dockerignore` | 완료 |

---

## 12. AI 컨텍스트 유지 정책

> **이 섹션은 Cursor AI(너 자신)를 위한 지침이다.**

### 작업 전 필수 확인 순서

1. `ARCHITECTURE.md` (이 파일) — 전체 원칙 및 현재 상태 파악
2. `cmd/server/game.go` — `GamePlugin` 인터페이스 현행 규격 확인
3. 변경 대상 파일 — 기존 코드 숙지

### 작업 후 필수 업데이트

- 새 플러그인 추가 → 로드맵 표에서 상태 `미구현` → `완료` 로 변경
- 코어 구조 변경 → 섹션 5, 7 업데이트
- 프로토콜 변경 → 섹션 6 업데이트
- Phase 완료 → 섹션 11 로그 추가

### 절대 위반 금지 규칙

| 번호 | 규칙 |
|---|---|
| R-01 | 플러그인에서 `client.Records` 직접 수정 금지 — 반드시 `RecordResult()` 사용 |
| R-02 | `manager.go`에 게임 로직(규칙, 판정) 추가 금지 |
| R-03 | `game.go`의 `GamePlugin` 인터페이스 변경 시 모든 플러그인 동시 업데이트 필수 |
| R-04 | 새 게임 추가 시 반드시 `plugin_<게임명>.go` 파일 이름 규칙 준수 |
| R-05 | 이 ARCHITECTURE.md를 삭제하거나 내용을 임의로 축소하지 말 것 |
