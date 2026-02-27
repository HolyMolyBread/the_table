package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// allowedOrigins는 ALLOWED_ORIGINS 환경 변수에서 읽어온 허용 도메인 집합입니다.
var allowedOrigins map[string]bool

// buildAllowedOrigins는 환경 변수 ALLOWED_ORIGINS를 파싱하여 맵을 구성합니다.
// 값이 없으면 localhost 개발 주소를 기본으로 허용합니다.
func buildAllowedOrigins() map[string]bool {
	raw := os.Getenv("ALLOWED_ORIGINS")
	m := make(map[string]bool)
	if raw != "" {
		for _, o := range strings.Split(raw, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				m[o] = true
			}
		}
		log.Printf("[CORS] 허용 Origin 목록: %v", raw)
	} else {
		// 개발 기본값: localhost 계열 허용
		defaults := []string{
			"http://localhost:8080",
			"http://127.0.0.1:8080",
			"http://localhost:3000",
		}
		for _, o := range defaults {
			m[o] = true
		}
		log.Println("[CORS] ALLOWED_ORIGINS 미설정 — 개발용 localhost 기본 허용")
	}
	return m
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// Origin이 없으면 (curl, 서버-서버 요청 등) 허용
		if origin == "" {
			return true
		}
		if allowedOrigins[origin] {
			return true
		}
		log.Printf("[CORS] 차단된 Origin: %s", origin)
		return false
	},
}

// manager는 서버 전체에서 단 하나의 RoomManager 인스턴스를 공유합니다.
var manager = NewRoomManager()

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ERROR] WebSocket 업그레이드 실패: %v", err)
		return
	}

	client := newClient(conn, manager)

	// 접속 즉시 환영 메시지를 send 버퍼에 넣습니다.
	client.SendJSON(ServerResponse{
		Type:    "system",
		Message: "Connected to The Table",
	})

	go client.writePump()
	client.readPump()
}

// indexHandler는 index.html 하나만 서빙합니다.
// 기존의 http.FileServer(http.Dir("."))는 .env, go.mod 등 모든 파일을 노출하므로 사용하지 않습니다.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func main() {
	// .env 파일 로드 (파일이 없어도 무시 — 프로덕션 환경 변수로 대체 가능)
	if err := godotenv.Load(); err != nil {
		log.Println("[ENV] .env 파일 없음 — 환경 변수만 사용합니다")
	}

	// CORS 허용 Origin 목록 초기화
	allowedOrigins = buildAllowedOrigins()

	// Supabase DB 클라이언트 초기화
	db = newDBClient()

	http.HandleFunc("/ws", handleWebSocket)
	// 정적 파일 서빙 (JS, CSS 폴더)
	http.Handle("/js/", http.StripPrefix("/js/", http.FileServer(http.Dir("js"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("css"))))
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))
	http.HandleFunc("/", indexHandler) // index.html 단일 서빙 (디렉터리 노출 없음)

	addr := ":8080"
	log.Printf("========================================")
	log.Printf("  The Table — Phase 6.2 배포 준비 서버")
	log.Printf("  WebSocket: ws://localhost%s/ws", addr)
	log.Printf("  클라이언트: http://localhost%s/", addr)
	if db != nil {
		log.Printf("  DB: Supabase 연결됨")
	} else {
		log.Printf("  DB: 비활성화 (인메모리 전용)")
	}
	log.Printf("========================================")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[FATAL] 서버 시작 실패: %v", err)
	}
}
