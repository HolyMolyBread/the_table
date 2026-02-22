package main

import (
	"io"
	"log"
	"testing"
)

func init() {
	log.SetOutput(io.Discard)
}

func TestNewRoomManager(t *testing.T) {
	m := NewRoomManager()
	if m == nil {
		t.Fatal("NewRoomManager returned nil")
	}
}

func TestGetOrCreateRoom(t *testing.T) {
	m := NewRoomManager()

	room1 := m.getOrCreateRoom("chat_room1")
	if room1 == nil {
		t.Fatal("getOrCreateRoom returned nil")
	}
	if room1.ID != "chat_room1" {
		t.Errorf("room.ID = %q, want chat_room1", room1.ID)
	}

	room2 := m.getOrCreateRoom("chat_room1")
	if room1 != room2 {
		t.Error("getOrCreateRoom should return same room for same ID")
	}

	room3 := m.getOrCreateRoom("chat_room2")
	if room1 == room3 {
		t.Error("getOrCreateRoom should return different room for different ID")
	}
}

func TestJoinRoomAndLeaveRoom(t *testing.T) {
	m := NewRoomManager()

	client := &Client{
		manager:   m,
		conn:      nil,
		send:      nil,
		UserID:    "test_user",
		RoomID:    "",
		IsBot:     true,
		BotProcess: func([]byte) {},
	}

	m.JoinRoom("lobby_test1", "test_user", client)

	if client.RoomID != "lobby_test1" {
		t.Errorf("JoinRoom: client.RoomID = %q, want lobby_test1", client.RoomID)
	}

	room := m.getOrCreateRoom("lobby_test1")
	if room.count() != 1 {
		t.Errorf("room count after join = %d, want 1", room.count())
	}

	m.leaveRoom(client)

	if client.RoomID != "" {
		t.Errorf("leaveRoom: client.RoomID = %q, want empty", client.RoomID)
	}

	m.mu.RLock()
	_, exists := m.rooms["lobby_test1"]
	m.mu.RUnlock()
	if exists {
		t.Error("empty room should be deleted after leaveRoom")
	}
}

func TestLeaveRoomWhenEmptyRoomDeleted(t *testing.T) {
	m := NewRoomManager()

	c1 := &Client{
		manager:   m,
		UserID:    "user1",
		RoomID:    "",
		IsBot:     true,
		BotProcess: func([]byte) {},
	}
	c2 := &Client{
		manager:   m,
		UserID:    "user2",
		RoomID:    "",
		IsBot:     true,
		BotProcess: func([]byte) {},
	}

	m.JoinRoom("lobby_multi", "user1", c1)
	m.JoinRoom("lobby_multi", "user2", c2)

	if room := m.getOrCreateRoom("lobby_multi"); room.count() != 2 {
		t.Errorf("room count = %d, want 2", room.count())
	}

	m.leaveRoom(c1)
	if room := m.getOrCreateRoom("lobby_multi"); room.count() != 1 {
		t.Errorf("after c1 leave: room count = %d, want 1", room.count())
	}

	m.leaveRoom(c2)
	m.mu.RLock()
	_, exists := m.rooms["lobby_multi"]
	m.mu.RUnlock()
	if exists {
		t.Error("room should be deleted when last client leaves")
	}
}

func TestJoinRoomSwitchesRoom(t *testing.T) {
	m := NewRoomManager()

	client := &Client{
		manager:   m,
		UserID:    "switcher",
		RoomID:    "",
		IsBot:     true,
		BotProcess: func([]byte) {},
	}

	m.JoinRoom("room_a", "switcher", client)
	if client.RoomID != "room_a" {
		t.Errorf("client.RoomID = %q, want room_a", client.RoomID)
	}

	m.JoinRoom("room_b", "switcher", client)
	if client.RoomID != "room_b" {
		t.Errorf("client.RoomID = %q, want room_b", client.RoomID)
	}

	m.mu.RLock()
	_, existsA := m.rooms["room_a"]
	_, existsB := m.rooms["room_b"]
	m.mu.RUnlock()

	if existsA {
		t.Error("room_a should be deleted (client left for room_b)")
	}
	if !existsB {
		t.Error("room_b should exist")
	}
}
