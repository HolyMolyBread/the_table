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
	case "omok", "connect4", "tictactoe", "indian", "holdem", "sevenpoker", "thief", "onecard", "mahjong", "mahjong3", "blackjack":
		// 지원하는 게임
	default:
		log.Printf("[BOT] 지원하지 않는 게임 접두사: %s", gamePrefix)
		return nil // 에러 대신 무시
	}

	// 인원 제한: 1:1 게임은 2명, 다인(holdem, sevenpoker, thief, onecard, blackjack)은 4명, mahjong3은 3명
	maxPlayers := 2
	if prefix == "holdem" || prefix == "sevenpoker" || prefix == "thief" || prefix == "onecard" || prefix == "mahjong" || prefix == "blackjack" {
		maxPlayers = 4
	}
	if prefix == "mahjong3" {
		maxPlayers = 3
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
			"total":       {},
			"omok":        {},
			"tictactoe":   {},
			"connect4":    {},
			"indian":      {},
			"holdem":      {},
			"sevenpoker":  {},
			"thief":       {},
			"onecard":     {},
			"mahjong":     {},
			"mahjong3":    {},
			"blackjack":   {},
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

	// 마작/삼마는 플러그인 OnJoin에서 ready_update를 전송하므로 여기서 보내지 않음 (ReadyCount 0으로 꼬이는 버그 방지)
	if prefix != "mahjong" && prefix != "mahjong3" {
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
	var thiefActionDone bool
	var lastOmokBoard [15][15]int
	var lastOmokExclude map[[2]int]bool
	var lastOmokX, lastOmokY int
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
		case "error":
			if gamePrefix != "omok" {
				return
			}
			var errResp struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(msg, &errResp) != nil {
				return
			}
			if !strings.Contains(errResp.Message, "금수") {
				return
			}
			if lastOmokExclude == nil {
				lastOmokExclude = make(map[[2]int]bool)
			}
			lastOmokExclude[[2]int{lastOmokX, lastOmokY}] = true
			time.Sleep(500 * time.Millisecond)
			myColor := 1
			x, y := botPickOmokExcluding(lastOmokBoard, myColor, lastOmokExclude)
			if x < 0 {
				return
			}
			lastOmokX, lastOmokY = x, y
			payload, _ := json.Marshal(map[string]any{"cmd": "place", "x": x, "y": y})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "board_update":
			if gamePrefix != "omok" {
				return
			}
			var d BoardData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			lastOmokBoard = d.Board
			lastOmokExclude = nil
			if d.Turn != bot.UserID {
				return
			}
			myColor := d.Colors[bot.UserID]
			if myColor == 0 {
				myColor = 1
			}
			time.Sleep(botThinkDelay)
			x, y := botPickOmok(d.Board, myColor)
			if x < 0 {
				return
			}
			lastOmokX, lastOmokY = x, y
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
			myColor := d.Colors[bot.UserID]
			if myColor == 0 {
				myColor = 1
			}
			time.Sleep(botThinkDelay)
			r, c := botPickTicTacToe(d.Board, myColor)
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
			myColor := d.Colors[bot.UserID]
			if myColor == 0 {
				myColor = 1
			}
			time.Sleep(botThinkDelay)
			col := botPickConnect4(d.Board, myColor)
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
			cmd := botPickIndian(d)
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
			cmd := botPickHoldem(d, bot.UserID)
			payload, _ := json.Marshal(map[string]any{"cmd": cmd})
			room.Plugin.HandleAction(bot, "game_action", payload)

		case "sevenpoker_state":
			if gamePrefix != "sevenpoker" {
				return
			}
			var d SevenPokerData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			// 초이스 페이즈는 전원 동시 진행이므로 턴 검사 전에 처리
			if d.Phase == "choice" {
				if d.MyChoiceDone {
					return
				}
				time.Sleep(botThinkDelay)
				discardIdx, openIdx := botPickSevenPokerChoice(d, bot.UserID)
				payload, _ := json.Marshal(map[string]any{"cmd": "choice", "discardIdx": discardIdx, "openIdx": openIdx})
				room.Plugin.HandleAction(bot, "game_action", payload)
				return
			}
			// 베팅 페이즈 턴 검사
			if d.CurrentTurn != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			cmd := botPickSevenPokerBet(d, bot.UserID)
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

		case "mahjong3_state":
			if gamePrefix != "mahjong3" {
				return
			}
			var d MahjongData
			if json.Unmarshal(base.Data, &d) != nil {
				return
			}
			if d.CurrentTurn != bot.UserID {
				return
			}
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

		case "blackjack_state", "blackjack_pvp_state":
			if gamePrefix != "blackjack" {
				return
			}
			var data map[string]any
			if json.Unmarshal(base.Data, &data) != nil {
				return
			}
			// PVP 블랙잭은 data["turn"] 사용, PVE는 data["currentTurn"] - 둘 다 지원
			turnUser, _ := data["turn"].(string)
			if turnUser == "" {
				turnUser, _ = data["currentTurn"].(string)
			}
			if turnUser == "" {
				// turnOrder + currentTurnIdx로부터 유도
				if order, ok := data["turnOrder"].([]any); ok {
					if idx, ok := data["currentTurnIdx"].(float64); ok && int(idx) < len(order) {
						if u, ok := order[int(idx)].(string); ok {
							turnUser = u
						}
					}
				}
			}
			if turnUser != bot.UserID {
				return
			}
			time.Sleep(botThinkDelay)
			// Level 1: 70% hit, 30% stand
			cmd := "hit"
			if rand.Intn(100) < 30 {
				cmd = "stand"
			}
			payload, _ := json.Marshal(map[string]any{"cmd": cmd})
			room.Plugin.HandleAction(bot, "game_action", payload)
		}
		}()
	}
}

