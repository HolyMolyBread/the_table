# The Table — 배포 전 보안 점검 보고서 (Security Audit)

> **작성일**: 2026-02-21
> **대상 버전**: Phase 5.2 (Supabase 연동 완료, 모바일 UI 구축 완료)
> **점검 범위**: 백엔드 코어(`cmd/server/`), DB 스키마(`temp_sql/`), 클라이언트(`index.html`)
> **상태 범례**: 🔴 Critical — 즉시 수정 필요 &nbsp;|&nbsp; 🟡 Warning — 배포 전 개선 권장 &nbsp;|&nbsp; 🟢 Safe — 현재 안전

---

## 목차

1. [요약 대시보드](#1-요약-대시보드)
2. [🔴 Critical 취약점](#2--critical-취약점)
3. [🟡 Warning 취약점](#3--warning-취약점)
4. [🟢 안전 항목](#4--안전-항목)
5. [수정 로드맵](#5-수정-로드맵)
6. [참조 파일 인덱스](#6-참조-파일-인덱스)

---

## 1. 요약 대시보드

| 등급 | 항목 수 | 요약 |
|---|:---:|---|
| 🔴 Critical | 0 | ~~WebSocket CORS 전면 개방~~ **해결(Phase 6.2)** |
| 🟡 Warning  | 0 | ~~Rate Limiting 부재~~ **해결(Phase 6.2)**, ~~정적 파일 서빙 노출~~ **해결(Phase 6.2)** |
| 🟢 Safe     | 7 | CORS 화이트리스트, Rate Limiting, 정적파일 보호, 인증(Auth), RLS, XSS 방어, 동시성 방어 |

> **결론**: Phase 6.2에서 남은 모든 취약점이 해결되었습니다. 배포 가능한 상태입니다.

---

## 2. 🔴 Critical 취약점

---

### ~~C-01~~ ✅ S-06 · WebSocket CORS — Phase 6.2 해결 완료

| 항목 | 내용 |
|---|---|
| **파일** | `cmd/server/main.go` — `upgrader.CheckOrigin` |
| **공격 유형** | CSWSH (Cross-Site WebSocket Hijacking) |
| **위험도** | ~~Critical~~ → **🟢 해결 완료 (Phase 6.2)** |

**해결 방법 (Phase 6.2 적용)**

`ALLOWED_ORIGINS` 환경 변수(`.env`)로 허용할 Origin을 쉼표 구분으로 지정합니다.

```go
// cmd/server/main.go — buildAllowedOrigins()
var allowedOrigins = buildAllowedOrigins() // .env의 ALLOWED_ORIGINS 파싱

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        if origin == "" { return true } // curl 등 non-browser 허용
        return allowedOrigins[origin]   // 화이트리스트 외 차단
    },
}
```

배포 시 `.env`에 실제 도메인으로 설정: `ALLOWED_ORIGINS=https://your-app.com`

---

### ~~C-02~~ ✅ S-03 · 인증(Auth) — Phase 6.1 해결 완료

| 항목 | 내용 |
|---|---|
| **파일** | `index.html`, `cmd/server/manager.go`, `cmd/server/db.go` |
| **공격 유형** | 식별자 위조, 전적 어뷰징, DB Upsert 조작 |
| **위험도** | ~~Critical~~ → **🟢 해결 완료 (Phase 6.1)** |

**해결 방법 (Phase 6.1 적용)**

```
1. index.html: 닉네임 입력 폼 제거 → Supabase 이메일/비밀번호 로그인 폼으로 교체
2. WS 연결 후 즉시 {"action":"auth","payload":{"token":"<JWT>"}} 전송
3. manager.go: "auth" 액션 추가 → db.ValidateAuthToken() 호출
4. db.go: GET /auth/v1/user 호출로 JWT 검증 → UUID + email 반환
5. client.UserUUID = auth.uid(), client.UserID = email (위조 불가)
6. "join", "game_action" 액션에 client.UserUUID == "" 가드 추가
```

---

## 3. 🟡 Warning 취약점

---

### ~~W-01~~ ✅ S-05 · DB RLS — Phase 6.1 해결 완료

| 항목 | 내용 |
|---|---|
| **파일** | `temp_sql/001_initial_schema.sql` → `temp_sql/003_auth_rls.sql` 적용 |
| **공격 유형** | 직접 REST API 호출을 통한 전적 위조 |
| **위험도** | ~~Warning~~ → **🟢 해결 완료 (Phase 6.1)** |

**해결 방법 (Phase 6.1 적용)**

`temp_sql/003_auth_rls.sql`을 Supabase SQL Editor에서 실행하면 기존 전면 개방 정책이 삭제되고, `auth.uid() = user_id` 조건의 강화 정책으로 교체됩니다.

```sql
-- Phase 6.1 적용 정책
create policy "game_records_select_own" on game_records
  for select using (auth.uid() = user_id);

create policy "game_records_insert_own" on game_records
  for insert with check (auth.uid() = user_id);

create policy "game_records_update_own" on game_records
  for update using (auth.uid() = user_id) with check (auth.uid() = user_id);
```

인증된 본인의 `auth.uid()`와 일치하는 레코드만 접근 가능합니다.

---

### ~~W-02~~ ✅ S-07 · Rate Limiting — Phase 6.2 해결 완료

| 항목 | 내용 |
|---|---|
| **파일** | `cmd/server/client.go` — `readPump()` |
| **공격 유형** | 메시지 플러딩(Message Flooding), DoS |
| **위험도** | ~~Warning~~ → **🟢 해결 완료 (Phase 6.2)** |

**해결 방법 (Phase 6.2 적용)**

`golang.org/x/time/rate` 패키지의 토큰 버킷 방식으로 초당 10 메시지 제한을 적용했습니다.

```go
// Client 구조체: limiter 필드 추가
limiter: rate.NewLimiter(rate.Every(100*time.Millisecond), 10),

// readPump: 메시지 수신 직후 속도 체크
if !c.limiter.Allow() {
    c.SendJSON(ServerResponse{Type: "error", Message: "메시지 전송 속도가 너무 빠릅니다."})
    continue
}
```

---

### ~~W-03~~ ✅ S-08 · 정적 파일 보호 — Phase 6.2 해결 완료

| 항목 | 내용 |
|---|---|
| **파일** | `cmd/server/main.go` — `http.FileServer` 제거 |
| **공격 유형** | 소스 코드 및 `.env` 노출 |
| **위험도** | ~~Warning~~ → **🟢 해결 완료 (Phase 6.2)** |

**해결 방법 (Phase 6.2 적용)**

`http.FileServer(http.Dir("."))` 를 완전히 제거하고, `index.html` 단일 파일만 서빙하는 커스텀 핸들러로 교체했습니다.

```go
// main.go — indexHandler (index.html만 서빙)
func indexHandler(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "index.html")
}
http.HandleFunc("/", indexHandler)
```

Docker 이미지에도 `index.html`과 실행 바이너리만 포함되며, `.env`, `go.mod` 등은 `.dockerignore`로 제외됩니다.

---

## 4. 🟢 안전 항목

---

### S-01 · XSS(Cross-Site Scripting) 방어 적용

| 항목 | 내용 |
|---|---|
| **파일** | `index.html` — `escapeHTML()` 함수 |
| **상태** | ✅ 현재 안전 |

서버에서 수신한 모든 사용자 입력값(닉네임, 채팅 메시지, 시스템 공지 등)을
`innerHTML`에 삽입하기 전에 `escapeHTML()`을 통해 HTML 특수문자를 이스케이프합니다.

```javascript
// index.html
function escapeHTML(str) {
    // <, >, &, ", ' 등의 특수문자를 HTML 엔티티로 변환
}

// 사용 예시
el.innerHTML = `<div class="chat-bubble">${escapeHTML(content)}</div>`;
el.innerHTML = `<div class="system-msg">👋 ${escapeHTML(parsed.userId || '')}님이 입장했습니다</div>`;
```

`<script>alert(1)</script>` 같은 악성 페이로드는 텍스트로만 렌더링되며 실행되지 않습니다.

---

### S-02 · 동시성(Race Condition) 방어 적용

| 항목 | 내용 |
|---|---|
| **파일** | `cmd/server/client.go`, `manager.go`, `plugin_gomoku.go`, `plugin_blackjack.go` |
| **상태** | ✅ 현재 안전 |

Go의 동시성 환경에서 발생할 수 있는 데이터 경쟁(Race Condition) 및 메모리 충돌을
뮤텍스로 철저히 방어하고 있습니다.

| 위치 | 적용 방식 | 보호 대상 |
|---|---|---|
| `client.go` — `SafeSend()` | `sync.Mutex` (`sendMu`) | send 채널 동시 쓰기 방지 |
| `client.go` — `closeOnce()` | `sync.Once` | send 채널 이중 닫기(panic) 방지 |
| `manager.go` — `RoomManager` | `sync.RWMutex` | rooms 맵 동시 읽기/쓰기 방지 |
| `manager.go` — `Room` | `sync.RWMutex` | clients 맵 동시 읽기/쓰기 방지 |
| `plugin_gomoku.go` — `GomokuGame` | `sync.Mutex` | 게임 상태 동시 접근 방지 |
| `plugin_blackjack.go` — `BlackjackGame` | `sync.Mutex` | 딜러 AI 고루틴 충돌 방지 |

Go의 `-race` 플래그로 빌드/테스트 시 경쟁 조건이 감지되지 않습니다.

---

## 5. 수정 로드맵 (전체 완료)

| 우선순위 | ID | 항목 | 완료 Phase |
|:---:|---|---|---|
| 1 | ~~C-01~~ | ~~WebSocket Origin 화이트리스트 도입~~ | ✅ Phase 6.2 |
| 2 | ~~C-02~~ | ~~Supabase Auth JWT 검증 연동~~ | ✅ Phase 6.1 |
| 3 | ~~W-03~~ | ~~정적 파일 서버 제거 및 index.html 단일 서빙~~ | ✅ Phase 6.2 |
| 4 | ~~W-01~~ | ~~RLS 정책 — Auth UID 기반으로 강화~~ | ✅ Phase 6.1 |
| 5 | ~~W-02~~ | ~~readPump 토큰 버킷 Rate Limiter 도입~~ | ✅ Phase 6.2 |

> 모든 보안 취약점이 해결되었습니다. **배포 가능한 상태입니다.**

---

## 6. 참조 파일 인덱스

| 파일 | 관련 항목 |
|---|---|
| `cmd/server/main.go` | C-01 (CORS), W-03 (정적 파일 서버) |
| `cmd/server/client.go` | W-02 (Rate Limiting), S-01 (XSS 방어 기반), S-02 (동시성) |
| `cmd/server/manager.go` | C-02 (Auth 부재 — JoinRoom), S-02 (동시성) |
| `cmd/server/db.go` | C-02 (Auth 부재 — GetOrCreateProfile) |
| `temp_sql/001_initial_schema.sql` | W-01 (RLS 전면 개방) |
| `index.html` | C-02 (닉네임 무검증 입력), S-01 (escapeHTML) |

---

*이 문서는 The Table 프로젝트의 배포 전 체크리스트로 활용됩니다.*
*취약점이 수정될 때마다 해당 항목의 상태를 🔴/🟡 → 🟢 로 업데이트하고, 수정 커밋 해시를 기록하세요.*
