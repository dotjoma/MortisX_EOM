package characterstats

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/models"
)

type mockStatBot struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	stats  models.BotRuntimeStats

	allocCalls []allocateCall
}

type allocateCall struct {
	AddStr   int
	AddSta   int
	AddDex   int
	AddInt   int
	AutoStat int
}

func (m *mockStatBot) ID() string   { return m.id }
func (m *mockStatBot) Name() string { return m.name }
func (m *mockStatBot) Online() bool { return m.online }
func (m *mockStatBot) Raw() any     { return m }

func (m *mockStatBot) Summary() models.BotSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return models.BotSummary{
		ID:     m.id,
		Name:   m.name,
		Status: "online",
		Stats:  m.stats,
	}
}

func (m *mockStatBot) AllocateStats(addStr, addSta, addDex, addInt, autoStat int) (*models.BotSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allocCalls = append(m.allocCalls, allocateCall{
		AddStr:   addStr,
		AddSta:   addSta,
		AddDex:   addDex,
		AddInt:   addInt,
		AutoStat: autoStat,
	})
	total := addStr + addSta + addDex + addInt
	m.stats.STR += uint32(addStr)
	m.stats.STA += uint32(addSta)
	m.stats.DEX += uint32(addDex)
	m.stats.INT += uint32(addInt)
	if m.stats.StatPoints >= uint32(total) {
		m.stats.StatPoints -= uint32(total)
	}
	if autoStat >= 0 {
		m.stats.AutoStat = uint32(autoStat)
	}
	return &models.BotSummary{
		ID:    m.id,
		Stats: m.stats,
	}, nil
}

type mockProvider struct {
	bots map[string]*mockStatBot
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

func TestCharacterStats_Allocate(t *testing.T) {
	bot1 := &mockStatBot{
		id:     "bot-1",
		name:   "Knight1",
		online: true,
		stats: models.BotRuntimeStats{
			STR:        15,
			STA:        15,
			DEX:        15,
			INT:        15,
			StatPoints: 10,
		},
	}
	bot2 := &mockStatBot{
		id:     "bot-2",
		name:   "Knight2",
		online: true,
		stats: models.BotRuntimeStats{
			STR:        15,
			STA:        15,
			DEX:        15,
			INT:        15,
			StatPoints: 2, // only 2 points
		},
	}

	p := &mockProvider{bots: map[string]*mockStatBot{"bot-1": bot1, "bot-2": bot2}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	payload, _ := json.Marshal(AllocatePayload{
		AddStr:   5,
		AddSta:   5,
		AddDex:   0,
		AddInt:   0,
		AutoStat: 1,
	})

	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	res, err := d.RunCommand(context.Background(), target, "main.stats.allocate", payload, multi.Opts{})
	if err != nil {
		t.Fatalf("unexpected RunCommand error: %v", err)
	}

	// bot-1 has 10 points (needs 10) -> OK
	// bot-2 has 2 points (needs 10) -> Skipped
	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK and 1 Skipped, got OK=%d Skipped=%d: %+v", res.Summary.OK, res.Summary.Skipped, res.Summary)
	}

	if bot1.stats.STR != 20 || bot1.stats.STA != 20 || bot1.stats.StatPoints != 0 {
		t.Fatalf("bot1 stats mismatch: %+v", bot1.stats)
	}
}

func TestCharacterStats_Autofill(t *testing.T) {
	botAcrobat := &mockStatBot{
		id:     "bot-acrobat",
		name:   "BowAcrobat",
		online: true,
		stats: models.BotRuntimeStats{
			Job:        "acrobat",
			STR:        15,
			STA:        15,
			DEX:        15,
			INT:        15,
			StatPoints: 40,
		},
	}

	p := &mockProvider{bots: map[string]*mockStatBot{"bot-acrobat": botAcrobat}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := &Module{
		loadStatsJson: func() (models.StatsJsonMap, error) {
			return models.StatsJsonMap{
				"acrobat": models.ClassBuildConfig{
					Job:         "acrobat",
					Mode:        "cap",
					CapStat:     "dex",
					CapValue:    43,
					ThenStat:    "str",
					AutoStat:    1,
					Description: "43 DEX then STR",
				},
			}, nil
		},
	}
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-acrobat"}, FocusedID: "bot-acrobat"}
	res, err := d.RunCommand(context.Background(), target, "main.stats.autofill", nil, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}

	if res.Summary.OK != 1 {
		t.Fatalf("expected 1 OK, got OK=%d Skipped=%d", res.Summary.OK, res.Summary.Skipped)
	}

	// Cur DEX is 15. Cap is 43. Needed = 28.
	// Out of 40 free points: 28 goes to DEX (DEX becomes 43), remaining 12 goes to STR (STR becomes 27).
	if botAcrobat.stats.DEX != 43 || botAcrobat.stats.STR != 27 || botAcrobat.stats.StatPoints != 0 {
		t.Fatalf("acrobat stats after autofill mismatch: DEX=%d, STR=%d, rem=%d",
			botAcrobat.stats.DEX, botAcrobat.stats.STR, botAcrobat.stats.StatPoints)
	}
}

func TestCharacterStats_SetAutoStat(t *testing.T) {
	bot1 := &mockStatBot{
		id:     "bot-1",
		name:   "Mage",
		online: true,
		stats: models.BotRuntimeStats{
			AutoStat: 0,
		},
	}

	p := &mockProvider{bots: map[string]*mockStatBot{"bot-1": bot1}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	payload, _ := json.Marshal(AutoStatPayload{AutoStat: 4}) // INT
	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}
	res, err := d.RunCommand(context.Background(), target, "main.stats.set_autostat", payload, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}
	if res.Summary.OK != 1 {
		t.Fatalf("expected 1 OK, got %+v", res.Summary)
	}
	if bot1.stats.AutoStat != 4 {
		t.Fatalf("expected AutoStat=4, got %d", bot1.stats.AutoStat)
	}
}
