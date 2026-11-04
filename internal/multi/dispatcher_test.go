package multi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testBot struct {
	mu     sync.RWMutex
	id     string
	name   string
	online bool
	data   any
}

func (b *testBot) ID() string { return b.id }
func (b *testBot) Name() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.name
}
func (b *testBot) Online() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.online
}
func (b *testBot) SetOnline(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.online = v
}
func (b *testBot) Raw() any {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.data
}

type testBotProvider struct {
	mu   sync.RWMutex
	bots map[string]*testBot
}

func newTestBotProvider(bots ...*testBot) *testBotProvider {
	bp := &testBotProvider{bots: make(map[string]*testBot)}
	for _, b := range bots {
		bp.bots[b.id] = b
	}
	return bp
}

func (p *testBotProvider) Get(id string) (Bot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	b, ok := p.bots[id]
	return b, ok
}

func (p *testBotProvider) ByName(name string) (Bot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *testBotProvider) All() []Bot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := make([]Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

type testEmitter struct {
	mu     sync.Mutex
	events []struct {
		Name string
		Data any
	}
}

func (e *testEmitter) Emit(eventName string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, struct {
		Name string
		Data any
	}{Name: eventName, Data: data})
}

type testPersister struct {
	mu    sync.Mutex
	saved []string
}

func (p *testPersister) Save(b Bot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.saved = append(p.saved, b.ID())
	return nil
}

func (p *testPersister) Flush() error { return nil }

func setupTestDispatcher(botCount int) (*Dispatcher, *testBotProvider, *testEmitter, *testPersister) {
	bots := make([]*testBot, botCount)
	for i := 0; i < botCount; i++ {
		id := fmt.Sprintf("bot-%d", i+1)
		bots[i] = &testBot{
			id:     id,
			name:   fmt.Sprintf("Madrigal_%d", i+1),
			online: true,
		}
	}

	bp := newTestBotProvider(bots...)
	emitter := &testEmitter{}
	persister := &testPersister{}

	d := NewDispatcher(Deps{
		Bots:      bp,
		Emitter:   emitter,
		Persister: persister,
		Workers:   4,
		Clock:     RealClock{},
	})

	return d, bp, emitter, persister
}

// 1. Tests for 1, 3, and 10 bots
func TestDispatcher_BotScaling(t *testing.T) {
	for _, count := range []int{1, 3, 10} {
		t.Run(fmt.Sprintf("%d_bots", count), func(t *testing.T) {
			d, _, _, _ := setupTestDispatcher(count)

			executedCount := int32(0)
			d.Registry().RegisterCommand("test.", CommandDef{
				Key:   "test.ping",
				Scope: AllSelected,
				Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
					atomic.AddInt32(&executedCount, 1)
					return "pong", nil
				},
			})

			targetIDs := make([]string, count)
			for i := 0; i < count; i++ {
				targetIDs[i] = fmt.Sprintf("bot-%d", i+1)
			}

			res, err := d.RunCommand(context.Background(), Target{
				IDs:       targetIDs,
				FocusedID: targetIDs[0],
			}, "test.ping", nil, Opts{})

			if err != nil || res.Error != "" {
				t.Fatalf("unexpected error: %v / %s", err, res.Error)
			}
			if res.Summary.Total != count || res.Summary.OK != count {
				t.Fatalf("expected total=%d ok=%d, got total=%d ok=%d", count, count, res.Summary.Total, res.Summary.OK)
			}
			if atomic.LoadInt32(&executedCount) != int32(count) {
				t.Fatalf("expected %d executions, got %d", count, executedCount)
			}
		})
	}
}

func findBotResult(s Summary, botID string) *BotResult {
	for i := range s.Results {
		if s.Results[i].BotID == botID {
			return &s.Results[i]
		}
	}
	return nil
}

// 2. Offline bot skipped
func TestDispatcher_OfflineBotSkipped(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(2)
	bp.bots["bot-2"].online = false

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.attack",
		Scope: AllSelected,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.attack", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK 1 Skipped, got %d OK %d Skipped", res.Summary.OK, res.Summary.Skipped)
	}
	r2 := findBotResult(res.Summary, "bot-2")
	if r2 == nil || r2.Reason != "offline" {
		t.Fatalf("expected offline reason for bot-2, got %+v", r2)
	}
}