// omokHeuristicWeights는 휴리스틱 점수 가중치입니다.
const (
	omokMyFive      = 100000 // 내 5목 완성
	omokBlockFive   = 50000  // 상대 5목 차단
	omokMyOpenFour  = 10000  // 내 활사(Open 4)
	omokBlockOpen4  = 8000   // 상대 활사 차단
	omokMyOpenThree = 1000   // 내 활삼(Open 3)
	omokBlockOpen3  = 800    // 상대 활삼 차단
	omokMyFour      = 5000   // 내 4목 (한쪽 막힌 것 포함)
	omokBlockFour   = 2000   // 상대 4목 차단
	omokMyThree     = 500    // 내 3목
	omokBlockThree  = 300    // 상대 3목 차단
)

// omokLineInfo는 (x,y)에서 방향 (dx,dy)로 color 돌의 연속 개수와 양끝 개방 여부를 반환합니다.
func omokLineInfo(board [15][15]int, x, y, dx, dy, color int) (count int, leftOpen, rightOpen bool) {
	count = 1
	// 음의 방향
	for i := 1; i < 15; i++ {
		nx, ny := x-dx*i, y-dy*i
		if nx < 0 || nx >= 15 || ny < 0 || ny >= 15 {
			break
		}
		if board[nx][ny] != color {
			leftOpen = (board[nx][ny] == 0)
			break
		}
		count++
	}
	// 양의 방향
	for i := 1; i < 15; i++ {
		nx, ny := x+dx*i, y+dy*i
		if nx < 0 || nx >= 15 || ny < 0 || ny >= 15 {
			break
		}
		if board[nx][ny] != color {
			rightOpen = (board[nx][ny] == 0)
			break
		}
		count++
	}
	return count, leftOpen, rightOpen
}

