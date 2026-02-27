package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// ── DBClient ─────────────────────────────────────────────────────────────────

// DBClient는 Supabase PostgREST REST API와 Auth API를 Go 기본 net/http로 호출하는 클라이언트입니다.
// 무거운 서드파티 SDK 없이 경량으로 동작합니다.
type DBClient struct {
	baseURL    string // Supabase URL + "/rest/v1"
	authURL    string // Supabase URL + "/auth/v1"
	apiKey     string // anon key
	httpClient *http.Client
	mu         sync.Mutex // 쓰기 작업 직렬화 (동시 UpsertGameRecord 방지)
}

// db는 서버 전체에서 공유하는 싱글턴 DB 클라이언트입니다.
// SUPABASE_URL 또는 SUPABASE_ANON_KEY가 없으면 nil이며, 모든 DB 호출은 nil 체크 후 실행됩니다.
var db *DBClient

// newDBClient는 환경 변수를 읽어 DBClient를 초기화합니다.
// 환경 변수가 없으면 nil을 반환하며, DB 기능은 비활성화됩니다.
func newDBClient() *DBClient {
	supabaseURL := os.Getenv("SUPABASE_URL")
	apiKey := os.Getenv("SUPABASE_ANON_KEY")
	if supabaseURL == "" || apiKey == "" {
		log.Println("[DB] 경고: SUPABASE_URL 또는 SUPABASE_ANON_KEY 미설정 — DB 기능 비활성화")
		return nil
	}
	log.Printf("[DB] Supabase 연결 초기화: %s", supabaseURL)
	return &DBClient{
		baseURL:    supabaseURL + "/rest/v1",
		authURL:    supabaseURL + "/auth/v1",
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// ── 내부 HTTP 헬퍼 ────────────────────────────────────────────────────────────

// do는 Supabase REST API에 HTTP 요청을 보내고 응답을 반환합니다.
// extraHeaders로 Prefer 등 요청별 추가 헤더를 지정할 수 있습니다.
func (d *DBClient) do(method, path string, body any, extraHeaders map[string]string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, d.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", d.apiKey)
	req.Header.Set("Authorization", "Bearer "+d.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation") // 기본: INSERT/UPDATE 결과 반환
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return d.httpClient.Do(req)
}

// ── DB 행 타입 ────────────────────────────────────────────────────────────────

type profileRow struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type gameRecordRow struct {
	UserID   string `json:"user_id"`
	GameName string `json:"game_name"` // "omok_pvp", "blackjack_pve" 등
	IsPVE    bool   `json:"is_pve"`
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Draws    int    `json:"draws"`
}

// ── 이름 매핑 헬퍼 ────────────────────────────────────────────────────────────

// gameDBName은 인메모리 게임 키를 DB의 game_name 및 is_pve 플래그로 변환합니다.
// 일반(PVE): blackjack / 보스 레이드: blackjack_raid
func gameDBName(game string) (dbName string, isPVE bool) {
	switch game {
	case "blackjack":
		return "blackjack", true
	case "blackjack_raid":
		return "blackjack_raid", false
	case "omok":
		return "omok_pvp", false
	default:
		return game, false
	}
}

// gameMemKey는 DB의 game_name을 인메모리 Records 맵 키로 역변환합니다.
func gameMemKey(dbName string) string {
	switch dbName {
	case "blackjack":
		return "blackjack"
	case "blackjack_raid":
		return "blackjack_raid"
	case "omok_pvp":
		return "omok"
	default:
		return dbName
	}
}

// GetProfileByUUID는 profiles 테이블에서 id(UUID)로 조회하여 username(닉네임)을 반환합니다.
// 없으면 빈 문자열을 반환합니다.
func (d *DBClient) GetProfileByUUID(uuid string) string {
	path := "/profiles?select=username&id=eq." + url.QueryEscape(uuid) + "&limit=1"
	resp, err := d.do("GET", path, nil, nil)
	if err != nil {
		log.Printf("[DB] GetProfileByUUID 실패 [%s]: %v", uuid[:min(8, len(uuid))], err)
		return ""
	}
	defer resp.Body.Close()

	var rows []profileRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil || len(rows) == 0 {
		return ""
	}
	return rows[0].Username
}

// CheckNicknameUnique는 profiles 테이블에 해당 username이 존재하는지 확인합니다.
// excludeUUID가 비어있지 않으면 해당 id를 가진 행은 제외합니다 (본인 닉네임 유지 시).
// 반환: true = 사용 가능(중복 없음), false = 이미 사용 중
func (d *DBClient) CheckNicknameUnique(nickname string, excludeUUID string) bool {
	path := "/profiles?select=id&username=eq." + url.QueryEscape(nickname) + "&limit=1"
	if excludeUUID != "" {
		path += "&id=neq." + url.QueryEscape(excludeUUID)
	}
	resp, err := d.do("GET", path, nil, nil)
	if err != nil {
		log.Printf("[DB] CheckNicknameUnique 실패: %v", err)
		return false // 오류 시 사용 불가로 처리
	}
	defer resp.Body.Close()

	var rows []profileRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return false
	}
	return len(rows) == 0
}

// UpsertProfile은 profiles 테이블에 id(UUID)와 username(닉네임)을 삽입하거나 업데이트합니다.
func (d *DBClient) UpsertProfile(uuid, nickname string) error {
	body := map[string]string{"id": uuid, "username": nickname}
	resp, err := d.do("POST", "/profiles", body, map[string]string{
		"Prefer": "resolution=merge-duplicates,return=minimal",
	})
	if err != nil {
		return fmt.Errorf("profiles upsert 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 204 {
		return fmt.Errorf("profiles upsert 실패 (status %d)", resp.StatusCode)
	}
	log.Printf("[DB] UpsertProfile 완료 [%s] → %s", uuid[:min(8, len(uuid))], nickname)
	return nil
}

// ── 공개 메서드 ───────────────────────────────────────────────────────────────

// ValidateAuthToken은 Supabase Auth의 GET /auth/v1/user API를 호출하여
// JWT 토큰을 검증하고, 유효하면 유저의 UUID(id)와 email을 반환합니다.
// 토큰이 유효하지 않거나 만료되었으면 에러를 반환합니다.
func (d *DBClient) ValidateAuthToken(token string) (uuid string, email string, err error) {
	req, err := http.NewRequest("GET", d.authURL+"/user", nil)
	if err != nil {
		return "", "", fmt.Errorf("요청 생성 실패: %w", err)
	}
	req.Header.Set("apikey", d.apiKey)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("Auth API 호출 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("토큰 검증 실패 (status %d)", resp.StatusCode)
	}

	var result struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("Auth 응답 파싱 실패: %w", err)
	}
	if result.ID == "" {
		return "", "", fmt.Errorf("Auth 응답에 id 필드 없음")
	}
	log.Printf("[AUTH] 토큰 검증 성공: %s (%s)", result.Email, result.ID[:8])
	return result.ID, result.Email, nil
}

// GetOrCreateProfile은 profiles 테이블에서 username으로 유저를 조회하고 UUID를 반환합니다.
// 없으면 새 행을 INSERT한 뒤 UUID를 반환합니다.
func (d *DBClient) GetOrCreateProfile(username string) (string, error) {
	// 1) SELECT
	path := "/profiles?select=id&username=eq." + url.QueryEscape(username) + "&limit=1"
	resp, err := d.do("GET", path, nil, nil)
	if err != nil {
		return "", fmt.Errorf("profiles 조회 실패: %w", err)
	}
	defer resp.Body.Close()

	var rows []profileRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return "", fmt.Errorf("profiles 응답 파싱 실패: %w", err)
	}
	if len(rows) > 0 {
		log.Printf("[DB] 프로필 조회 성공: %s → %s", username, rows[0].ID)
		return rows[0].ID, nil
	}

	// 2) INSERT (처음 접속하는 유저)
	insertResp, err := d.do("POST", "/profiles", map[string]string{"username": username}, nil)
	if err != nil {
		return "", fmt.Errorf("profiles 생성 실패: %w", err)
	}
	defer insertResp.Body.Close()

	var newRows []profileRow
	if err := json.NewDecoder(insertResp.Body).Decode(&newRows); err != nil || len(newRows) == 0 {
		return "", fmt.Errorf("profiles 생성 응답 파싱 실패 (status %d)", insertResp.StatusCode)
	}
	log.Printf("[DB] 신규 프로필 생성: %s → %s", username, newRows[0].ID)
	return newRows[0].ID, nil
}

// LoadUserRecords는 game_records 테이블에서 유저의 모든 전적을 읽어
// client.Records 형태(map[string]*GameRecord)로 반환합니다.
// "total" 전적은 모든 게임 전적의 합계로 계산됩니다.
// token: 유저 JWT — RLS 정책 통과를 위해 Authorization 헤더에 사용
func (d *DBClient) LoadUserRecords(userUUID string, token string) map[string]*GameRecord {
	path := "/game_records?user_id=eq." + url.QueryEscape(userUUID)
	resp, err := d.do("GET", path, nil, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		log.Printf("[DB] LoadUserRecords 실패 [%s]: %v", userUUID, err)
		return nil
	}
	defer resp.Body.Close()

	var rows []gameRecordRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		log.Printf("[DB] LoadUserRecords 파싱 실패: %v", err)
		return nil
	}

	records := map[string]*GameRecord{
		"total":          {},
		"omok":           {},
		"blackjack":      {},
		"blackjack_raid": {},
	}
	for _, row := range rows {
		memKey := gameMemKey(row.GameName)
		records[memKey] = &GameRecord{
			Wins:   row.Wins,
			Losses: row.Losses,
			Draws:  row.Draws,
		}
		// PVE 전적은 메인 랭킹(total)에서 제외 (blackjack 일반만, blackjack_raid 레이드는 포함)
		if row.GameName != "blackjack" {
			records["total"].Wins += row.Wins
			records["total"].Losses += row.Losses
			records["total"].Draws += row.Draws
		}
	}
	log.Printf("[DB] 전적 복구 [%s]: total=%dW/%dL/%dD",
		userUUID[:min(8, len(userUUID))],
		records["total"].Wins, records["total"].Losses, records["total"].Draws)
	return records
}