// 3. Bot that panics (recover per bot)
func TestDispatcher_PanicRecovery(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.panic",
		Scope: AllSelected,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			if b.ID() == "bot-2" {
				panic("simulated fatal crash")
			}
			return "safe", nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.panic", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Failed != 1 {
		t.Fatalf("expected 1 OK 1 Failed, got %d OK %d Failed", res.Summary.OK, res.Summary.Failed)
	}
}

// 4. Bot that times out
func TestDispatcher_Timeout(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:     "test.slow",
		Scope:   AllSelected,
		Timeout: 20 * time.Millisecond,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			if b.ID() == "bot-2" {
				select {
				case <-time.After(100 * time.Millisecond):
					return nil, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return "quick", nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.slow", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Failed != 1 {
		t.Fatalf("expected 1 OK 1 Failed, got %d OK %d Failed", res.Summary.OK, res.Summary.Failed)
	}
}

// 5. Validation failure reports Skipped
func TestDispatcher_ValidationFailure(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.levelreq",
		Scope: AllSelected,
		Validate: func(b Bot, payload json.RawMessage) error {
			if b.ID() == "bot-2" {
				return errors.New("level 60 required")
			}
			return nil
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.levelreq", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK 1 Skipped, got %d OK %d Skipped", res.Summary.OK, res.Summary.Skipped)
	}
	r2 := findBotResult(res.Summary, "bot-2")
	if r2 == nil || r2.Reason != "level 60 required" {
		t.Fatalf("expected level 60 required reason, got %+v", r2)
	}
}

// 6. FocusedOnly command with multiple bots selected
func TestDispatcher_FocusedOnlyScope(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(3)

	executedBot := ""
	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.focused_action",
		Scope: FocusedOnly,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			executedBot = b.ID()
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2", "bot-3"},
		FocusedID: "bot-2",
	}, "test.focused_action", nil, Opts{})

	if res.Summary.Total != 1 || res.Summary.OK != 1 {
		t.Fatalf("expected 1 total, got %d", res.Summary.Total)
	}
	if executedBot != "bot-2" {
		t.Fatalf("expected execution on focused bot-2, got %q", executedBot)
	}
}

// 7. Focused bot not in selection rejected
func TestDispatcher_FocusedNotInSelection(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(3)

	d.Registry().RegisterSetting("test.", SettingDef{
		Key: "test.speed",
		Set: func(b Bot, v any) error { return nil },
	})

	res, _ := d.ApplySetting(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-3", // Not in selection
	}, "test.speed", 1.5, Opts{})

	if res.Error == "" {
		t.Fatalf("expected error when focused bot not in IDs")
	}
}

// 8. Confirm gate returns value and executes only with Confirm
func TestDispatcher_ConfirmationGate(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(3)

	var executed atomic.Bool
	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.drop_all",
		Scope: AllSelected,
		Risk:  High,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			executed.Store(true)
			return nil, nil
		},
	})

	// Without confirm: returns Confirm request
	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.drop_all", nil, Opts{Confirm: false})

	if res.Confirm == nil {
		t.Fatalf("expected Confirm request to be returned as value")
	}
	if res.Confirm.Entry != "test.drop_all" || len(res.Confirm.Bots) != 2 {
		t.Fatalf("unexpected confirm payload: %+v", res.Confirm)
	}
	if executed.Load() {
		t.Fatalf("should not have executed without confirm")
	}

	// With confirm: executes
	res2, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.drop_all", nil, Opts{Confirm: true})

	if res2.Confirm != nil || res2.Summary.OK != 2 {
		t.Fatalf("expected successful execution with confirm, got %+v", res2)
	}
	if !executed.Load() {
		t.Fatalf("expected execution to proceed with confirm")
	}
}

// 9. RiskFor dynamic escalation
func TestDispatcher_DynamicRiskFor(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.sell_item",
		Scope: AllSelected,
		Risk:  Low, // Static risk is Low
		RiskFor: func(b Bot, payload any) Risk {
			// Dynamic escalation to High if item is refined
			var p struct{ Refine int }
			_ = json.Unmarshal(payload.(json.RawMessage), &p)
			if p.Refine > 0 {
				return High
			}
			return Low
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	// Unrefined: executes without confirm
	raw0, _ := json.Marshal(map[string]int{"Refine": 0})
	res0, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.sell_item", raw0, Opts{Confirm: false})

	if res0.Confirm != nil || res0.Summary.OK != 2 {
		t.Fatalf("expected refine 0 to succeed without confirm")
	}

	// Refined (+3): escalates to High and requires confirm
	raw3, _ := json.Marshal(map[string]int{"Refine": 3})
	res3, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "test.sell_item", raw3, Opts{Confirm: false})

	if res3.Confirm == nil {
		t.Fatalf("expected refine > 0 to escalate to High risk and return Confirm")
	}
}

