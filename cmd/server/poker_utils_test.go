package main

import (
	"testing"
)

func TestEvaluateHand_RoyalStraightFlush(t *testing.T) {
	cards := []Card{
		{Suit: "♥", Value: "10"}, {Suit: "♥", Value: "J"}, {Suit: "♥", Value: "Q"},
		{Suit: "♥", Value: "K"}, {Suit: "♥", Value: "A"}, {Suit: "♠", Value: "2"},
		{Suit: "♣", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 10 {
		t.Errorf("로얄 스트레이트 플러시: rank=%d, want 10", rank)
	}
	if name := HandRankName(score); name != "로티플" {
		t.Errorf("HandRankName: got %q, want 로티플", name)
	}
}

func TestEvaluateHand_StraightFlush(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "9"}, {Suit: "♠", Value: "10"}, {Suit: "♠", Value: "J"},
		{Suit: "♠", Value: "Q"}, {Suit: "♠", Value: "K"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 9 {
		t.Errorf("스트레이트 플러시: rank=%d, want 9", rank)
	}
}

func TestEvaluateHand_FourOfAKind(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "A"}, {Suit: "♥", Value: "A"}, {Suit: "♦", Value: "A"},
		{Suit: "♣", Value: "A"}, {Suit: "♠", Value: "K"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 8 {
		t.Errorf("포카드: rank=%d, want 8", rank)
	}
}

func TestEvaluateHand_FullHouse(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "K"}, {Suit: "♥", Value: "K"}, {Suit: "♦", Value: "K"},
		{Suit: "♠", Value: "Q"}, {Suit: "♥", Value: "Q"}, {Suit: "♦", Value: "2"},
		{Suit: "♣", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 7 {
		t.Errorf("풀하우스: rank=%d, want 7", rank)
	}
	if name := HandRankName(score); name != "풀하우스" {
		t.Errorf("HandRankName: got %q, want 풀하우스", name)
	}
}

func TestEvaluateHand_Flush(t *testing.T) {
	cards := []Card{
		{Suit: "♦", Value: "A"}, {Suit: "♦", Value: "10"}, {Suit: "♦", Value: "7"},
		{Suit: "♦", Value: "4"}, {Suit: "♦", Value: "2"}, {Suit: "♠", Value: "K"},
		{Suit: "♥", Value: "Q"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 6 {
		t.Errorf("플러시: rank=%d, want 6", rank)
	}
}

func TestEvaluateHand_Straight(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "10"}, {Suit: "♥", Value: "9"}, {Suit: "♦", Value: "8"},
		{Suit: "♣", Value: "7"}, {Suit: "♠", Value: "6"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 5 {
		t.Errorf("스트레이트: rank=%d, want 5", rank)
	}
}

func TestEvaluateHand_ThreeOfAKind(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "7"}, {Suit: "♥", Value: "7"}, {Suit: "♦", Value: "7"},
		{Suit: "♠", Value: "A"}, {Suit: "♥", Value: "K"}, {Suit: "♦", Value: "2"},
		{Suit: "♣", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 4 {
		t.Errorf("트리플: rank=%d, want 4", rank)
	}
}

func TestEvaluateHand_TwoPair(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "J"}, {Suit: "♥", Value: "J"}, {Suit: "♦", Value: "8"},
		{Suit: "♣", Value: "8"}, {Suit: "♠", Value: "A"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 3 {
		t.Errorf("투페어: rank=%d, want 3", rank)
	}
}

func TestEvaluateHand_OnePair(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "A"}, {Suit: "♥", Value: "A"}, {Suit: "♦", Value: "K"},
		{Suit: "♣", Value: "Q"}, {Suit: "♠", Value: "J"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 2 {
		t.Errorf("원페어: rank=%d, want 2", rank)
	}
}

func TestEvaluateHand_HighCard(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "A"}, {Suit: "♥", Value: "K"}, {Suit: "♦", Value: "Q"},
		{Suit: "♣", Value: "J"}, {Suit: "♠", Value: "9"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	score := EvaluateHand(cards)
	rank := int(score >> 20)
	if rank != 1 {
		t.Errorf("하이카드: rank=%d, want 1", rank)
	}
}

func TestEvaluateHand_RoyalBeatsStraightFlush(t *testing.T) {
	royal := []Card{
		{Suit: "♥", Value: "10"}, {Suit: "♥", Value: "J"}, {Suit: "♥", Value: "Q"},
		{Suit: "♥", Value: "K"}, {Suit: "♥", Value: "A"}, {Suit: "♠", Value: "2"},
		{Suit: "♣", Value: "3"},
	}
	sf := []Card{
		{Suit: "♠", Value: "9"}, {Suit: "♠", Value: "10"}, {Suit: "♠", Value: "J"},
		{Suit: "♠", Value: "Q"}, {Suit: "♠", Value: "K"}, {Suit: "♥", Value: "2"},
		{Suit: "♦", Value: "3"},
	}
	sRoyal := EvaluateHand(royal)
	sSf := EvaluateHand(sf)
	if sRoyal <= sSf {
		t.Errorf("로얄 플러시(%d) > 스트레이트 플러시(%d) 여야 함", sRoyal, sSf)
	}
}

func TestEvaluateHand_FewerThanFiveCards(t *testing.T) {
	cards := []Card{
		{Suit: "♠", Value: "A"}, {Suit: "♥", Value: "K"},
	}
	score := EvaluateHand(cards)
	if score != 0 {
		t.Errorf("5장 미만: score=%d, want 0", score)
	}
}
