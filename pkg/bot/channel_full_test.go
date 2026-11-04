package bot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"madrigal-multibot/pkg/models"
)

func TestChannelFullStatusAndClose(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	receivedError := make(chan struct{}, 1)

	// Mock server: emits exact live log error packet, waits for client to process, then closes with 4005
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. Send exact server error packet
		errPkt := []byte(`{"t":"error","text":"Channel 4 is full. Please pick another channel."}`)
		_ = conn.WriteMessage(websocket.TextMessage, errPkt)

		// Deterministic wait: unblock only when client processed packet and reached channel_full status
		select {
		case <-receivedError:
		case <-time.After(2 * time.Second):
			t.Errorf("timed out waiting for client to process error packet")
			return
		}

		// 2. Immediately close with code 4005
		closeMsg := websocket.FormatCloseMessage(4005, "channel full")
		_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
	}))
	defer wsServer.Close()

	wsEndpoint := "ws" + strings.TrimPrefix(wsServer.URL, "http")

	var statusMu sync.Mutex
	statusHistory := make([]string, 0, 4)

	instance := NewBotInstance(models.BotProfile{
		ID:            "ch-full-bot",
		Name:          "tester",
		Server:        "Channel 4",
		CharacterName: "test_char",
	}, nil, func(id string, stats models.BotRuntimeStats, status, statusMsg string) {
		statusMu.Lock()
		statusHistory = append(statusHistory, status)
		statusMu.Unlock()

		if status == "channel_full" {
			select {
			case receivedError <- struct{}{}:
			default:
			}
		}
	}, nil, nil)

	if err := instance.connectWebSocket(context.Background(), wsEndpoint); err != nil {
		t.Fatalf("failed to connect websocket: %v", err)
	}

	instance.mu.Lock()
	instance.Status = "online"
	instance.StatusMessage = "Session active"
	instance.cancelCh = make(chan struct{})
	instance.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		instance.runLoop()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("runLoop did not exit after WS close")
	}

	summary := instance.Summary()
	if summary.Status != "channel_full" {
		t.Fatalf("expected status 'channel_full', got '%s'", summary.Status)
	}
	if summary.StatusMessage != "Channel is Full" {
		t.Fatalf("expected status message 'Channel is Full', got '%s'", summary.StatusMessage)
	}

	// Verify no reconnect was attempted
	if instance.ws.IsConnected() {
		t.Fatalf("expected websocket to remain disconnected, but was connected")
	}

	statusMu.Lock()
	defer statusMu.Unlock()
	for _, st := range statusHistory {
		if st == "connecting" {
			t.Fatalf("unexpected reconnect transition to 'connecting' found in history: %v", statusHistory)
		}
	}
}