// 10. Dedupe collapses duplicate work
func TestDispatcher_Dedupe(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(3)

	executedAccounts := make(map[string]int)
	var mu sync.Mutex

	d.Registry().RegisterCommand("store.", CommandDef{
		Key:   "store.redeem",
		Scope: AllSelected,
		Dedupe: func(b Bot, payload json.RawMessage) string {
			// All 3 bots share the same account "acc_alpha"
			return "acc_alpha"
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			mu.Lock()
			executedAccounts["acc_alpha"]++
			mu.Unlock()
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2", "bot-3"},
		FocusedID: "bot-1",
	}, "store.redeem", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Skipped != 2 {
		t.Fatalf("expected 1 OK 2 Skipped, got %d OK %d Skipped", res.Summary.OK, res.Summary.Skipped)
	}
	if executedAccounts["acc_alpha"] != 1 {
		t.Fatalf("expected exactly 1 execution on account, got %d", executedAccounts["acc_alpha"])
	}
}

// 11. Activity lock: Farm running on bot makes Transfer start Skipped with busy: farm
func TestDispatcher_ActivityLock(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	// Set bot-1 as busy with "farm"
	d.Activities().Set("bot-1", "farm")

	d.Registry().RegisterCommand("transfer.", CommandDef{
		Key:       "transfer.start",
		Scope:     AllSelected,
		Exclusive: "transfer",
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, "transfer.start", nil, Opts{})

	if res.Summary.OK != 1 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 1 OK 1 Skipped, got %d OK %d Skipped", res.Summary.OK, res.Summary.Skipped)
	}
	r1 := findBotResult(res.Summary, "bot-1")
	if r1 == nil || r1.Reason != "busy: farm" {
		t.Fatalf("expected busy: farm reason, got %+v", r1)
	}
}

// 12. Bot-to-bot self-reference and offline receiver handling
func TestDispatcher_BotToBotRefs(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(3)
	bp.bots["bot-3"].online = false

	d.Registry().RegisterCommand("automation.transfer.", CommandDef{
		Key:   "automation.transfer.trade",
		Scope: AllSelected,
		RefsBots: func(payload json.RawMessage) []BotRef {
			var p struct{ Target string }
			_ = json.Unmarshal(payload, &p)
			return []BotRef{{Name: p.Target}}
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	// Self-reference test: bot-1 trades with Madrigal_1
	selfPayload, _ := json.Marshal(map[string]string{"Target": "Madrigal_1"})
	resSelf, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1"},
		FocusedID: "bot-1",
	}, "automation.transfer.trade", selfPayload, Opts{})

	if resSelf.Summary.Skipped != 1 || resSelf.Summary.Results[0].Reason != "self-reference" {
		t.Fatalf("expected self-reference reason, got %+v", resSelf.Summary)
	}

	// Offline receiver test: bot-1 trades with offline Madrigal_3
	offlinePayload, _ := json.Marshal(map[string]string{"Target": "Madrigal_3"})
	resOff, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1"},
		FocusedID: "bot-1",
	}, "automation.transfer.trade", offlinePayload, Opts{})

	if resOff.Summary.Skipped != 1 || resOff.Summary.Results[0].Reason != "referenced bot offline" {
		t.Fatalf("expected referenced bot offline reason, got %+v", resOff.Summary)
	}
}

// 13. Mixed-ConfigSpec batch rejected
func TestDispatcher_MixedConfigSpecBatchRejected(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	spec1 := &ConfigSpec{Prefix: "spec1."}
	spec2 := &ConfigSpec{Prefix: "spec2."}

	d.Registry().RegisterSetting("spec1.", SettingDef{
		Key:        "spec1.settingA",
		ConfigSpec: spec1,
		Set:        func(b Bot, v any) error { return nil },
	})
	d.Registry().RegisterSetting("spec2.", SettingDef{
		Key:        "spec2.settingB",
		ConfigSpec: spec2,
		Set:        func(b Bot, v any) error { return nil },
	})

	res, _ := d.ApplySettings(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, map[string]any{
		"spec1.settingA": 10,
		"spec2.settingB": 20,
	}, Opts{})

	if res.Error != "batch spans more than one ConfigSpec" {
		t.Fatalf("expected batch spans more than one ConfigSpec error, got %q", res.Error)
	}
}

// 14. Sensitive masking and ReadSettings
func TestDispatcher_SensitiveMaskingAndReadSettings(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	values := map[string]string{
		"bot-1": "secret_alpha",
		"bot-2": "secret_beta",
	}

	d.Registry().RegisterSetting("test.", SettingDef{
		Key:       "test.webhook",
		Sensitive: true,
		Get: func(b Bot) (any, error) {
			return values[b.ID()], nil
		},
	})

	// When mixed: PerBot is masked with "***"
	resMixed, err := d.ReadSettings(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, []string{"test.webhook"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mv := resMixed["test.webhook"]
	if !mv.Mixed {
		t.Fatalf("expected mixed state for divergent secrets")
	}
	if mv.PerBot["bot-1"] != "***" || mv.PerBot["bot-2"] != "***" {
		t.Fatalf("expected masked PerBot values, got %+v", mv.PerBot)
	}

	// When NOT mixed: real value for focused bot is returned
	values["bot-2"] = "secret_alpha"
	resUniform, _ := d.ReadSettings(context.Background(), Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}, []string{"test.webhook"})

	mvUniform := resUniform["test.webhook"]
	if mvUniform.Mixed {
		t.Fatalf("expected non-mixed state")
	}
	if mvUniform.Value != "secret_alpha" {
		t.Fatalf("expected real value for focused bot when uniform, got %v", mvUniform.Value)
	}
}

// 15. Batch ReadSettings equals N single reads
func TestDispatcher_BatchReadEqualsSingleReads(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterSetting("test.", SettingDef{
		Key: "test.val1",
		Get: func(b Bot) (any, error) { return "val1", nil },
	})
	d.Registry().RegisterSetting("test.", SettingDef{
		Key: "test.val2",
		Get: func(b Bot) (any, error) { return 42, nil },
	})

	target := Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// Batch read
	batchRes, err := d.ReadSettings(context.Background(), target, []string{"test.val1", "test.val2"})
	if err != nil {
		t.Fatalf("batch read failed: %v", err)
	}

	// Single reads
	single1, _ := d.ReadSettings(context.Background(), target, []string{"test.val1"})
	single2, _ := d.ReadSettings(context.Background(), target, []string{"test.val2"})

	if batchRes["test.val1"].Value != single1["test.val1"].Value {
		t.Fatalf("mismatch on val1")
	}
	if batchRes["test.val2"].Value != single2["test.val2"].Value {
		t.Fatalf("mismatch on val2")
	}
}

// 16. Rapid concurrent changes on the same bot are serialized
func TestDispatcher_RapidConcurrentChanges(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(1)

	var counter int64
	d.Registry().RegisterSetting("test.", SettingDef{
		Key: "test.counter",
		Set: func(b Bot, v any) error {
			time.Sleep(2 * time.Millisecond)
			atomic.AddInt64(&counter, 1)
			return nil
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			_, _ = d.ApplySetting(context.Background(), Target{
				IDs:       []string{"bot-1"},
				FocusedID: "bot-1",
			}, "test.counter", val, Opts{})
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt64(&counter) != 20 {
		t.Fatalf("expected 20 serialized updates, got %d", counter)
	}
}

// 17. Target changes mid-dispatch (snapshot isolation)
func TestDispatcher_TargetChangesMidDispatch(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(3)

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.snapshot",
		Scope: AllSelected,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			time.Sleep(10 * time.Millisecond)
			return nil, nil
		},
	})

	targetSnapshot := Target{
		IDs:       []string{"bot-1", "bot-2"},
		FocusedID: "bot-1",
	}

	go func() {
		time.Sleep(2 * time.Millisecond)
		// Simulate external mutation of bot provider or selection
		bp.mu.Lock()
		bp.bots["bot-3"].online = false
		bp.mu.Unlock()
	}()

	res, err := d.RunCommand(context.Background(), targetSnapshot, "test.snapshot", nil, Opts{})
	if err != nil || res.Summary.Total != 2 || res.Summary.OK != 2 {
		t.Fatalf("expected dispatch on initial snapshot of 2 bots, got %+v", res)
	}
}

// 18. LockRefs=false does not lock receiver
func TestDispatcher_LockRefsFalse(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	d.Registry().RegisterCommand("automation.transfer.", CommandDef{
		Key:       "automation.transfer.mail",
		Scope:     AllSelected,
		Exclusive: "mail",
		LockRefs:  false, // mail does NOT lock receiver
		RefsBots: func(payload json.RawMessage) []BotRef {
			return []BotRef{{Name: "Madrigal_2"}}
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{
		IDs:       []string{"bot-1"},
		FocusedID: "bot-1",
	}, "automation.transfer.mail", nil, Opts{})

	if res.Summary.OK != 1 {
		t.Fatalf("expected mail transfer to succeed: %+v", res.Summary)
	}

	// Verify receiver (bot-2) was NOT locked
	if act := d.Activities().Get("bot-2"); act != "" {
		t.Fatalf("expected bot-2 to NOT be locked under LockRefs=false, got activity %q", act)
	}
}

// 19. Sequential order is respected
func TestDispatcher_SequentialOrder(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(3)

	var order []string
	var mu sync.Mutex

	d.Registry().RegisterCommand("test.", CommandDef{
		Key:   "test.ordered",
		Scope: AllSelected,
		Mode:  Sequential,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			mu.Lock()
			order = append(order, b.ID())
			mu.Unlock()
			return nil, nil
		},
	})

	target := Target{IDs: []string{"bot-1", "bot-2", "bot-3"}, FocusedID: "bot-1"}
	res, _ := d.RunCommand(context.Background(), target, "test.ordered", nil, Opts{})

	if res.Summary.OK != 3 {
		t.Fatalf("expected 3 OK")
	}
	if len(order) != 3 || order[0] != "bot-1" || order[1] != "bot-2" || order[2] != "bot-3" {
		t.Fatalf("expected sequential order [bot-1, bot-2, bot-3], got %v", order)
	}
}

// 20. Idempotent start/stop convergence
func TestDispatcher_IdempotentStartStopConvergence(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(3)
	// bot-1 is online, bot-2 is offline, bot-3 is online
	bp.bots["bot-2"].online = false

	d.Registry().RegisterCommand("main.bot.", CommandDef{
		Key:   "main.bot.connect",
		Scope: AllSelected,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			if b.Online() {
				return "already online", nil
			}
			bp.mu.Lock()
			bp.bots[b.ID()].online = true
			bp.mu.Unlock()
			return "connected", nil
		},
	})

	// Dispatch to all 3
	target := Target{IDs: []string{"bot-1", "bot-2", "bot-3"}, FocusedID: "bot-1"}
	res, _ := d.RunCommand(context.Background(), target, "main.bot.connect", nil, Opts{})

	// bot-2 was offline (skipped by default validation or handled if CanRun handles offline)
	if res.Summary.OK != 2 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 2 OK (already online) and 1 Skipped (offline), got %+v", res.Summary)
	}
}

// 21. PerBot payload evaluation
func TestDispatcher_PerBotPayload(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(2)
	bp.bots["bot-1"].data = 100 // Level 100
	bp.bots["bot-2"].data = 60  // Level 60

	allocations := make(map[string]int)
	var mu sync.Mutex

	d.Registry().RegisterCommand("main.stats.", CommandDef{
		Key:           "main.stats.autofill",
		Scope:         AllSelected,
		PayloadSource: PayloadPerBot,
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			// Sourced from b.Raw()
			lvl := b.Raw().(int)
			mu.Lock()
			allocations[b.ID()] = lvl * 2
			mu.Unlock()
			return nil, nil
		},
	})

	target := Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	res, _ := d.RunCommand(context.Background(), target, "main.stats.autofill", nil, Opts{})

	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK")
	}
	if allocations["bot-1"] != 200 || allocations["bot-2"] != 120 {
		t.Fatalf("expected per-bot computed allocations (200, 120), got %+v", allocations)
	}
}

