package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

const botThinkDelay = 1500 * time.Millisecond

// SpawnBot은 지정된 방에 AI 봇(가상 클라이언트)을 추가합니다.
// gamePrefix는 방 ID 접두사(omok, connect4, tictactoe)로, 해당 게임의 착수 로직을 결정합니다.
func SpawnBot(m *RoomManager, room *Room, gamePrefix string) error {
	prefix := strings.ToLower(strings.TrimSpace(gamePrefix))
	switch prefix {
	case "omok", "connect4", "tictactoe", "indian", "holdem", "sevenpoker", "thief", "onecard", "mahjong":
		// 지원하는 게임
	default:
		log.Printf("[BOT] 지원하지 않는 게임 접두사: %s", gamePrefix)
		return nil // 에러 대신 무시
	}

	// 인원 제한: 1:1 게임은 2명, 다인(holdem, sevenpoker, thief, onecard)은 4명
	maxPlayers := 2
	if prefix == "holdem" || prefix == "sevenpoker" || prefix == "thief" || prefix == "onecard" || prefix == "mahjong" {
		maxPlayers = 4
	}
	if room.count() >= maxPlayers {
		log.Printf("[BOT] room:[%s] 인원이 가득 찼습니다", room.ID)
		return nil
	}

	bot := &Client{
		manager:   m,
		conn:      nil,
		send:      nil, // 봇은 send 채널 미사용
		UserID:    fmt.Sprintf("🤖 AI (Level 1) - %04d", rand.Intn(10000)),
		RoomID:    room.ID,
		IsBot:     true,
		BotProcess: nil, // 아래에서 설정
		Records: map[string]*GameRecord{
			"total":      {},
			"omok":       {},
			"tictactoe":  {},
			"connect4":   {},
			"indian":     {},
			"holdem":     {},
			"sevenpoker": {},
			"thief":      {},
			"onecard":    {},
			"mahjong":    {},
		},
	}

	bot.BotProcess = makeBotProcess(bot, room, prefix)

	// 방에 합류 (JoinRoom과 동일한 방식으로 처리)
	room.mu.Lock()
	room.clients[bot] = true
	room.mu.Unlock()
	bot.RoomID = room.ID

	log.Printf("[BOT] [%s] → room:[%s] (게임: %s)", bot.UserID, room.ID, prefix)

	// 입장 브로드캐스트
	resp := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		UserID  string `json:"userId"`
		RoomID  string `json:"roomId"`
	}{"join", "[" + bot.UserID + "] 님이 입장했습니다", bot.UserID, room.ID}
	data, _ := json.Marshal(resp)
	room.broadcastAll(data)

	// 마작은 플러그인 OnJoin에서 ready_update를 전송하므로 여기서 보내지 않음 (ReadyCount 0으로 꼬이는 버그 방지)
	if prefix != "mahjong" {
		m.broadcastRoomUpdate(room)
	}

	if room.Plugin != nil {
		room.Plugin.OnJoin(bot)
		// 자동 레디: 1초 후 ready 액션 전송
		go func() {
			time.Sleep(1 * time.Second)
			payload, _ := json.Marshal(map[string]any{"cmd": "ready"})
			room.Plugin.HandleAction(bot, "game_action", payload)
		}()
	}

	return nil
}

