package main

import (
	"encoding/json"
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
	case "omok", "connect4", "tictactoe":
		// 지원하는 게임
	default:
		log.Printf("[BOT] 지원하지 않는 게임 접두사: %s", gamePrefix)
		return nil // 에러 대신 무시
	}

	// 이미 봇이 있는지 확인
	room.mu.RLock()
	for c := range room.clients {
		if c.IsBot {
			room.mu.RUnlock()
			log.Printf("[BOT] room:[%s] 이미 봇이 있습니다", room.ID)
			return nil
		}
	}
	room.mu.RUnlock()

	// 1:1 게임은 2명이면 꽉 참
	if room.count() >= 2 {
		log.Printf("[BOT] room:[%s] 인원이 가득 찼습니다", room.ID)
		return nil
	}

	bot := &Client{
		manager:   m,
		conn:      nil,
		send:      nil, // 봇은 send 채널 미사용
		UserID:    "🤖 AI (Level 1)",
		RoomID:    room.ID,
		IsBot:     true,
		BotProcess: nil, // 아래에서 설정
		Records: map[string]*GameRecord{
			"total":     {},
			"omok":      {},
			"tictactoe": {},
			"connect4":  {},
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

	if room.Plugin != nil {
		room.Plugin.OnJoin(bot)
	}

	return nil
}

func makeBotProcess(bot *Client, room *Room, gamePrefix string) func(msg []byte) {
	return func(msg []byte) {
		var base struct {
			Type   string          `json:"type"`
			RoomID string          `json:"roomId"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(msg, &base); err != nil {
			return
		}

		switch base.Type {
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
		}
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
