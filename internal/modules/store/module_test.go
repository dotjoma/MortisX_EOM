package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockStoreBot struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	penya  int
	inv    []interface{}
	bought []string
	sold   []string
	junk   bool
	redeem []string
}

func (m *mockStoreBot) ID() string   { return m.id }
func (m *mockStoreBot) Name() string { return m.name }
func (m *mockStoreBot) Online() bool { return m.online }
func (m *mockStoreBot) Raw() any     { return m }

func (m *mockStoreBot) getInventoryDataLocked() map[string]interface{} {
	return map[string]interface{}{
		"penya": float64(m.penya),
		"inv":   m.inv,
	}
}

func (m *mockStoreBot) GetInventoryData() (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getInventoryDataLocked(), nil
}

func (m *mockStoreBot) BuyShopItem(itemID string, itemName string, count int, unitCost int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := count * unitCost
	if m.penya < total {
		return nil, fmt.Errorf("insufficient penya (have %d, need %d)", m.penya, total)
	}
	m.penya -= total
	m.bought = append(m.bought, fmt.Sprintf("%s x%d", itemID, count))
	m.inv = append(m.inv, map[string]interface{}{"id": itemID, "n": count})
	return m.getInventoryDataLocked(), nil
}

func (m *mockStoreBot) SellShopItem(slotIndex int, count int, unitPrice int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.penya += count * unitPrice
	m.sold = append(m.sold, fmt.Sprintf("slot %d x%d", slotIndex, count))
	return m.getInventoryDataLocked(), nil
}

func (m *mockStoreBot) SellJunkItems() (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.junk = true
	return m.getInventoryDataLocked(), nil
}

func (m *mockStoreBot) RedeemCode(code string) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.redeem = append(m.redeem, code)
	return m.getInventoryDataLocked(), nil
}

type storeProvider struct {
	bots map[string]*mockStoreBot
}

func (p *storeProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *storeProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *storeProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestStoreModule_BuyAndSell(t *testing.T) {
	b1 := &mockStoreBot{id: "bot-1", name: "Bot1", online: true, penya: 10000}
	b2 := &mockStoreBot{id: "bot-2", name: "Bot2", online: true, penya: 100} // insufficient penya
	p := &storeProvider{bots: map[string]*mockStoreBot{"bot-1": b1, "bot-2": b2}}

	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	// 1. Buy items: 10 * 50 = 500 penya
	buyPayload, _ := json.Marshal(BuyItemPayload{
		ItemID:   "II_POT_HP",
		ItemName: "HP Potion",
		Count:    10,
		UnitCost: 50,
	})
	resBuy, err := d.RunCommand(ctx, target, "store.shop.buy", buyPayload, multi.Opts{})
	if err != nil {
		t.Fatalf("Buy command unexpected error: %v", err)
	}
	// b1 OK (had 10000), b2 Failed (had only 100)
	if resBuy.Summary.OK != 1 || resBuy.Summary.Failed != 1 {
		t.Fatalf("expected 1 OK 1 Failed, got OK=%d Failed=%d", resBuy.Summary.OK, resBuy.Summary.Failed)
	}
	if b1.penya != 9500 {
		t.Fatalf("b1 penya mismatch, expected 9500 got %d", b1.penya)
	}

	// 2. Sell Junk Items
	resJunk, err := d.RunCommand(ctx, target, "store.shop.sell_junk", nil, multi.Opts{})
	if err != nil || resJunk.Summary.OK != 2 {
		t.Fatalf("Sell junk failed: %v, %+v", err, resJunk.Summary)
	}
	if !b1.junk || !b2.junk {
		t.Fatalf("expected junk sold on both bots")
	}
}

func TestStoreModule_AutoBuyRestock(t *testing.T) {
	b1 := &mockStoreBot{
		id:     "bot-1",
		name:   "Bot1",
		online: true,
		penya:  50000,
		inv: []interface{}{
			map[string]interface{}{"id": "II_POT_HP", "n": 10}, // current count 10 <= 20 threshold
		},
	}

	p := &storeProvider{bots: map[string]*mockStoreBot{"bot-1": b1}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}

	payload, _ := json.Marshal(AutoBuyPayload{
		Rules: []AutoBuyRule{
			{
				ID:           "II_POT_HP",
				Name:         "HP Potion",
				Cost:         100,
				Qty:          50, // need 50 - 10 = 40
				RestockBelow: 20,
			},
		},
		PenyaReserve:   1000,
		MaxSpendPerRun: 10000,
	})

	res, err := d.RunCommand(ctx, target, "store.shop.auto_buy_restock", payload, multi.Opts{})
	if err != nil || res.Summary.OK != 1 {
		t.Fatalf("Auto-buy restock failed: %v, %+v", err, res.Summary)
	}

	// 40 potions bought at 100 each = 4000 spent. Penya becomes 50000 - 4000 = 46000
	if b1.penya != 46000 {
		t.Fatalf("expected 46000 penya remaining, got %d", b1.penya)
	}
	if len(b1.bought) != 1 || b1.bought[0] != "II_POT_HP x40" {
		t.Fatalf("expected 40 potions bought, got: %+v", b1.bought)
	}
}

func TestStoreModule_RedeemCode(t *testing.T) {
	b1 := &mockStoreBot{id: "bot-1", name: "Bot1", online: true}
	p := &storeProvider{bots: map[string]*mockStoreBot{"bot-1": b1}}

	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}

	payload, _ := json.Marshal(RedeemPayload{Code: "PROMO2026"})
	res, err := d.RunCommand(ctx, target, "store.shop.redeem", payload, multi.Opts{})
	if err != nil || res.Summary.OK != 1 {
		t.Fatalf("Redeem failed: %v, %+v", err, res.Summary)
	}
	if len(b1.redeem) != 1 || b1.redeem[0] != "PROMO2026" {
		t.Fatalf("redeem code mismatch: %+v", b1.redeem)
	}
}
