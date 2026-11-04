package botinfo

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockBotInstance struct {
	id         string
	name       string
	online     bool
	started    bool
	stopped    bool
	target     string
	warped     string
	job        string
	server     string
	mu         sync.Mutex
}

func (m *mockBotInstance) ID() string   { return m.id }
func (m *mockBotInstance) Name() string { return m.name }
func (m *mockBotInstance) Online() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.online
}
func (m *mockBotInstance) Raw() any { return m }

func (m *mockBotInstance) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.online = true
	return nil
}

func (m *mockBotInstance) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.online = false
	return nil
}

func (m *mockBotInstance) SetFollowTarget(t string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.target = t
}

func (m *mockBotInstance) Warp(t string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warped = t
	return nil
}

func (m *mockBotInstance) AdvanceJob(j string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.job = j
	return nil, nil
}

func (m *mockBotInstance) UpdateServer(s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.server = s
	return nil
}

type testBotProvider struct {
	bots map[string]*mockBotInstance
}

func (p *testBotProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *testBotProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *testBotProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestBotInfoModule_Commands(t *testing.T) {
	b1 := &mockBotInstance{id: "bot-1", name: "Alice", online: false}
	b2 := &mockBotInstance{id: "bot-2", name: "Bob", online: true}
	provider := &testBotProvider{bots: map[string]*mockBotInstance{"bot-1": b1, "bot-2": b2}}

	d := multi.NewDispatcher(multi.Deps{
		Bots:    provider,
		Workers: 4,
	})

	// Register commands using mockBotInstance
	reg := d.Registry()
	reg.RegisterCommand("main.bot.", multi.CommandDef{
		Key:          "main.bot.connect",
		Scope:        multi.AllSelected,
		AllowOffline: true,
		Mode:         multi.Parallel,
		CanRun: func(b multi.Bot) error {
			if b.Online() {
				return errors.New("already connected")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			m := b.(*mockBotInstance)
			return nil, m.Start()
		},
	})

	reg.RegisterCommand("main.bot.", multi.CommandDef{
		Key:   "main.bot.disconnect",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		CanRun: func(b multi.Bot) error {
			if !b.Online() {
				return errors.New("already disconnected")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			m := b.(*mockBotInstance)
			return nil, m.Stop()
		},
	})

	reg.RegisterCommand("main.bot.", multi.CommandDef{
		Key:   "main.bot.follow",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct{ Target string `json:"target"` }
			_ = json.Unmarshal(payload, &p)
			if p.Target != "" && p.Target == b.Name() {
				return errors.New("cannot follow self")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct{ Target string `json:"target"` }
			_ = json.Unmarshal(payload, &p)
			m := b.(*mockBotInstance)
			m.SetFollowTarget(p.Target)
			return nil, nil
		},
	})

	reg.RegisterCommand("main.bot.", multi.CommandDef{
		Key:   "main.bot.warp",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct{ Target string `json:"target"` }
			_ = json.Unmarshal(payload, &p)
			if p.Target == b.Name() {
				return errors.New("cannot warp to self")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct{ Target string `json:"target"` }
			_ = json.Unmarshal(payload, &p)
			m := b.(*mockBotInstance)
			return nil, m.Warp(p.Target)
		},
	})

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// 1. Connect: bot-1 starts, bot-2 skipped (already connected)
	res, _ := d.RunCommand(ctx, target, "main.bot.connect", nil, multi.Opts{})
	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK, 1 Skipped on connect; got OK=%d, Skipped=%d", res.Summary.OK, res.Summary.Skipped)
	}
	if !b1.started {
		t.Fatalf("expected bot-1 started")
	}

	// 2. Disconnect: both are online now, both disconnect
	res, _ = d.RunCommand(ctx, target, "main.bot.disconnect", nil, multi.Opts{})
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK on disconnect; got OK=%d, Skipped=%d", res.Summary.OK, res.Summary.Skipped)
	}

	// 3. Follow self check: target "Alice"
	b1.online = true
	b2.online = true
	followPayload, _ := json.Marshal(map[string]string{"target": "Alice"})
	res, _ = d.RunCommand(ctx, target, "main.bot.follow", followPayload, multi.Opts{})
	// bot-1 is Alice (skipped self), bot-2 is Bob (OK)
	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK, 1 Skipped on follow self; got OK=%d, Skipped=%d", res.Summary.OK, res.Summary.Skipped)
	}

	// 4. Warp self check: target "Bob"
	warpPayload, _ := json.Marshal(map[string]string{"target": "Bob"})
	res, _ = d.RunCommand(ctx, target, "main.bot.warp", warpPayload, multi.Opts{})
	// bot-2 is Bob (skipped self), bot-1 is Alice (OK)
	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK, 1 Skipped on warp self; got OK=%d, Skipped=%d", res.Summary.OK, res.Summary.Skipped)
	}
}
