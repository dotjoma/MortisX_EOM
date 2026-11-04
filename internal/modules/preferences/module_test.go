package preferences

import (
	"context"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/models"
)

type mockBotWithPrefs struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	prefs  models.BotPreferences
}

func (m *mockBotWithPrefs) ID() string   { return m.id }
func (m *mockBotWithPrefs) Name() string { return m.name }
func (m *mockBotWithPrefs) Online() bool { return m.online }
func (m *mockBotWithPrefs) Raw() any    { return m }

func (m *mockBotWithPrefs) GetPreferences() models.BotPreferences {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prefs
}

func (m *mockBotWithPrefs) MutatePreferences(mutate func(p *models.BotPreferences)) models.BotPreferences {
	m.mu.Lock()
	defer m.mu.Unlock()
	mutate(&m.prefs)
	return m.prefs
}

type mockProvider struct {
	bots map[string]*mockBotWithPrefs
}

func (p *mockProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *mockProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *mockProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestPreferencesModule_MetadataAndSchema(t *testing.T) {
	mod := New()
	if mod.Tab() != "Main" {
		t.Fatalf("expected Tab 'Main', got %q", mod.Tab())
	}
	if mod.ID() != "preferences" {
		t.Fatalf("expected ID 'preferences', got %q", mod.ID())
	}
	if mod.Title() != "Preferences" {
		t.Fatalf("expected Title 'Preferences', got %q", mod.Title())
	}

	d := multi.NewDispatcher(multi.Deps{
		Workers: 4,
	})

	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("failed to register preferences module: %v", err)
	}

	schema := d.GetSchema()
	found := false
	for _, m := range schema.Modules {
		if m.ID == "preferences" && m.Tab == "Main" {
			found = true
			if len(m.Fields) == 0 {
				t.Fatalf("expected module fields in schema, got 0")
			}
			break
		}
	}
	if !found {
		t.Fatalf("preferences module not found in schema")
	}
}

func TestPreferencesModule_Validation(t *testing.T) {
	// Valid
	validCfg := models.DefaultJobProgressionConfig()
	if err := models.ValidateJobProgressionConfig(validCfg); err != nil {
		t.Fatalf("expected valid default config, got err: %v", err)
	}

	// Invalid 2nd job
	invalidJob := validCfg
	invalidJob.Target1stJob = "acrobat"
	invalidJob.Target2ndJob = "blade"
	if err := models.ValidateJobProgressionConfig(invalidJob); err == nil {
		t.Fatalf("expected error for acrobat -> blade")
	}

	// Invalid ratio sum
	invalidRatio := validCfg
	invalidRatio.StatMode = models.StatModeRatio
	invalidRatio.CustomRatio.StrPercent = 50
	invalidRatio.CustomRatio.IntPercent = 40 // sum 90
	if err := models.ValidateJobProgressionConfig(invalidRatio); err == nil {
		t.Fatalf("expected error for ratio sum != 100")
	}

	// Invalid threshold cap
	invalidCap := validCfg
	invalidCap.StatMode = models.StatModeThreshold
	invalidCap.CustomRatio.CapStat = "dex"
	invalidCap.CustomRatio.ThenStat = "dex"
	if err := models.ValidateJobProgressionConfig(invalidCap); err == nil {
		t.Fatalf("expected error for cap_stat == then_stat")
	}

	invalidCapVal := validCfg
	invalidCapVal.StatMode = models.StatModeThreshold
	invalidCapVal.CustomRatio.CapStat = "dex"
	invalidCapVal.CustomRatio.ThenStat = "str"
	invalidCapVal.CustomRatio.CapValue = 10 // < 15
	if err := models.ValidateJobProgressionConfig(invalidCapVal); err == nil {
		t.Fatalf("expected error for cap_value < 15")
	}
}

