package main

import (
	"math/rand"
	"sort"
)

// ── 카드 / 덱 (포커·블랙잭·인디언 공통) ─────────────────────────────────────────

// Card는 트럼프 카드 한 장을 표현합니다.
type Card struct {
	Suit   string `json:"suit"`   // ♠ ♥ ♦ ♣
	Value  string `json:"value"`  // A 2~10 J Q K
	Hidden bool   `json:"hidden"` // true → 클라이언트에 뒷면으로 전송
}

var (
	standardSuits  = []string{"♠", "♥", "♦", "♣"}
	standardValues = []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
)

// NewShuffledDeck은 52장의 표준 덱을 생성하고 셔플합니다.
func NewShuffledDeck() []Card {
	deck := make([]Card, 0, 52)
	for _, s := range standardSuits {
		for _, v := range standardValues {
			deck = append(deck, Card{Suit: s, Value: v})
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

// ── 족보 판정 (홀덤·세븐포커 공통) ─────────────────────────────────────────────

// cardRank는 카드 숫자 값을 반환합니다. 2=2, ..., A=14
func cardRank(c Card) int {
	m := map[string]int{
		"2": 2, "3": 3, "4": 4, "5": 5, "6": 6,
		"7": 7, "8": 8, "9": 9, "10": 10,
		"J": 11, "Q": 12, "K": 13, "A": 14,
	}
	if v, ok := m[c.Value]; ok {
		return v
	}
	return 0
}

// suitRank는 문양 순위 (동점 시 비교용). ♣<♦<♥<♠
func suitRank(c Card) int {
	m := map[string]int{"♣": 1, "♦": 2, "♥": 3, "♠": 4}
	if v, ok := m[c.Suit]; ok {
		return v
	}
	return 0
}

// EvaluateHand는 7장 카드에서 최고 5장 족보 점수를 반환합니다.
// 점수: (족보등급 << 20) | (타이브레이크 값들)
// 족보: 로티플(10) > 스트레이트플러시(9) > 포카드(8) > 풀하우스(7) > 플러시(6) > 스트레이트(5) > 트리플(4) > 투페어(3) > 원페어(2) > 하이카드(1)
func EvaluateHand(cards []Card) int64 {
	if len(cards) < 5 {
		return 0
	}
	best := int64(0)
	indices := make([]int, 5)
	var comb func(start, depth int)
	comb = func(start, depth int) {
		if depth == 5 {
			five := make([]Card, 5)
			for i, idx := range indices {
				five[i] = cards[idx]
			}
			score := evalFive(five)
			if score > best {
				best = score
			}
			return
		}
		for i := start; i <= len(cards)-5+depth; i++ {
			indices[depth] = i
			comb(i+1, depth+1)
		}
	}
	comb(0, 0)
	return best
}

func evalFive(cards []Card) int64 {
	ranks := make([]int, 5)
	suits := make([]int, 5)
	for i, c := range cards {
		ranks[i] = cardRank(c)
		suits[i] = suitRank(c)
	}
	sort.Slice(cards, func(i, j int) bool {
		ri, rj := cardRank(cards[i]), cardRank(cards[j])
		if ri != rj {
			return ri > rj
		}
		return suitRank(cards[i]) > suitRank(cards[j])
	})
	rankCounts := make(map[int]int)
	for _, r := range ranks {
		rankCounts[r]++
	}
	sortedRanks := make([]int, len(ranks))
	copy(sortedRanks, ranks)
	sort.Sort(sort.Reverse(sort.IntSlice(sortedRanks)))

	flush := suits[0] == suits[1] && suits[1] == suits[2] && suits[2] == suits[3] && suits[3] == suits[4]
	straightVal := straightValue(sortedRanks)

	if flush && straightVal == 14 {
		return (10 << 20) | int64(sortedRanks[0])
	}
	if flush && straightVal > 0 {
		return (9 << 20) | int64(straightVal)
	}
	for r, cnt := range rankCounts {
		if cnt == 4 {
			kicker := 0
			for _, v := range sortedRanks {
				if v != r {
					kicker = v
					break
				}
			}
			return (8 << 20) | (int64(r) << 8) | int64(kicker)
		}
	}
	var trip, pair int
	for r, cnt := range rankCounts {
		if cnt == 3 {
			trip = r
		}
		if cnt == 2 {
			if pair == 0 || r > pair {
				pair = r
			}
		}
	}
	if trip > 0 && pair > 0 {
		return (7 << 20) | (int64(trip) << 8) | int64(pair)
	}
	if flush {
		score := int64(6 << 20)
		for i, r := range sortedRanks {
			score |= int64(r) << (uint(4-i) * 4)
		}
		return score
	}
	if straightVal > 0 {
		return (5 << 20) | int64(straightVal)
	}
	if trip > 0 {
		kickers := make([]int, 0)
		for _, r := range sortedRanks {
			if r != trip {
				kickers = append(kickers, r)
			}
		}
		score := (4 << 20) | (int64(trip) << 12)
		for i, k := range kickers {
			if i < 2 {
				score |= int64(k) << (uint(1-i) * 4)
			}
		}
		return score
	}
	pairs := make([]int, 0)
	for r, cnt := range rankCounts {
		if cnt == 2 {
			pairs = append(pairs, r)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))
	if len(pairs) >= 2 {
		kicker := 0
		for _, r := range sortedRanks {
			if r != pairs[0] && r != pairs[1] {
				kicker = r
				break
			}
		}
		return (3 << 20) | (int64(pairs[0]) << 12) | (int64(pairs[1]) << 8) | int64(kicker)
	}
	if len(pairs) == 1 {
		kickers := make([]int, 0)
		for _, r := range sortedRanks {
			if r != pairs[0] {
				kickers = append(kickers, r)
			}
		}
		score := (2 << 20) | (int64(pairs[0]) << 12)
		for i, k := range kickers {
			if i < 3 {
				score |= int64(k) << (uint(2-i) * 4)
			}
		}
		return score
	}
	score := int64(1 << 20)
	for i, r := range sortedRanks {
		if i < 5 {
			score |= int64(r) << (uint(4-i) * 4)
		}
	}
	return score
}

// HandRankName은 족보 점수에서 한글 족보명을 반환합니다.
func HandRankName(score int64) string {
	rank := int(score >> 20)
	names := map[int]string{
		10: "로티플", 9: "스트레이트플러시", 8: "포카드", 7: "풀하우스",
		6: "플러시", 5: "스트레이트", 4: "트리플", 3: "투페어",
		2: "원페어", 1: "하이카드",
	}
	if n, ok := names[rank]; ok {
		return n
	}
	return "하이카드"
}

// PokerHandDisplayName은 HandRankName 결과를 UI 표시용 이름으로 변환합니다.
func PokerHandDisplayName(rankName string) string {
	m := map[string]string{
		"로티플": "로얄 스트레이트 플러시",
		"스트레이트플러시": "스트레이트 플러시",
		"하이카드": "하이카드 (탑)",
	}
	if d, ok := m[rankName]; ok {
		return d
	}
	return rankName
}

func straightValue(sorted []int) int {
	unique := make([]int, 0)
	seen := make(map[int]bool)
	for _, r := range sorted {
		if !seen[r] {
			seen[r] = true
			unique = append(unique, r)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(unique)))
	for i := 0; i <= len(unique)-5; i++ {
		if unique[i]-unique[i+4] == 4 {
			return unique[i]
		}
	}
	if seen[14] && seen[2] && seen[3] && seen[4] && seen[5] {
		return 5
	}
	return 0
}

// ── 쇼다운 응답 타입 (홀덤·세븐포커 공통) ─────────────────────────────────────

// PokerShowdownParticipant는 쇼다운 참가자 정보입니다.
type PokerShowdownParticipant struct {
	UserID              string `json:"userId"`
	HandName            string `json:"handName"`
	WinReason           string `json:"winReason,omitempty"`           // "A Kicker", "Spade" 등 승리 요인
	HighCardHighlightIdx int   `json:"highCardHighlightIdx,omitempty"` // 하이카드 시 톱 카드 인덱스 (0~6)
}

// PokerShowdownResultData는 poker_showdown_result 메시지의 data 필드입니다.
type PokerShowdownResultData struct {
	WinnerID     string                    `json:"winnerId"`
	WinningHand  string                    `json:"winningHand"`
	WinReason    string                    `json:"winReason,omitempty"`
	Participants []PokerShowdownParticipant `json:"participants"`
}

// rankToStr은 카드 랭크 숫자를 문자열로 변환합니다.
func rankToStr(r int) string {
	m := map[int]string{14: "A", 13: "K", 12: "Q", 11: "J", 10: "10", 9: "9", 8: "8", 7: "7", 6: "6", 5: "5", 4: "4", 3: "3", 2: "2"}
	if s, ok := m[r]; ok {
		return s
	}
	return "?"
}

// HandWinReason은 족보 점수에서 승리 요인 문자열을 추출합니다.
func HandWinReason(score int64) string {
	rank := int(score >> 20)
	switch rank {
	case 10:
		return "로얄"
	case 9:
		return rankToStr(int(score & 0xFFFF)) + " 하이"
	case 8:
		quad := int((score >> 8) & 0xFF)
		kicker := int(score & 0xFF)
		return rankToStr(quad) + " 포카드, " + rankToStr(kicker) + " Kicker"
	case 7:
		trip := int((score >> 8) & 0xFF)
		pair := int(score & 0xFF)
		return rankToStr(trip) + " 풀하우스 " + rankToStr(pair)
	case 6:
		top := int((score >> 16) & 0xF)
		return rankToStr(top) + " 하이 플러시"
	case 5:
		return rankToStr(int(score & 0xFFFF)) + " 스트레이트"
	case 4:
		trip := int((score >> 12) & 0xFF)
		k1 := int((score >> 8) & 0xF)
		return rankToStr(trip) + " 트리플, " + rankToStr(k1) + " Kicker"
	case 3:
		p1 := int((score >> 12) & 0xFF)
		p2 := int((score >> 8) & 0xFF)
		k := int(score & 0xFF)
		return rankToStr(p1) + "/" + rankToStr(p2) + " 투페어, " + rankToStr(k) + " Kicker"
	case 2:
		pair := int((score >> 12) & 0xFF)
		k1 := int((score >> 8) & 0xF)
		return rankToStr(pair) + " 원페어, " + rankToStr(k1) + " Kicker"
	case 1:
		top := int((score >> 16) & 0xF)
		return rankToStr(top) + " Kicker"
	}
	return ""
}

// EvaluateHandHighCardIdx는 7장 카드에서 하이카드 족보일 때 톱 카드의 인덱스(0~6)를 반환합니다.
// 하이카드가 아니면 -1을 반환합니다.
func EvaluateHandHighCardIdx(cards []Card) int {
	if len(cards) < 5 {
		return -1
	}
	score := EvaluateHand(cards)
	if int(score>>20) != 1 {
		return -1
	}
	// 하이카드: 가장 높은 카드 찾기
	bestIdx := 0
	bestRank, bestSuit := cardRank(cards[0]), suitRank(cards[0])
	for i := 1; i < len(cards); i++ {
		r, s := cardRank(cards[i]), suitRank(cards[i])
		if r > bestRank || (r == bestRank && s > bestSuit) {
			bestRank, bestSuit = r, s
			bestIdx = i
		}
	}
	return bestIdx
}