// 22. Collection rules
func TestDispatcher_CollectionRules(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(2)

	appliedOps := make(map[string][]string)
	var mu sync.Mutex

	d.Registry().RegisterCollection("store.", CollectionDef{
		Key: "store.buyList",
		Ops: []string{"add", "remove", "update"},
		ValidateOp: func(b Bot, op string, payload json.RawMessage) error {
			if op != "add" && op != "remove" && op != "update" {
				return fmt.Errorf("unsupported op: %s", op)
			}
			return nil
		},
		Apply: func(b Bot, op string, payload json.RawMessage) error {
			mu.Lock()
			appliedOps[b.ID()] = append(appliedOps[b.ID()], op)
			mu.Unlock()
			return nil
		},
	})

	col, ok := d.Registry().GetCollection("store.buyList")
	if !ok {
		t.Fatalf("expected store.buyList collection to be registered")
	}

	bot1, _ := d.deps.Bots.Get("bot-1")
	if err := col.ValidateOp(bot1, "add", nil); err != nil {
		t.Fatalf("add op should be valid: %v", err)
	}
	if err := col.ValidateOp(bot1, "delete", nil); err == nil {
		t.Fatalf("delete op should be invalid")
	}

	if err := col.Apply(bot1, "add", nil); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(appliedOps["bot-1"]) != 1 || appliedOps["bot-1"][0] != "add" {
		t.Fatalf("expected add op applied, got %+v", appliedOps)
	}
}