func makeBotProcess(bot *Client, room *Room, gamePrefix string) func(msg []byte) {
	var thiefActionDone bool // 도둑잡기 이번 턴 행동 여부 기억
	return func(msg []byte) {
		var base struct {
			Type   string          `json:"type"`
			RoomID string          `json:"roomId"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			return
		}

		go func() {
			switch base.Type {
		case "game_result":
			var gr struct {
				RematchEnabled bool `json:"rematchEnabled"`
			}
			if json.Unmarshal(msg, &gr) == nil && gr.RematchEnabled {
				go func() {
					time.Sleep(2 * time.Second)
					payload, _ := json.Marshal(map[string]any{"cmd": "rematch"})
					room.Plugin.HandleAction(bot, "game_action", payload)
				}()
			}
			return
		case "board_update":
			if gamePrefix != "omok" {
				return
			}
			var d BoardData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.Turn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			x, y := botPickOmok(d.Board)
			if x < 0 {
				return
			}
			payload, _ := json.Marshal(map[string]any{"cmd": "place", "x": x, "y": y})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "tictactoe_state":
			if gamePrefix != "tictactoe" {
				return
			}
			var d TicTacToeData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.Turn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			r, c := botPickTicTacToe(d.Board)
			if r < 0 {
				return
			}
			payload, _ := json.Marshal(map[string]any{"cmd": "place", "r": r, "c": c})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "connect4_state":
			if gamePrefix != "connect4" {
				return
			}
			var d Connect4Data
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.Turn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			col := botPickConnect4(d.Board)
			if col < 0 {
				return
			}
			payload, _ := json.Marshal(map[string]any{"cmd": "place", "col": col})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "indian_state":
			if gamePrefix != "indian" {
				return
			}
			var d IndianData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.Turn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			// Level 1: 70% showdown, 30% give_up
			cmd := "showdown"
			if rand.Intn(100) < 30 {
				cmd = "give_up"
			}
			payload, _ := json.Marshal(map[string]any{"cmd": cmd})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "holdem_state":
			if gamePrefix != "holdem" {
				return
			}
			var d HoldemData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.CurrentTurn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			// Level 1: 95% check, 5% fold (테스트 용이성)
			cmd := "check"
			if rand.Intn(100) < 5 {
				cmd = "fold"
			}
			payload, _ := json.Marshal(map[string]any{"cmd": cmd})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "sevenpoker_state":
			if gamePrefix != "sevenpoker" {
				return
			}
			var d struct {
				Phase        string `json:"phase"`
				CurrentTurn  string `json:"currentTurn"`
				MyChoiceDone bool   `json:"myChoiceDone"`
			}
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			// 초이스 페이즈는 전원 동시 진행이므로 턴 검사 전에 처리
			if d.Phase == "choice" {
				if d.MyChoiceDone {
					return
				}
				time.Sleep(botThinkDelay)
				discardIdx := rand.Intn(4)
				openIdx := rand.Intn(4)
				for openIdx == discardIdx {
					openIdx = rand.Intn(4)
				}
				payload, _ := json.Marshal(map[string]any{"cmd": "choice", "discardIdx": discardIdx, "openIdx": openIdx})
				room.Plugin.HandleAction(bot, "game_action", payload)
				return
			}
			// 베팅 페이즈 턴 검사
			if d.CurrentTurn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			cmd := "check"
			if rand.Intn(100) < 5 {
				cmd = "fold"
			}
			payload, _ := json.Marshal(map[string]any{"cmd": cmd})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "thief_state":
			if gamePrefix != "thief" {
				return
			}
			var d ThiefData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			// 내 턴이 아니면 행동 플래그 초기화
			if d.Turn != bot.UserID {
				thiefActionDone = false
				return
			}
			// 이번 턴에 이미 행동을 예약했다면 무시 (1.5초 딜레이 중복 실행 방지)
			if thiefActionDone {
				return
			}
			thiefActionDone = true

			targetID := d.TargetUserID
			cardCount := 0
			for _, p := range d.Players {
				if p.UserID == targetID {
					cardCount = p.CardCount
					break
				}
			}
			if targetID == "" || cardCount == 0 {
				return
			}
			go func() {
				time.Sleep(botThinkDelay) // 턴 시작 후 짝 맞추기 연출 대기

				idx1 := rand.Intn(cardCount)
				payload1, _ := json.Marshal(map[string]any{"cmd": "hover", "targetId": targetID, "index": idx1})
				room.Plugin.HandleAction(bot, "game_action", payload1)
				time.Sleep(800 * time.Millisecond)
				idx2 := idx1
				if cardCount > 1 {
					idx2 = rand.Intn(cardCount)
				}
				payload2, _ := json.Marshal(map[string]any{"cmd": "hover", "targetId": targetID, "index": idx2})
				room.Plugin.HandleAction(bot, "game_action", payload2)
				time.Sleep(800 * time.Millisecond)
				payload3, _ := json.Marshal(map[string]any{"cmd": "draw", "targetId": targetID, "index": idx2})
				room.Plugin.HandleAction(bot, "game_action", payload3)
			}()

		case "mahjong_state":
			if gamePrefix != "mahjong" {
				return
			}
			var d MahjongData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.CurrentTurn != bot.UserID {
				return
			}
			// 14장일 때 타패 가능
			myHand := d.MyHand
			if len(myHand) != 14 {
				return
			}
			go func() {
				time.Sleep(botThinkDelay)
				idx := rand.Intn(14)
				payload, _ := json.Marshal(map[string]any{"cmd": "discard", "index": idx})
				room.Plugin.HandleAction(bot, "game_action", payload)
			}()

		case "onecard_state":
			if gamePrefix != "onecard" {
				return
			}
			var d OneCardData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.Turn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)

			playIdx := botPickOneCard(d)

			if playIdx >= 0 {
				card := d.Hand[playIdx]
				payloadMap := map[string]any{"cmd": "play", "index": playIdx}
				if card.Value == "7" {
					suits := []string{"♠", "♥", "♦", "♣"}
					payloadMap["targetSuit"] = suits[rand.Intn(len(suits))]
				}
				payload, _ := json.Marshal(payloadMap)
				room.Plugin.HandleAction(bot, "game_action", payload)
			} else {
				payload, _ := json.Marshal(map[string]any{"cmd": "draw"})
				room.Plugin.HandleAction(bot, "game_action", payload)
			}
		}
		}()
	}
}

func botPickOmok(board [15][15]int) (x, y int) {
	// 기존 돌과 인접한 빈칸 후보 수집
	var candidates [][2]int
	for i := 0; i < 15; i++ {
		for j := 0; j < 15; j++ {
			if board[i][j] != 0 {
				continue
			}
			adj := false
			for di := -1; di <= 1 && !adj; di++ {
				for dj := -1; dj <= 1 && !adj; dj++ {
					if di == 0 && dj == 0 {
						continue
					}
					ni, nj := i+di, j+dj
					if ni >= 0 && ni < 15 && nj >= 0 && nj < 15 && board[ni][nj] != 0 {
						adj = true
					}
				}
			}
			if adj || isEmptyBoard(board) {
				candidates = append(candidates, [2]int{i, j})
			}
		}
	}
	if len(candidates) == 0 {
		return -1, -1
	}
	pick := candidates[rand.Intn(len(candidates))]
	return pick[0], pick[1]
}

func isEmptyBoard(board [15][15]int) bool {
	for i := 0; i < 15; i++ {
		for j := 0; j < 15; j++ {
			if board[i][j] != 0 {
				return false
			}
		}
	}
	return true
}

func botPickTicTacToe(board [3][3]int) (r, c int) {
	var empty [][2]int
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if board[i][j] == 0 {
				empty = append(empty, [2]int{i, j})
			}
		}
	}
	if len(empty) == 0 {
		return -1, -1
	}
	pick := empty[rand.Intn(len(empty))]
	return pick[0], pick[1]
}

func botPickConnect4(board [6][7]int) int {
	var cols []int
	for c := 0; c < 7; c++ {
		if board[0][c] == 0 {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return -1
	}
	return cols[rand.Intn(len(cols))]
}

func botPickOneCard(d OneCardData) int {
	top := d.TopCard
	suit := d.TargetSuit
	if suit == "" {
		suit = top.Suit
	}
	for i, c := range d.Hand {
		if d.AttackPenalty > 0 {
			// 방어 로직 (A, B_JOKER, C_JOKER)
			if top.Value == "A" && (c.Value == "A" || c.Value == "B_JOKER" || c.Value == "C_JOKER") {
				return i
			}
			if top.Value == "B_JOKER" && c.Value == "C_JOKER" {
				return i
			}
		} else {
			// 일반 플레이 로직
			if c.Suit == suit || c.Value == top.Value || c.Value == "B_JOKER" || c.Value == "C_JOKER" {
				return i
			}
		}
	}
	return -1 // 낼 카드가 없으면 -1 반환 (Draw)
}
