package farm

import (
	"context"
	"sync"
	"testing"
	"time"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/models"
)

type mockFarmBot struct {
	id      string
	name    string
	online  bool
	mu      sync.Mutex
	cfg     models.FarmConfig
	running bool
	x, y, z float32
}

func (m *mockFarmBot) ID() string     { return m.id }
func (m *mockFarmBot) Name() string   { return m.name }
func (m *mockFarmBot) Online() bool   { return m.online }
func (m *mockFarmBot) Raw() any       { return m }

func (m *mockFarmBot) GetFarmConfig() models.FarmConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *mockFarmBot) SetFarmConfig(cfg models.FarmConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	return nil
}

func (m *mockFarmBot) MutateFarmConfig(mutate func(c *models.FarmConfig)) models.FarmConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	mutate(&m.cfg)
	return m.cfg
}

func (m *mockFarmBot) StartFarm() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = true
	return nil
}

func (m *mockFarmBot) StopFarm() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return nil
}

func (m *mockFarmBot) IsFarmRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *mockFarmBot) GetPosition() (float32, float32, float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.x, m.y, m.z
}

type mockBotProvider struct {
	bots map[string]*mockFarmBot
}

func (p *mockBotProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *mockBotProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *mockBotProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

type noopEmitter struct{}

func (noopEmitter) Emit(event string, payload any) {}

func setupDispatcher(bots map[string]*mockFarmBot) (*multi.Dispatcher, error) {
	d := multi.NewDispatcher(multi.Deps{
		Bots:      &mockBotProvider{bots: bots},
		Emitter:   noopEmitter{},
		Persister: multi.NewDebouncedPersister(10*time.Millisecond, func(b multi.Bot) error { return nil }, multi.RealClock{}),
		Workers:   4,
		Clock:     multi.RealClock{},
	})
	err := d.RegisterModule(New())
	return d, err
}

func TestFarmModule_StartStopIdempotent(t *testing.T) {
	b1 := &mockFarmBot{id: "bot-1", name: "Bot1", online: true}
	b2 := &mockFarmBot{id: "bot-2", name: "Bot2", online: true}
	bots := map[string]*mockFarmBot{"bot-1": b1, "bot-2": b2}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// 1. Start Farm
	res, err := d.RunCommand(context.Background(), target, "automation.farm.start", nil, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand start failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}
	if !b1.IsFarmRunning() || !b2.IsFarmRunning() {
		t.Fatalf("expected both bots running")
	}

	// 2. Start again (idempotent)
	res, err = d.RunCommand(context.Background(), target, "automation.farm.start", nil, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand start 2 failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK on idempotent start, got %d", res.Summary.OK)
	}

	// 3. Stop Farm
	res, err = d.RunCommand(context.Background(), target, "automation.farm.stop", nil, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand stop failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}
	if b1.IsFarmRunning() || b2.IsFarmRunning() {
		t.Fatalf("expected bots stopped")
	}
}

func TestFarmModule_SetAnchorPerBot(t *testing.T) {
	b1 := &mockFarmBot{id: "bot-1", name: "Bot1", online: true, x: 100, y: 50, z: 200}
	b2 := &mockFarmBot{id: "bot-2", name: "Bot2", online: true, x: 300, y: 60, z: 400}
	bots := map[string]*mockFarmBot{"bot-1": b1, "bot-2": b2}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	res, err := d.RunCommand(context.Background(), target, "automation.farm.set_anchor", nil, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand set_anchor failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}

	cfg1 := b1.GetFarmConfig()
	if cfg1.AnchorX != 100 || cfg1.AnchorZ != 200 {
		t.Errorf("bot-1 anchor mismatch: got (%v, %v), want (100, 200)", cfg1.AnchorX, cfg1.AnchorZ)
	}

	cfg2 := b2.GetFarmConfig()
	if cfg2.AnchorX != 300 || cfg2.AnchorZ != 400 {
		t.Errorf("bot-2 anchor mismatch: got (%v, %v), want (300, 400)", cfg2.AnchorX, cfg2.AnchorZ)
	}
}

func TestFarmModule_ConfigSpecSettings(t *testing.T) {
	b1 := &mockFarmBot{id: "bot-1", name: "Bot1", online: true}
	b2 := &mockFarmBot{id: "bot-2", name: "Bot2", online: true}
	bots := map[string]*mockFarmBot{"bot-1": b1, "bot-2": b2}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// Apply setting: radius
	res, err := d.ApplySetting(context.Background(), target, "automation.farm.radius", float32(45.0), multi.Opts{})
	if err != nil {
		t.Fatalf("ApplySetting radius failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}
	if b1.GetFarmConfig().Radius != 45.0 || b2.GetFarmConfig().Radius != 45.0 {
		t.Fatalf("expected radius 45.0 on both bots")
	}

	// Apply setting with Risk=High without confirm should trigger confirm gate on N>1
	res, err = d.ApplySetting(context.Background(), target, "automation.farm.exploit_god_mode", true, multi.Opts{Confirm: false})
	if err != nil {
		t.Fatalf("expected confirmation required as value, got err: %v", err)
	}
	if res.Confirm == nil {
		t.Fatalf("expected res.Confirm != nil for Risk=High with N>1")
	}

	// Apply with Confirm: true
	res, err = d.ApplySetting(context.Background(), target, "automation.farm.exploit_god_mode", true, multi.Opts{Confirm: true})
	if err != nil {
		t.Fatalf("ApplySetting with confirm failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}
	if !b1.GetFarmConfig().ExploitGodMode || !b2.GetFarmConfig().ExploitGodMode {
		t.Fatalf("expected exploit_god_mode true on both bots")
	}
}

func TestFarmModule_StartWithConfigPayloadPropagatesToAllSelectedBots(t *testing.T) {
	b1 := &mockFarmBot{id: "bot-1", name: "Bot1", online: true, x: 10, y: 0, z: 20}
	b2 := &mockFarmBot{id: "bot-2", name: "Bot2", online: true, x: 30, y: 0, z: 40}
	b3 := &mockFarmBot{id: "bot-3", name: "Bot3", online: true, x: 50, y: 0, z: 60}
	bots := map[string]*mockFarmBot{"bot-1": b1, "bot-2": b2, "bot-3": b3}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	// Select bot-1 and bot-2, leave bot-3 unselected
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// Custom FarmConfig to apply
	payload := []byte(`{
		"mode": "hunt",
		"radius": 55.0,
		"smart_mode": true,
		"avoid_kill_steal": true,
		"evacuate_on_player_nearby": true,
		"player_detection_radius": 75.0,
		"ghost_mode_block_rec_put": true,
		"auto_potions": true,
		"auto_loot": true,
		"stop_when_bag_full": true,
		"auto_lucky": true,
		"exploit_one_kill": true,
		"exploit_multi_sweep": true,
		"exploit_god_mode": true,
		"exploit_cap_hp1": true,
		"discord_webhook_url": "https://discord.com/api/webhooks/test",
		"webhook_alert_on_player": true,
		"webhook_alert_on_death": true,
		"webhook_alert_on_rare_drop": true,
		"webhook_alert_on_bag_full": true
	}`)

	res, err := d.RunCommand(context.Background(), target, "automation.farm.start", payload, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand start failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}

	// Verify both selected bots received the EXACT configured settings
	for _, b := range []*mockFarmBot{b1, b2} {
		cfg := b.GetFarmConfig()
		if !b.IsFarmRunning() {
			t.Errorf("expected bot %s to be running", b.ID())
		}
		if cfg.Radius != 55.0 {
			t.Errorf("bot %s: got Radius %v, want 55.0", b.ID(), cfg.Radius)
		}
		if !cfg.SmartMode {
			t.Errorf("bot %s: got SmartMode false, want true", b.ID())
		}
		if !cfg.ExploitOneKill {
			t.Errorf("bot %s: got ExploitOneKill false, want true", b.ID())
		}
		if !cfg.ExploitGodMode {
			t.Errorf("bot %s: got ExploitGodMode false, want true", b.ID())
		}
		if cfg.DiscordWebhookURL != "https://discord.com/api/webhooks/test" {
			t.Errorf("bot %s: got Webhook URL %s, want https://discord.com/api/webhooks/test", b.ID(), cfg.DiscordWebhookURL)
		}
	}

	// Verify unselected bot-3 is untouched
	if b3.IsFarmRunning() {
		t.Errorf("unselected bot-3 should not be running")
	}
	if b3.GetFarmConfig().Radius != 0 {
		t.Errorf("unselected bot-3 config should be untouched, got Radius %v", b3.GetFarmConfig().Radius)
	}
}
