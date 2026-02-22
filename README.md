# The Table

**WebSocket 기반 멀티플레이어 보드게임 플랫폼**

블랙잭, 텍사스 홀덤, 마작, 오목 등 다양한 보드게임을 플러그인 형태로 실시간 플레이할 수 있는 범용 게임 엔진입니다.

---

## 핵심 기능

- **실시간 멀티플레이**: WebSocket 기반 저지연 통신
- **플러그인 아키텍처**: 게임별 독립 플러그인, OCP 원칙 준수
- **AI 봇 지원**: 오목, 4목, 틱택토, 포커류, 도둑잡기, 원카드, 마작 등
- **전적 시스템**: Supabase 연동 영구 저장 (선택)
- **Docker 배포**: 멀티스테이지 빌드로 경량 이미지

---

## 기술 스택

| 영역 | 기술 |
|------|------|
| 백엔드 | Go 1.24, gorilla/websocket |
| 실시간 통신 | WebSocket (JSON) |
| 데이터베이스 | Supabase (PostgreSQL) |
| 클라이언트 | HTML/JS (단일 페이지) |
| 배포 | Docker (golang:1.24-alpine → alpine:3.20) |

---

## 실행 방법

### 1. 사전 요구사항

- Go 1.24 이상
- (선택) Supabase 프로젝트 — 전적·인증 사용 시

### 2. 의존성 설치

```bash
go mod download
```

### 3. 환경 변수 설정

프로젝트 루트에 `.env` 파일을 생성합니다:

```env
# Supabase (전적·인증 — 없으면 인메모리 모드)
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your-anon-key

# CORS 허용 Origin (쉼표 구분, 없으면 localhost 기본)
ALLOWED_ORIGINS=http://localhost:3000,http://127.0.0.1:8080
```

> `.env`가 없어도 서버는 동작합니다. DB 기능만 비활성화됩니다.

### 4. 서버 실행

```bash
go run ./cmd/server/
```

기본 포트 **8080**에서 HTTP + WebSocket 서버가 시작됩니다.

### 5. 클라이언트 접속

브라우저에서 `http://localhost:8080` 접속 후 로그인/회원가입하여 게임을 플레이할 수 있습니다.

---

## Docker 실행

```bash
docker build -t the-table .
docker run -p 8080:8080 \
  -e SUPABASE_URL=... \
  -e SUPABASE_ANON_KEY=... \
  -e ALLOWED_ORIGINS=http://localhost:8080 \
  the-table
```

---

## 구현된 게임 목록

### 카드 게임
| 게임 | 인원 | 비고 |
|------|------|------|
| 블랙잭 | 1 vs 딜러 | PVE, 관전 모드 |
| 텍사스 홀덤 | 2~4인 | 별 서바이벌, 족보 판정 |
| 세븐 포커 | 2~4인 | 4장 초이스, 히든 카드 |
| 인디언 포커 | 1:1 | 하트 서바이벌 |
| 도둑잡기 | 2~4인 | 53장, 페어 제거 |
| 원카드 | 2~4인 | 탑 카드 매칭 |

### 보드/타일 게임
| 게임 | 인원 | 비고 |
|------|------|------|
| 오목 | 1:1 | 렌주룰, 15초 타이머 |
| 4목 (Connect 4) | 1:1 | 6×7, 중력 낙하 |
| 틱택토 | 1:1 | 3×3 |
| 마작 | 4인 | Phase 1: 패 분배·쯔모·타패 |

---

## 스크린샷

> 스크린샷을 추가할 경우 이 영역에 이미지를 배치하세요.
>
> ```markdown
> ![로비 화면](docs/screenshots/lobby.png)
> ![오목 게임](docs/screenshots/gomoku.png)
> ```

---

## 문서

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** — 아키텍처 설계, 플러그인 규격, 개발 이력
- **[SECURITY_AUDIT.md](./SECURITY_AUDIT.md)** — 보안 점검 보고서 (Phase 6.2 기준)

---

## 테스트

```bash
go test ./cmd/server/ -v
```

- `poker_utils_test.go` — 포커 족보 판정 (로얄 플러시, 풀하우스 등)
- `manager_test.go` — 방 생성, 입장/퇴장 로직

---

## 라이선스

[MIT License](./LICENSE)