// 23. multi:summary and multi:setting-changed payloads are emitted correctly
func TestDispatcher_EventPayloadShapes(t *testing.T) {
	d, _, emitter, _ := setupTestDispatcher(2)

	d.Registry().RegisterSetting("test.", SettingDef{
		Key: "test.volume",
		Set: func(b Bot, v any) error { return nil },
	})

	target := Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	_, err := d.ApplySetting(context.Background(), target, "test.volume", 75, Opts{})
	if err != nil {
		t.Fatalf("ApplySetting failed: %v", err)
	}

	emitter.mu.Lock()
	events := make([]struct {
		Name string
		Data any
	}, len(emitter.events))
	copy(events, emitter.events)
	emitter.mu.Unlock()

	var settingChanged *SettingChangedPayload
	var summaryEmitted *Summary

	for _, e := range events {
		if e.Name == "multi:setting-changed" {
			p, ok := e.Data.(SettingChangedPayload)
			if ok {
				settingChanged = &p
			}
		} else if e.Name == "multi:summary" {
			s, ok := e.Data.(Summary)
			if ok {
				summaryEmitted = &s
			}
		}
	}

	if settingChanged == nil {
		t.Fatalf("expected multi:setting-changed event emitted")
	}
	if len(settingChanged.Keys) != 1 || settingChanged.Keys[0] != "test.volume" {
		t.Fatalf("invalid setting-changed keys: %v", settingChanged.Keys)
	}
	if len(settingChanged.BotIDs) != 2 {
		t.Fatalf("invalid setting-changed botIDs count: %v", settingChanged.BotIDs)
	}

	if summaryEmitted == nil {
		t.Fatalf("expected multi:summary event emitted")
	}
	if summaryEmitted.Total != 2 || summaryEmitted.OK != 2 {
		t.Fatalf("invalid summaryEmitted total/ok: %+v", summaryEmitted)
	}
}

