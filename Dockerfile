# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /build

# 의존성 레이어 캐싱 (소스 변경 시 재다운로드 방지)
COPY go.mod go.sum ./
RUN go mod download

# 소스 전체 복사 후 빌드
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# ── Stage 2: Run ──────────────────────────────────────────────────────────────
FROM alpine:3.20

WORKDIR /app

# 인증서 (HTTPS 외부 API 호출용)
RUN apk --no-cache add ca-certificates

# 실행 바이너리 + 클라이언트 파일만 복사
COPY --from=builder /build/server    ./server
COPY --from=builder /build/index.html ./index.html

# 환경 변수는 런타임에 주입 (SUPABASE_URL, SUPABASE_ANON_KEY, ALLOWED_ORIGINS)
# .env 파일은 이미지에 포함하지 않습니다.

EXPOSE 8080

CMD ["./server"]
