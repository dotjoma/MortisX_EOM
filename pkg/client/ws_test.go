package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestGameWebSocket_ConnectAndRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"t":"welcome","id":100}`))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws := NewGameWebSocket()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := ws.ConnectWithContext(ctx, wsURL); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !ws.IsConnected() {
		t.Fatalf("expected ws to be connected")
	}

	msg, err := ws.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !strings.Contains(string(msg), "welcome") {
		t.Fatalf("unexpected message: %s", string(msg))
	}

	_ = ws.Close()
	if ws.IsConnected() {
		t.Fatalf("expected ws to be disconnected after Close()")
	}
}

func TestGameWebSocket_DialCancellation(t *testing.T) {
	// A server that hangs during handshake until client cancels
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	ws := NewGameWebSocket()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := ws.ConnectWithContext(ctx, wsURL)
	if err == nil {
		t.Fatalf("expected error on cancelled dial, got nil")
	}
	if ws.IsConnected() {
		t.Fatalf("expected ws not connected on cancelled dial")
	}
}

func TestGameWebSocket_MockEndpoint(t *testing.T) {
	ws := NewGameWebSocket()
	if err := ws.Connect("mock"); err != nil {
		t.Fatalf("Connect('mock') failed: %v", err)
	}
	if !ws.IsConnected() {
		t.Fatalf("expected mock ws to be connected")
	}
	_ = ws.Close()
	if ws.IsConnected() {
		t.Fatalf("expected mock ws to be disconnected")
	}
}