// evaluateOmokMove는 (x,y)에 myColor를 두었을 때의 점수를 반환합니다.
func evaluateOmokMove(board [15][15]int, x, y, myColor, oppColor int) int {
	board[x][y] = myColor
	defer func() { board[x][y] = 0 }()

	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	score := 0

	// 내 돌 패턴 점수
	for _, d := range dirs {
		cnt, lo, ro := omokLineInfo(board, x, y, d[0], d[1], myColor)
		openEnds := 0
		if lo {
			openEnds++
		}
		if ro {
			openEnds++
		}
		switch {
		case cnt >= 5:
			score += omokMyFive
		case cnt == 4:
			if openEnds == 2 {
				score += omokMyOpenFour
			} else {
				score += omokMyFour
			}
		case cnt == 3:
			if openEnds == 2 {
				score += omokMyOpenThree
			} else {
				score += omokMyThree
			}
		}
	}

	// 상대 돌 차단 점수 (이 수로 상대의 위협을 막는지)
	board[x][y] = oppColor
	for _, d := range dirs {
		cnt, lo, ro := omokLineInfo(board, x, y, d[0], d[1], oppColor)
		openEnds := 0
		if lo {
			openEnds++
		}
		if ro {
			openEnds++
		}
		switch {
		case cnt >= 5:
			score += omokBlockFive
		case cnt == 4:
			if openEnds == 2 {
				score += omokBlockOpen4
			} else {
				score += omokBlockFour
			}
		case cnt == 3:
			if openEnds == 2 {
				score += omokBlockOpen3
			} else {
				score += omokBlockThree
			}
		}
	}
	board[x][y] = 0

	return score
}

// botPickOmokExcluding은 exclude에 있는 좌표를 제외하고 최적의 수를 선택합니다.
func botPickOmokExcluding(board [15][15]int, myColor int, exclude map[[2]int]bool) (x, y int) {
	if myColor == 0 {
		myColor = 1
	}
	oppColor := 3 - myColor

	var bestCandidates [][2]int
	bestScore := -1

	for i := 0; i < 15; i++ {
		for j := 0; j < 15; j++ {
			if board[i][j] != 0 {
				continue
			}
			if exclude != nil && exclude[[2]int{i, j}] {
				continue
			}
			// 흑(1)일 때 렌주룰 금수 자리 제외
			if myColor == 1 {
				if forbidden, _ := IsRenjuForbiddenBoard(board, i, j); forbidden {
					continue
				}
			}
			score := evaluateOmokMove(board, i, j, myColor, oppColor)
			if score > bestScore {
				bestScore = score
				bestCandidates = [][2]int{{i, j}}
			} else if score == bestScore && score >= 0 {
				bestCandidates = append(bestCandidates, [2]int{i, j})
			}
		}
	}

	if len(bestCandidates) == 0 {
		return -1, -1
	}
	pick := bestCandidates[rand.Intn(len(bestCandidates))]
	return pick[0], pick[1]
}

func botPickOmok(board [15][15]int, myColor int) (x, y int) {
	return botPickOmokExcluding(board, myColor, nil)
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

// tttWouldWin은 (r,c)에 color를 두었을 때 3목 완성인지 검사합니다.
func tttWouldWin(board [3][3]int, r, c, color int) bool {
	board[r][c] = color
	defer func() { board[r][c] = 0 }()
	// 가로
	for row := 0; row < 3; row++ {
		if board[row][0] == color && board[row][1] == color && board[row][2] == color {
			return true
		}
	}
	// 세로
	for col := 0; col < 3; col++ {
		if board[0][col] == color && board[1][col] == color && board[2][col] == color {
			return true
		}
	}
	// 대각선
	if board[0][0] == color && board[1][1] == color && board[2][2] == color {
		return true
	}
	if board[0][2] == color && board[1][1] == color && board[2][0] == color {
		return true
	}
	return false
}

// tttWouldBlock은 (r,c)에 두면 상대(oppColor)의 승리를 막는지 검사합니다.
func tttWouldBlock(board [3][3]int, r, c, oppColor int) bool {
	return tttWouldWin(board, r, c, oppColor)
}

func botPickTicTacToe(board [3][3]int, myColor int) (r, c int) {
	if myColor == 0 {
		myColor = 1
	}
	oppColor := 3 - myColor

	var winMoves, blockMoves, centerMoves, cornerMoves, otherMoves [][2]int

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if board[i][j] != 0 {
				continue
			}
			cell := [2]int{i, j}
			if tttWouldWin(board, i, j, myColor) {
				winMoves = append(winMoves, cell)
			} else if tttWouldBlock(board, i, j, oppColor) {
				blockMoves = append(blockMoves, cell)
			} else if i == 1 && j == 1 {
				centerMoves = append(centerMoves, cell)
			} else if (i == 0 || i == 2) && (j == 0 || j == 2) {
				cornerMoves = append(cornerMoves, cell)
			} else {
				otherMoves = append(otherMoves, cell)
			}
		}
	}

	for _, cand := range [][][2]int{winMoves, blockMoves, centerMoves, cornerMoves, otherMoves} {
		if len(cand) > 0 {
			pick := cand[rand.Intn(len(cand))]
			return pick[0], pick[1]
		}
	}
	return -1, -1
}

