package mapnav

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockNavBot struct {
	id      string
	name    string
	online  bool
	mu      sync.Mutex
	wpX     float32
	wpZ     float32
	cleared bool
	warped  string
}

func (m *mockNavBot) ID() string   { return m.id }
func (m *mockNavBot) Name() string { return m.name }
func (m *mockNavBot) Online() bool { return m.online }
func (m *mockNavBot) Raw() any     { return m }

func (m *mockNavBot) SetWaypoint(x, z float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wpX = x
	m.wpZ = z
	m.cleared = false
}

func (m *mockNavBot) ClearWaypoint() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleared = true
}

func (m *mockNavBot) Warp(target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warped = target
	return nil
}

type navProvider struct {
	bots map[string]*mockNavBot
}

func (p *navProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *navProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *navProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestMapNavModule_Commands(t *testing.T) {
	b1 := &mockNavBot{id: "bot-1", name: "Bot1", online: true}
	b2 := &mockNavBot{id: "bot-2", name: "Bot2", online: true}
	p := &navProvider{bots: map[string]*mockNavBot{"bot-1": b1, "bot-2": b2}}

	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// 1. Set Waypoint
	setPayload, _ := json.Marshal(CoordsPayload{X: 1234.5, Z: 5678.9})
	res1, err := d.RunCommand(ctx, target, "map.nav.waypoint_set", setPayload, multi.Opts{})
	if err != nil || res1.Summary.OK != 2 {
		t.Fatalf("Set waypoint failed: err=%v, res=%+v", err, res1.Summary)
	}
	if b1.wpX != 1234.5 || b1.wpZ != 5678.9 || b2.wpX != 1234.5 || b2.wpZ != 5678.9 {
		t.Fatalf("Coordinates not set properly on bots")
	}

	// 2. Clear Waypoint
	res2, err := d.RunCommand(ctx, target, "map.nav.waypoint_clear", nil, multi.Opts{})
	if err != nil || res2.Summary.OK != 2 {
		t.Fatalf("Clear waypoint failed: err=%v, res=%+v", err, res2.Summary)
	}
	if !b1.cleared || !b2.cleared {
		t.Fatalf("Waypoints were not marked cleared")
	}

	// 3. Warp to Coords
	warpPayload, _ := json.Marshal(CoordsPayload{X: 999.0, Z: 888.0})
	res3, err := d.RunCommand(ctx, target, "map.nav.warp", warpPayload, multi.Opts{})
	if err != nil || res3.Summary.OK != 2 {
		t.Fatalf("Warp failed: err=%v, res=%+v", err, res3.Summary)
	}
	if b1.warped != "999.0, 888.0" || b2.warped != "999.0, 888.0" {
		t.Fatalf("Warp coordinate string mismatch: b1=%s, b2=%s", b1.warped, b2.warped)
	}
}
