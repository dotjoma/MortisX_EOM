package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var chRegex = regexp.MustCompile(`(?i)channel\s*(\d+)`)

func ResolveWSEndpoint(server string) string {
	s := strings.TrimSpace(server)
	if strings.EqualFold(s, "mock") {
		return "mock"
	}
	if strings.HasPrefix(s, "ws://") || strings.HasPrefix(s, "wss://") {
		return s
	}

	matches := chRegex.FindStringSubmatch(s)
	chNum := 2
	if len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil && n > 0 {
			chNum = n
		}
	}
	return fmt.Sprintf("wss://echoesofmadrigal.com/ws?ch=%d", chNum)
}

type WSLoggerFunc func(direction string, payload []byte)

type GameWebSocket struct {
	mu        sync.Mutex
	conn      *websocket.Conn
	connected atomic.Bool
	endpoint  string
	logger    WSLoggerFunc
}

func NewGameWebSocket() *GameWebSocket {
	return &GameWebSocket{}
}

func (ws *GameWebSocket) SetLogger(fn WSLoggerFunc) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.logger = fn
}

func (ws *GameWebSocket) Connect(endpoint string) error {
	return ws.ConnectWithContext(context.Background(), endpoint)
}

func (ws *GameWebSocket) ConnectWithContext(ctx context.Context, endpoint string) error {
	return ws.ConnectWithDialer(ctx, endpoint, nil)
}

func (ws *GameWebSocket) ConnectWithDialer(ctx context.Context, endpoint string, networkDialer *net.Dialer) error {
	if endpoint == "mock" {
		ws.mu.Lock()
		ws.connected.Store(true)
		ws.endpoint = "mock"
		ws.mu.Unlock()
		return nil
	}

	wsDialer := websocket.Dialer{
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: false},
		HandshakeTimeout: 10 * time.Second,
	}
	if networkDialer != nil {
		wsDialer.NetDialContext = networkDialer.DialContext
	}

	headers := http.Header{
		"User-Agent": []string{"GodotEngine/4.5.1 (Windows)"},
	}

	var conn *websocket.Conn
	var lastErr error
	var lastStatus string

	for attempt := 1; attempt <= 3; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c, resp, err := wsDialer.DialContext(ctx, endpoint, headers)
		if err == nil {
			conn = c
			break
		}
		lastErr = err
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastStatus = fmt.Sprintf(" (HTTP %d %s: %s)", resp.StatusCode, resp.Status, strings.TrimSpace(string(body)))
		}
		if attempt < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}

	if conn == nil {
		if errors.Is(lastErr, context.Canceled) || ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("websocket dial failed to %s%s: %w", endpoint, lastStatus, lastErr)
	}

	// Abort if context cancelled while establishing connection
	select {
	case <-ctx.Done():
		_ = conn.Close()
		return ctx.Err()
	default:
	}

	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})

	ws.mu.Lock()
	if ws.conn != nil {
		_ = ws.conn.Close()
	}
	ws.conn = conn
	ws.connected.Store(true)
	ws.endpoint = endpoint
	ws.mu.Unlock()
	return nil
}

func (ws *GameWebSocket) Send(payload []byte) error {
	if !ws.connected.Load() {
		return errors.New("websocket not connected")
	}

	ws.mu.Lock()
	logger := ws.logger
	isMock := ws.endpoint == "mock"
	conn := ws.conn
	if !isMock && conn != nil {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	var err error
	if !isMock && conn != nil {
		err = conn.WriteMessage(websocket.TextMessage, payload)
	}
	if err != nil {
		ws.connected.Store(false)
	}
	ws.mu.Unlock()

	if logger != nil {
		logger("OUT", payload)
	}
	return err
}

func (ws *GameWebSocket) Read() ([]byte, error) {
	if !ws.connected.Load() {
		return nil, errors.New("websocket not connected")
	}

	ws.mu.Lock()
	conn := ws.conn
	isMock := ws.endpoint == "mock"
	logger := ws.logger
	ws.mu.Unlock()

	if isMock {
		time.Sleep(1 * time.Second)
		msg := []byte(`{"t":"mock_ping"}`)
		if logger != nil {
			logger("IN", msg)
		}
		return msg, nil
	}

	if conn == nil {
		return nil, errors.New("websocket connection is nil")
	}

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		ws.connected.Store(false)
		ws.mu.Lock()
		l := ws.logger
		ws.mu.Unlock()
		if l != nil {
			l("CLOSE", []byte(err.Error()))
		}
		return nil, err
	}

	if logger != nil {
		logger("IN", msg)
	}
	return msg, nil
}

func (ws *GameWebSocket) Close() error {
	if !ws.connected.Swap(false) {
		return nil
	}

	ws.mu.Lock()
	conn := ws.conn
	ws.conn = nil
	ws.mu.Unlock()

	if conn != nil {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(250*time.Millisecond))
		return conn.Close()
	}
	return nil
}

func (ws *GameWebSocket) IsConnected() bool {
	return ws.connected.Load()
}