// c4Connect4 점수 상수
const (
	c4MyWin      = 10000
	c4BlockWin   = 5000
	c4CenterCol  = 100
	c4AdjCol     = 50
	c4MyThree    = 500
	c4GiveOppWin = -10000 // 상대에게 승리 기회를 주는 수 감점
)

// c4GetRow는 col에 돌을 두었을 때 착지하는 행을 반환합니다. 꽉 찼으면 -1.
func c4GetRow(board [6][7]int, col int) int {
	for r := 5; r >= 0; r-- {
		if board[r][col] == 0 {
			return r
		}
	}
	return -1
}

// c4CountDir는 (row,col)에서 방향 (dr,dc)로 연속된 color 개수를 셉니다.
func c4CountDir(board [6][7]int, row, col, dr, dc, color int) int {
	count := 1
	for i := 1; i < 4; i++ {
		r, c := row+dr*i, col+dc*i
		if r < 0 || r >= 6 || c < 0 || c >= 7 || board[r][c] != color {
			break
		}
		count++
	}
	for i := 1; i < 4; i++ {
		r, c := row-dr*i, col-dc*i
		if r < 0 || r >= 6 || c < 0 || c >= 7 || board[r][c] != color {
			break
		}
		count++
	}
	return count
}

// c4CheckWin은 (row,col)에 color가 4목을 달성했는지 검사합니다.
func c4CheckWin(board [6][7]int, row, col, color int) bool {
	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		if c4CountDir(board, row, col, d[0], d[1], color) >= 4 {
			return true
		}
	}
	return false
}

// c4HasThree는 (row,col)에 color를 두었을 때 3목 이상이 되는 방향이 있는지 검사합니다.
func c4HasThree(board [6][7]int, row, col, color int) bool {
	dirs := [4][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		if c4CountDir(board, row, col, d[0], d[1], color) >= 3 {
			return true
		}
	}
	return false
}

// c4OpponentCanWin은 board 상태에서 oppColor가 한 수로 이길 수 있는 열이 있는지 검사합니다.
func c4OpponentCanWin(board [6][7]int, oppColor int) bool {
	for col := 0; col < 7; col++ {
		row := c4GetRow(board, col)
		if row < 0 {
			continue
		}
		board[row][col] = oppColor
		wins := c4CheckWin(board, row, col, oppColor)
		board[row][col] = 0
		if wins {
			return true
		}
	}
	return false
}

func botPickConnect4(board [6][7]int, myColor int) int {
	if myColor == 0 {
		myColor = 1
	}
	oppColor := 3 - myColor

	var bestCols []int
	bestScore := -999999

	for col := 0; col < 7; col++ {
		row := c4GetRow(board, col)
		if row < 0 {
			continue
		}

		// 보드 복사 후 시뮬레이션 (원본 변경 방지)
		var sim [6][7]int
		for r := 0; r < 6; r++ {
			for c := 0; c < 7; c++ {
				sim[r][c] = board[r][c]
			}
		}

		sim[row][col] = myColor
		score := 0

		// 내 4목 (승리)
		if c4CheckWin(sim, row, col, myColor) {
			score += c4MyWin
		}
		sim[row][col] = 0

		// 상대 4목 차단: 이 위치에 상대가 두면 이기므로, 내가 두면 차단
		sim[row][col] = oppColor
		if c4CheckWin(sim, row, col, oppColor) {
			score += c4BlockWin
		}
		sim[row][col] = 0

		// 내 3목 (활성)
		sim[row][col] = myColor
		if c4HasThree(sim, row, col, myColor) {
			score += c4MyThree
		}

		// 감점: 내가 둔 후 상대가 다음 수로 이길 수 있으면
		if c4OpponentCanWin(sim, oppColor) {
			score += c4GiveOppWin
		}
		sim[row][col] = 0

		// 열 위치 보너스
		switch col {
		case 3:
			score += c4CenterCol
		case 2, 4:
			score += c4AdjCol
		}

		if score > bestScore {
			bestScore = score
			bestCols = []int{col}
		} else if score == bestScore {
			bestCols = append(bestCols, col)
		}
	}

	if len(bestCols) == 0 {
		return -1
	}
	return bestCols[rand.Intn(len(bestCols))]
}