// 24. Sensitive values never appear in Summary, events, or error strings
func TestDispatcher_SensitiveNeverInSummaryEventsOrErrors(t *testing.T) {
	d, _, emitter, _ := setupTestDispatcher(2)
	secretValue := "MY_SUPER_SECRET_DISCORD_TOKEN_9999"

	d.Registry().RegisterSetting("test.", SettingDef{
		Key:       "test.webhook",
		Sensitive: true,
		Set: func(b Bot, v any) error {
			return nil
		},
	})

	target := Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	res, _ := d.ApplySetting(context.Background(), target, "test.webhook", secretValue, Opts{})

	// Check Summary
	summaryBytes, _ := json.Marshal(res.Summary)
	if string(summaryBytes) != "" && strings.Contains(string(summaryBytes), secretValue) {
		t.Fatalf("sensitive value leaked into Summary JSON: %s", string(summaryBytes))
	}

	// Check Emitted Events
	emitter.mu.Lock()
	for _, e := range emitter.events {
		eBytes, _ := json.Marshal(e.Data)
		if strings.Contains(string(eBytes), secretValue) {
			t.Fatalf("sensitive value leaked into emitted event %s: %s", e.Name, string(eBytes))
		}
	}
	emitter.mu.Unlock()

	// Check Error string
	if strings.Contains(res.Error, secretValue) {
		t.Fatalf("sensitive value leaked into Result.Error: %s", res.Error)
	}
}

