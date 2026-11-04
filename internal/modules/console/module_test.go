package console

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockChatBot struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	chats  []string
}

func (m *mockChatBot) ID() string   { return m.id }
func (m *mockChatBot) Name() string { return m.name }
func (m *mockChatBot) Online() bool { return m.online }
func (m *mockChatBot) Raw() any     { return m }

func (m *mockChatBot) SendChat(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chats = append(m.chats, text)
	return nil
}

type chatProvider struct {
	bots map[string]*mockChatBot
}

func (p *chatProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *chatProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *chatProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestConsoleModule_SendChat(t *testing.T) {
	b1 := &mockChatBot{id: "bot-1", name: "Bot1", online: true}
	b2 := &mockChatBot{id: "bot-2", name: "Bot2", online: true}
	p := &chatProvider{bots: map[string]*mockChatBot{"bot-1": b1, "bot-2": b2}}

	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// Valid chat message
	payload, _ := json.Marshal(SendChatPayload{Message: "!Hello Madrigal"})
	res, err := d.RunCommand(ctx, target, "console.chat.send", payload, multi.Opts{})
	if err != nil || res.Summary.OK != 2 {
		t.Fatalf("RunCommand failed: err=%v, res=%+v", err, res.Summary)
	}

	if len(b1.chats) != 1 || b1.chats[0] != "!Hello Madrigal" {
		t.Fatalf("b1 chat mismatch: %+v", b1.chats)
	}
	if len(b2.chats) != 1 || b2.chats[0] != "!Hello Madrigal" {
		t.Fatalf("b2 chat mismatch: %+v", b2.chats)
	}

	// Empty chat message (should fail validation / skipped)
	emptyPayload, _ := json.Marshal(SendChatPayload{Message: "   "})
	resEmpty, _ := d.RunCommand(ctx, target, "console.chat.send", emptyPayload, multi.Opts{})
	if resEmpty.Summary.Skipped != 2 {
		t.Fatalf("expected 2 skipped for empty chat, got: %+v", resEmpty.Summary)
	}
}
