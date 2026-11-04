package movement

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockMoveBot struct {
	id     string
	name   string
	online bool
	dx     float32
	dz     float32
	stop   bool
	mu     sync.Mutex
}

func (m *mockMoveBot) ID() string   { return m.id }
func (m *mockMoveBot) Name() string { return m.name }
func (m *mockMoveBot) Online() bool { return m.online }
func (m *mockMoveBot) Raw() any    { return m }

func (m *mockMoveBot) Move(dx, dz float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dx = dx
	m.dz = dz
}

func (m *mockMoveBot) StopMove() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop = true
}

type moveProvider struct {
	bots map[string]*mockMoveBot
}

func (p *moveProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}
func (p *moveProvider) ByName(name string) (multi.Bot, bool) { return nil, false }
func (p *moveProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestMovementModule_Commands(t *testing.T) {
	b1 := &mockMoveBot{id: "bot-1", name: "Bot1", online: true}
	b2 := &mockMoveBot{id: "bot-2", name: "Bot2", online: true}
	provider := &moveProvider{bots: map[string]*mockMoveBot{"bot-1": b1, "bot-2": b2}}

	d := multi.NewDispatcher(multi.Deps{
		Bots:    provider,
		Workers: 4,
	})

	reg := d.Registry()
	reg.RegisterCommand("main.movement.", multi.CommandDef{
		Key:   "main.movement.move",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				Dx float32 `json:"dx"`
				Dz float32 `json:"dz"`
			}
			_ = json.Unmarshal(payload, &p)
			m := b.(*mockMoveBot)
			m.Move(p.Dx, p.Dz)
			return nil, nil
		},
	})

	reg.RegisterCommand("main.movement.", multi.CommandDef{
		Key:   "main.movement.stop",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			m := b.(*mockMoveBot)
			m.StopMove()
			return nil, nil
		},
	})

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// 1. Move
	movePayload, _ := json.Marshal(map[string]float32{"dx": 1.5, "dz": -2.0})
	res, _ := d.RunCommand(ctx, target, "main.movement.move", movePayload, multi.Opts{})
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK on move, got %d", res.Summary.OK)
	}
	if b1.dx != 1.5 || b1.dz != -2.0 || b2.dx != 1.5 || b2.dz != -2.0 {
		t.Fatalf("bots failed to receive vector: b1=(%v,%v) b2=(%v,%v)", b1.dx, b1.dz, b2.dx, b2.dz)
	}

	// 2. Stop Move
	res, _ = d.RunCommand(ctx, target, "main.movement.stop", nil, multi.Opts{})
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK on stop move, got %d", res.Summary.OK)
	}
	if !b1.stop || !b2.stop {
		t.Fatalf("bots failed to stop: b1=%v b2=%v", b1.stop, b2.stop)
	}
}