// 25. Bot-to-bot: self-reference separately asserted
func TestDispatcher_BotToBot_SelfReference(t *testing.T) {
	d, _, _, _ := setupTestDispatcher(1)

	d.Registry().RegisterCommand("automation.transfer.", CommandDef{
		Key:   "automation.transfer.direct",
		Scope: AllSelected,
		RefsBots: func(payload json.RawMessage) []BotRef {
			var p struct{ Target string }
			_ = json.Unmarshal(payload, &p)
			return []BotRef{{Name: p.Target}}
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	// bot-1 references itself by character name Madrigal_1
	payload, _ := json.Marshal(map[string]string{"Target": "Madrigal_1"})
	res, _ := d.RunCommand(context.Background(), Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}, "automation.transfer.direct", payload, Opts{})

	if res.Summary.Skipped != 1 {
		t.Fatalf("expected self-reference to be skipped, got: %+v", res.Summary)
	}
	r1 := findBotResult(res.Summary, "bot-1")
	if r1 == nil || r1.Reason != "self-reference" {
		t.Fatalf("expected reason 'self-reference', got: %+v", r1)
	}
}

// 26. Bot-to-bot: offline referenced bot separately asserted
func TestDispatcher_BotToBot_OfflineRef(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(2)
	bp.bots["bot-2"].online = false // Receiver offline

	d.Registry().RegisterCommand("automation.transfer.", CommandDef{
		Key:   "automation.transfer.direct",
		Scope: AllSelected,
		RefsBots: func(payload json.RawMessage) []BotRef {
			return []BotRef{{Name: "Madrigal_2"}}
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}, "automation.transfer.direct", nil, Opts{})

	if res.Summary.Skipped != 1 {
		t.Fatalf("expected offline ref to be skipped, got: %+v", res.Summary)
	}
	r1 := findBotResult(res.Summary, "bot-1")
	if r1 == nil || r1.Reason != "referenced bot offline" {
		t.Fatalf("expected reason 'referenced bot offline', got: %+v", r1)
	}
}

// 27. Bot-to-bot: name->bot resolution separately asserted
func TestDispatcher_BotToBot_NameResolution(t *testing.T) {
	d, bp, _, _ := setupTestDispatcher(2)

	var resolvedTargetID string
	d.Registry().RegisterCommand("automation.transfer.", CommandDef{
		Key:   "automation.transfer.direct",
		Scope: AllSelected,
		RefsBots: func(payload json.RawMessage) []BotRef {
			return []BotRef{{Name: "Madrigal_2"}} // references by name
		},
		ValidateRef: func(sender Bot, ref Bot, payload json.RawMessage) error {
			resolvedTargetID = ref.ID()
			return nil
		},
		Execute: func(ctx context.Context, b Bot, payload json.RawMessage) (any, error) {
			return nil, nil
		},
	})

	res, _ := d.RunCommand(context.Background(), Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}, "automation.transfer.direct", nil, Opts{})

	if res.Summary.OK != 1 {
		t.Fatalf("expected command with valid name resolution to succeed, got %+v", res.Summary)
	}
	if resolvedTargetID != "bot-2" {
		t.Fatalf("expected name Madrigal_2 to resolve to bot-2, got %q", resolvedTargetID)
	}
	// Verify lookup in provider
	b, ok := bp.ByName("Madrigal_2")
	if !ok || b.ID() != "bot-2" {
		t.Fatalf("expected ByName to return bot-2")
	}
}

// 28. Adapter concurrency and lock safety test
func TestAdapter_ConcurrencyAndLockSafety(t *testing.T) {
	provider := newTestBotProvider(
		&testBot{id: "bot-1", name: "Alpha", online: true},
		&testBot{id: "bot-2", name: "Beta", online: true},
	)

	var wg sync.WaitGroup
	// 10 concurrent readers calling Online(), Name(), Get(), ByName()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b, ok := provider.Get("bot-1")
				if ok {
					_ = b.Online()
					_ = b.Name()
				}
				b2, ok2 := provider.ByName("Beta")
				if ok2 {
					_ = b2.Online()
					_ = b2.Name()
				}
			}
		}()
	}

	// Concurrent writer mutating status
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			provider.mu.RLock()
			b1 := provider.bots["bot-1"]
			provider.mu.RUnlock()
			b1.SetOnline(j%2 == 0)
			time.Sleep(100 * time.Microsecond)
		}
	}()

	wg.Wait()
}
