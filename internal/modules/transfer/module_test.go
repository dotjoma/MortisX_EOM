package transfer

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"madrigal-multibot/internal/multi"
)

type mockTransferBot struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	inv    map[string]interface{}
}

func (m *mockTransferBot) ID() string   { return m.id }
func (m *mockTransferBot) Name() string { return m.name }
func (m *mockTransferBot) Online() bool { return m.online }
func (m *mockTransferBot) Raw() any     { return m }

func (m *mockTransferBot) GetInventoryData() (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inv, nil
}

func (m *mockTransferBot) SendMail(to string, title string, text string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]interface{}{"status": "mail_sent", "to": to}, nil
}

func (m *mockTransferBot) StartTrade(target string, slotIndex int, count int, penya int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]interface{}{"status": "trade_started", "target": target}, nil
}

type mockBotProvider struct {
	bots map[string]*mockTransferBot
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

func setupDispatcher(bots map[string]*mockTransferBot) (*multi.Dispatcher, error) {
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

func TestTransferModule_SelfTransferRejected(t *testing.T) {
	b1 := &mockTransferBot{id: "bot-1", name: "Alice", online: true}
	bots := map[string]*mockTransferBot{"bot-1": b1}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}

	// Self mail
	mailPayload, _ := json.Marshal(MailPayload{To: "Alice", Penya: 100})
	res, _ := d.RunCommand(context.Background(), target, "automation.transfer.send_mail", mailPayload, multi.Opts{})
	if res.Summary.Skipped != 1 {
		t.Fatalf("expected self mail to be skipped, got %v", res.Summary)
	}

	// Self trade
	tradePayload, _ := json.Marshal(TradePayload{Target: "Alice", Penya: 100})
	res, _ = d.RunCommand(context.Background(), target, "automation.transfer.start_trade", tradePayload, multi.Opts{})
	if res.Summary.Skipped != 1 {
		t.Fatalf("expected self trade to be skipped, got %v", res.Summary)
	}
}

func TestTransferModule_TradeOfflinePartnerSkipped(t *testing.T) {
	b1 := &mockTransferBot{id: "bot-1", name: "Alice", online: true}
	b2 := &mockTransferBot{id: "bot-2", name: "Bob", online: false}
	bots := map[string]*mockTransferBot{"bot-1": b1, "bot-2": b2}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}
	tradePayload, _ := json.Marshal(TradePayload{Target: "Bob", Penya: 100})
	res, err := d.RunCommand(context.Background(), target, "automation.transfer.start_trade", tradePayload, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand trade failed: %v", err)
	}
	if res.Summary.Skipped != 1 {
		t.Fatalf("expected trade with offline partner to be Skipped, got %v", res.Summary)
	}
}

func TestTransferModule_SendMailSuccess(t *testing.T) {
	b1 := &mockTransferBot{
		id:     "bot-1",
		name:   "Alice",
		online: true,
		inv: map[string]interface{}{
			"inv": []interface{}{
				map[string]interface{}{"id": "II_GEN_POT_PRI_REMA300", "n": 10},
			},
		},
	}
	b2 := &mockTransferBot{
		id:     "bot-2",
		name:   "Bob",
		online: true,
		inv: map[string]interface{}{
			"inv": []interface{}{
				map[string]interface{}{"id": "II_GEN_POT_PRI_REMA300", "n": 5},
			},
		},
	}
	bots := map[string]*mockTransferBot{"bot-1": b1, "bot-2": b2}

	d, err := setupDispatcher(bots)
	if err != nil {
		t.Fatalf("setupDispatcher failed: %v", err)
	}

	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}
	mailPayload, _ := json.Marshal(MailPayload{
		FocusedID: "bot-1",
		To:        "Charlie",
		SlotIndex: 0,
		ItemID:    "II_GEN_POT_PRI_REMA300",
		Count:     2,
		Penya:     500,
	})

	res, err := d.RunCommand(context.Background(), target, "automation.transfer.send_mail", mailPayload, multi.Opts{})
	if err != nil {
		t.Fatalf("RunCommand send_mail failed: %v", err)
	}
	if res.Summary.OK != 2 {
		t.Fatalf("expected 2 OK, got %d (failed: %d, skipped: %d)", res.Summary.OK, res.Summary.Failed, res.Summary.Skipped)
	}
}