func TestPreferencesModule_DispatcherIntegration(t *testing.T) {
	b1 := &mockBotWithPrefs{
		id:     "bot-1",
		name:   "BotOne",
		online: true,
		prefs: models.BotPreferences{
			Invisible: false,
			LegitMode: false,
			JobConfig: models.DefaultJobProgressionConfig(),
		},
	}
	b2 := &mockBotWithPrefs{
		id:     "bot-2",
		name:   "BotTwo",
		online: true,
		prefs: models.BotPreferences{
			Invisible: false,
			LegitMode: false,
			JobConfig: models.DefaultJobProgressionConfig(),
		},
	}

	provider := &mockProvider{
		bots: map[string]*mockBotWithPrefs{"bot-1": b1, "bot-2": b2},
	}

	d := multi.NewDispatcher(multi.Deps{
		Bots:    provider,
		Workers: 4,
	})

	// To register for mockBotWithPrefs, use a config spec targeting mockBotWithPrefs
	spec := multi.ConfigSpec{
		Prefix:  "main.preferences.",
		Target:  &models.BotPreferences{},
		Persist: true,
		Get: func(b multi.Bot) any {
			mock := b.(*mockBotWithPrefs)
			p := mock.GetPreferences()
			return &p
		},
		Update: func(b multi.Bot, mutate func(cfg any)) error {
			mock := b.(*mockBotWithPrefs)
			mock.MutatePreferences(func(p *models.BotPreferences) {
				mutate(p)
			})
			return nil
		},
		Validate: func(old, new any) error {
			p := new.(*models.BotPreferences)
			return models.ValidateJobProgressionConfig(p.JobConfig)
		},
	}
	d.RegisterConfig(spec)

	ctx := context.Background()

	// 1. Single setting apply: invisible -> true
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	res, err := d.ApplySetting(ctx, target, "main.preferences.invisible", true, multi.Opts{})
	if err != nil || res.Error != "" {
		t.Fatalf("ApplySetting failed: %v, res: %+v", err, res)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d", res.Summary.OK)
	}
	if !b1.GetPreferences().Invisible || !b2.GetPreferences().Invisible {
		t.Fatalf("expected both bots invisible = true")
	}

	// 2. ReadSettings: verify not mixed
	readRes, err := d.ReadSettings(ctx, target, []string{"main.preferences.invisible"})
	if err != nil {
		t.Fatalf("ReadSettings failed: %v", err)
	}
	mv := readRes["main.preferences.invisible"]
	if mv.Mixed || mv.Value != true {
		t.Fatalf("expected mixed=false, value=true, got %+v", mv)
	}

	// 3. Batch apply settings
	batch := map[string]any{
		"main.preferences.legit_mode": true,
		"main.preferences.move_step":  2.5,
	}
	batchRes, err := d.ApplySettings(ctx, target, batch, multi.Opts{})
	if err != nil || batchRes.Error != "" {
		t.Fatalf("ApplySettings failed: %v, res: %+v", err, batchRes)
	}
	if batchRes.Summary.OK != 2 {
		t.Fatalf("expected 2 OK in batch, got %d", batchRes.Summary.OK)
	}
	if !b1.GetPreferences().LegitMode || b1.GetPreferences().MoveStep != float32(2.5) {
		t.Fatalf("bot-1 failed to receive batch settings: %+v", b1.GetPreferences())
	}
	if !b2.GetPreferences().LegitMode || b2.GetPreferences().MoveStep != float32(2.5) {
		t.Fatalf("bot-2 failed to receive batch settings: %+v", b2.GetPreferences())
	}

	// 4. Auto Reconnect Delay validation and application
	resDelay, err := d.ApplySetting(ctx, target, "main.preferences.auto_reconnect_delay", 30, multi.Opts{})
	if err != nil || resDelay.Error != "" {
		t.Fatalf("ApplySetting auto_reconnect_delay failed: %v, res: %+v", err, resDelay)
	}
	if b1.GetPreferences().AutoReconnectDelay != 30 || b2.GetPreferences().AutoReconnectDelay != 30 {
		t.Fatalf("expected delay 30, got %d and %d", b1.GetPreferences().AutoReconnectDelay, b2.GetPreferences().AutoReconnectDelay)
	}

	// Range checks: min 1, max 3600
	resMin, _ := d.ApplySetting(ctx, target, "main.preferences.auto_reconnect_delay", 0, multi.Opts{})
	if resMin.Summary.Skipped != 2 {
		t.Fatalf("expected 0 to fail min validation, got: %+v", resMin.Summary)
	}

	resMax, _ := d.ApplySetting(ctx, target, "main.preferences.auto_reconnect_delay", 5000, multi.Opts{})
	if resMax.Summary.Skipped != 2 {
		t.Fatalf("expected 5000 to fail max validation, got: %+v", resMax.Summary)
	}
}