// SpawnBotsForPVE는 PVE 방에 유저가 입장했을 때 빈 자리를 봇으로 채웁니다.
func SpawnBotsForPVE(m *RoomManager, room *Room, gamePrefix string) {
	prefix := strings.ToLower(strings.TrimSpace(gamePrefix))
	// 블랙잭은 딜러 AI만 사용하므로 봇 소환 불필요
	if prefix == "blackjack" {
		return
	}
	maxPlayers := 2
	if prefix == "holdem" || prefix == "sevenpoker" || prefix == "thief" || prefix == "onecard" || prefix == "mahjong" {
		maxPlayers = 4
	}
	if prefix == "mahjong3" {
		maxPlayers = 3
	}
	for room.count() < maxPlayers {
		if err := SpawnBot(m, room, prefix); err != nil {
			return
		}
	}
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

// botPickHoldem은 HoldemData를 기반으로 check/fold를 결정합니다.
// EvaluateHand로 7장 족보 점수를 계산하고, 족보 등급에 따라 액션을 선택합니다.
func botPickHoldem(d HoldemData, myUserID string) string {
	cards7 := make([]Card, 0, 7)

	// 내 핸드 카드 (2장)
	for _, p := range d.Players {
		if p.UserID == myUserID && len(p.Cards) >= 2 {
			for _, c := range p.Cards {
				if c.Suit != "" || c.Value != "" {
					cards7 = append(cards7, c)
				}
			}
			break
		}
	}

	// 커뮤니티 카드 (페이즈에 따라 0~5장)
	communityVisible := 0
	switch d.Phase {
	case "flop":
		communityVisible = 3
	case "turn":
		communityVisible = 4
	case "river", "showdown":
		communityVisible = 5
	}
	for i := 0; i < communityVisible && i < len(d.CommunityCards); i++ {
		c := d.CommunityCards[i]
		if c.Suit != "" || c.Value != "" {
			cards7 = append(cards7, c)
		}
	}

	// 상대 배팅 여부: 다른 플레이어가 이미 체크(액션)했는지
	opponentBet := false
	for _, p := range d.Players {
		if p.UserID != myUserID && p.IsActive && p.Status == "check" {
			opponentBet = true
			break
		}
	}

	rank := 1 // 기본: 하이카드
	if len(cards7) >= 5 {
		score := EvaluateHand(cards7)
		rank = int(score >> 20)
		if rank <= 0 {
			rank = 1
		}
	}

	r := rand.Intn(100)
	switch {
	case rank >= 4: // 트리플 이상 (공격적: 90% Call, 10% Raise → check만 가능하므로 95% check)
		if r < 95 {
			return "check"
		}
		return "fold"
	case rank == 3: // 투페어
		if r < 80 {
			return "check"
		}
		return "fold"
	case rank == 2: // 원페어
		if opponentBet {
			if r < 50 {
				return "check"
			}
			return "fold"
		}
		return "check"
	default: // 하이카드 (rank 1)
		if opponentBet {
			if r < 90 {
				return "fold"
			}
			return "check" // 10% 블러핑
		}
		return "check"
	}
}

// botPickIndian은 IndianData를 기반으로 showdown/give_up을 결정합니다.
// 상대 카드(OpponentCard) 값을 숫자로 변환하여 전략을 적용합니다.
func botPickIndian(d IndianData) string {
	oppVal := cardRank(d.OpponentCard) // 2~14 (A=14)
	if oppVal <= 0 {
		oppVal = 7 // 알 수 없으면 중간값
	}

	r := rand.Intn(100)
	switch {
	case oppVal >= 8: // 8 이상 (8,9,10,J,Q,K,A)
		if r < 70 {
			return "give_up"
		}
		return "showdown"
	case oppVal <= 3: // 3 이하 (2,3)
		if r < 90 {
			return "showdown"
		}
		return "give_up"
	default: // 4~7
		if r < 60 {
			return "showdown"
		}
		return "give_up"
	}
}

// botPickSevenPokerChoice는 4장 중 버릴 카드(discardIdx)와 공개할 카드(openIdx)를 선택합니다.
// discardIdx: 페어가 아니면서 숫자가 낮은 카드. openIdx: 남은 카드 중 가장 높은 숫자/문양.
func botPickSevenPokerChoice(d SevenPokerData, myUserID string) (discardIdx, openIdx int) {
	var myCards []Card
	for _, p := range d.Players {
		if p.UserID == myUserID && len(p.Cards) >= 4 {
			myCards = p.Cards[0:4]
			break
		}
	}
	if len(myCards) < 4 {
		d, o := rand.Intn(4), rand.Intn(4)
		for o == d {
			o = rand.Intn(4)
		}
		return d, o
	}

	// 각 카드의 rank 빈도 계산 (페어 여부 판단)
	rankCount := make(map[int]int)
	for _, c := range myCards {
		rankCount[cardRank(c)]++
	}

	// discardIdx: 페어가 아닌 카드 중 가장 낮은 숫자. 모두 페어면 가장 낮은 카드.
	discardIdx = 0
	bestDiscardPriority := 999
	for i, c := range myCards {
		r := cardRank(c)
		s := suitRank(c)
		isPair := rankCount[r] >= 2
		// 페어면 우선 유지(높은 priority). 싱글톤 중 낮은 숫자 우선 버림.
		priority := r*10 + s
		if isPair {
			priority += 200
		}
		if priority < bestDiscardPriority {
			bestDiscardPriority = priority
			discardIdx = i
		}
	}

	// openIdx: discardIdx 제외한 3장 중 가장 높은 숫자, 동점이면 문양 좋은 것
	openIdx = -1
	openScore := -1
	for i, c := range myCards {
		if i == discardIdx {
			continue
		}
		r := cardRank(c)
		s := suitRank(c)
		score := r*10 + s
		if score > openScore {
			openScore = score
			openIdx = i
		}
	}
	if openIdx < 0 {
		for i := 0; i < 4; i++ {
			if i != discardIdx {
				openIdx = i
				break
			}
		}
	}
	return discardIdx, openIdx
}

// botPickSevenPokerBet는 현재 7장(또는 그 이하)의 족보를 기반으로 check/fold를 결정합니다.
func botPickSevenPokerBet(d SevenPokerData, myUserID string) string {
	var myCards []Card
	for _, p := range d.Players {
		if p.UserID == myUserID {
			for _, c := range p.Cards {
				if c.Suit != "" || c.Value != "" {
					myCards = append(myCards, c)
				}
			}
			break
		}
	}

	// 상대 배팅 여부
	opponentBet := false
	for _, p := range d.Players {
		if p.UserID != myUserID && p.IsActive && p.Status == "check" {
			opponentBet = true
			break
		}
	}

	rank := 1
	if len(myCards) >= 5 {
		score := EvaluateHand(myCards)
		rank = int(score >> 20)
		if rank <= 0 {
			rank = 1
		}
	}

	r := rand.Intn(100)
	switch {
	case rank >= 4: // 트리플 이상: 80% Raise, 20% Call → check만 가능하므로 95% check
		if r < 95 {
			return "check"
		}
		return "fold"
	case rank == 3: // 투페어: 90% Call, 10% Check
		if r < 90 {
			return "check"
		}
		return "fold"
	default: // 원페어 이하: 상대 배팅 시 70% Fold, 30% 뻥카 Call
		if opponentBet {
			if r < 70 {
				return "fold"
			}
			return "check"
		}
		return "check"
	}
}