// UpsertGameRecord는 game_records 테이블에 해당 유저+게임의 전적을 upsert(삽입 또는 갱신)합니다.
// Supabase PostgREST: POST + on_conflict=user_id,game_name + Prefer: resolution=merge-duplicates (ON CONFLICT DO UPDATE)
// token: 유저 JWT — RLS 정책 통과를 위해 Authorization 헤더에 사용 (Anon Key 대신)
// 실패 시 최대 2회 재시도하여 DB 저장을 보장합니다.
// 여러 고루틴이 동시에 호출해도 mu로 직렬화하여 한 명씩 순서대로 저장합니다.
func (d *DBClient) UpsertGameRecord(userUUID, gameName string, isPVE bool, wins, losses, draws int, token string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	body := gameRecordRow{
		UserID:   userUUID,
		GameName: gameName,
		IsPVE:    isPVE,
		Wins:     wins,
		Losses:   losses,
		Draws:    draws,
	}
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := d.do("POST", "/game_records?on_conflict=user_id,game_name", body, map[string]string{
			"Prefer":        "resolution=merge-duplicates,return=minimal",
			"Authorization": "Bearer " + token,
		})
		if err != nil {
			log.Printf("[DB] UpsertGameRecord 실패 [%s/%s] attempt %d: %v", userUUID[:min(8, len(userUUID))], gameName, attempt+1, err)
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("[DB] UpsertGameRecord 완료 [%s/%s] %dW/%dL/%dD (status %d)",
				userUUID[:min(8, len(userUUID))], gameName, wins, losses, draws, resp.StatusCode)
			return
		}
		log.Printf("[DB] UpsertGameRecord HTTP %d [%s/%s] attempt %d", resp.StatusCode, userUUID[:min(8, len(userUUID))], gameName, attempt+1)
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
		}
	}
}

// min은 Go 1.21 이전 버전을 위한 정수 최솟값 헬퍼입니다.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
