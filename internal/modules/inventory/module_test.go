package inventory

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"madrigal-multibot/internal/multi"
)

type mockInvBot struct {
	id     string
	name   string
	online bool
	mu     sync.Mutex
	inv    []interface{}
	locked map[int]bool
	equip  map[string]string
	drops  []int
	sorted bool
}

func (m *mockInvBot) ID() string   { return m.id }
func (m *mockInvBot) Name() string { return m.name }
func (m *mockInvBot) Online() bool { return m.online }
func (m *mockInvBot) Raw() any     { return m }

func (m *mockInvBot) getInventoryDataLocked() map[string]interface{} {
	return map[string]interface{}{
		"inv":   m.inv,
		"equip": m.equip,
	}
}

func (m *mockInvBot) GetInventoryData() (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) ToggleItemLock(slotIndex int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked == nil {
		m.locked = make(map[int]bool)
	}
	m.locked[slotIndex] = !m.locked[slotIndex]
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) ToggleItemJunk(slotIndex int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) EquipItem(slotIndex int, targetSlot string) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.equip == nil {
		m.equip = make(map[string]string)
	}
	if slotIndex >= 0 && slotIndex < len(m.inv) {
		if im, ok := m.inv[slotIndex].(map[string]interface{}); ok {
			id, _ := im["id"].(string)
			m.equip[targetSlot] = id
		}
	}
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) UnequipItem(slotKey string) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.equip, slotKey)
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) DropItem(slotIndex int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drops = append(m.drops, slotIndex)
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) UseInventoryItem(slotIndex int) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getInventoryDataLocked(), nil
}

func (m *mockInvBot) SortInventory(by string, keepLocked bool) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sorted = true
	return m.getInventoryDataLocked(), nil
}

type invProvider struct {
	bots map[string]*mockInvBot
}

func (p *invProvider) Get(id string) (multi.Bot, bool) {
	b, ok := p.bots[id]
	return b, ok
}

func (p *invProvider) ByName(name string) (multi.Bot, bool) {
	for _, b := range p.bots {
		if b.name == name {
			return b, true
		}
	}
	return nil, false
}

func (p *invProvider) All() []multi.Bot {
	res := make([]multi.Bot, 0, len(p.bots))
	for _, b := range p.bots {
		res = append(res, b)
	}
	return res
}

func TestInventoryModule_LockAndRefineResolution(t *testing.T) {
	// Bot 1 (Focused): has Sword (+3) in slot 2
	b1 := &mockInvBot{
		id:     "bot-1",
		name:   "Knight1",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_POT_HP", "refine": 0},
			map[string]interface{}{"id": "II_POT_MP", "refine": 0},
			map[string]interface{}{"id": "II_WEA_SWD", "refine": 3},
		},
	}

	// Bot 2: has Sword (+3) in slot 0
	b2 := &mockInvBot{
		id:     "bot-2",
		name:   "Knight2",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_WEA_SWD", "refine": 3},
			map[string]interface{}{"id": "II_POT_HP", "refine": 0},
		},
	}

	// Bot 3: has Sword (+0) - refine mismatch (+3 vs +0)
	b3 := &mockInvBot{
		id:     "bot-3",
		name:   "Knight3",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_WEA_SWD", "refine": 0},
		},
	}

	p := &invProvider{bots: map[string]*mockInvBot{"bot-1": b1, "bot-2": b2, "bot-3": b3}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2", "bot-3"}, FocusedID: "bot-1"}

	payload, _ := json.Marshal(ItemActionPayload{
		FocusedID: "bot-1",
		SlotIndex: 2,
		ItemID:    "II_WEA_SWD",
		Refine:    3,
	})

	// Refine=3 triggers Risk=High. Without Opts{Confirm: true}, Dispatcher must require confirmation.
	resGated, err := d.RunCommand(ctx, target, "inventory.item.lock", payload, multi.Opts{Confirm: false})
	if err != nil {
		t.Fatalf("unexpected RunCommand error: %v", err)
	}
	if resGated.Confirm == nil {
		t.Fatalf("expected confirm request for refine > 0, got none")
	}

	// Now run with Confirm: true
	res, err := d.RunCommand(ctx, target, "inventory.item.lock", payload, multi.Opts{Confirm: true})
	if err != nil {
		t.Fatalf("confirmed RunCommand failed: %v", err)
	}

	// b1 (slot 2) OK, b2 (matched slot 0) OK, b3 (no +3 sword) Skipped
	if res.Summary.OK != 2 || res.Summary.Skipped != 1 {
		t.Fatalf("expected 2 OK 1 Skipped, got OK=%d Skipped=%d: %+v", res.Summary.OK, res.Summary.Skipped, res.Summary)
	}

	if !b1.locked[2] {
		t.Fatalf("b1 slot 2 was not locked")
	}
	if !b2.locked[0] {
		t.Fatalf("b2 slot 0 was not locked via itemID/refine adapter")
	}
}

func TestInventoryModule_EquipAndUnequip(t *testing.T) {
	b1 := &mockInvBot{
		id:     "bot-1",
		name:   "Knight1",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_ARM_SUIT", "refine": 0},
		},
		equip: map[string]string{
			"weapon": "II_WEA_AXE",
		},
	}

	p := &invProvider{bots: map[string]*mockInvBot{"bot-1": b1}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1"}, FocusedID: "bot-1"}

	// 1. Equip suit
	eqPayload, _ := json.Marshal(ItemActionPayload{
		FocusedID:  "bot-1",
		SlotIndex:  0,
		ItemID:     "II_ARM_SUIT",
		Refine:     0,
		TargetSlot: "suit",
	})
	resEq, err := d.RunCommand(ctx, target, "inventory.item.equip", eqPayload, multi.Opts{})
	if err != nil || resEq.Summary.OK != 1 {
		t.Fatalf("Equip failed: %v, %+v", err, resEq.Summary)
	}
	if b1.equip["suit"] != "II_ARM_SUIT" {
		t.Fatalf("expected suit equipped, got: %+v", b1.equip)
	}

	// 2. Unequip weapon
	uneqPayload, _ := json.Marshal(ItemActionPayload{
		SlotKey: "weapon",
	})
	resUneq, err := d.RunCommand(ctx, target, "inventory.item.unequip", uneqPayload, multi.Opts{})
	if err != nil || resUneq.Summary.OK != 1 {
		t.Fatalf("Unequip failed: %v, %+v", err, resUneq.Summary)
	}
	if _, hasWep := b1.equip["weapon"]; hasWep {
		t.Fatalf("expected weapon unequipped, still present")
	}
}

func TestInventoryModule_DropRequiresConfirmation(t *testing.T) {
	b1 := &mockInvBot{
		id:     "bot-1",
		name:   "Knight1",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_JUNK", "refine": 0},
		},
	}
	b2 := &mockInvBot{
		id:     "bot-2",
		name:   "Knight2",
		online: true,
		inv: []interface{}{
			map[string]interface{}{"id": "II_JUNK", "refine": 0},
		},
	}

	p := &invProvider{bots: map[string]*mockInvBot{"bot-1": b1, "bot-2": b2}}
	d := multi.NewDispatcher(multi.Deps{Bots: p, Workers: 4})
	mod := New()
	if err := d.RegisterModule(mod); err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	ctx := context.Background()
	target := multi.Target{IDs: []string{"bot-1", "bot-2"}, FocusedID: "bot-1"}

	payload, _ := json.Marshal(ItemActionPayload{
		FocusedID: "bot-1",
		SlotIndex: 0,
		ItemID:    "II_JUNK",
	})

	// Drop across N>1 bots has Risk: High and requires confirmation
	resGated, err := d.RunCommand(ctx, target, "inventory.item.drop", payload, multi.Opts{Confirm: false})
	if err != nil || resGated.Confirm == nil {
		t.Fatalf("expected confirmation required for drop across N>1 bots, got: err=%v, res=%+v", err, resGated)
	}

	// With Confirm: true
	res, err := d.RunCommand(ctx, target, "inventory.item.drop", payload, multi.Opts{Confirm: true})
	if err != nil || res.Summary.OK != 2 {
		t.Fatalf("confirmed drop failed: %v, %+v", err, res.Summary)
	}
	if len(b1.drops) != 1 || b1.drops[0] != 0 {
		t.Fatalf("expected drop of slot 0 on b1, got: %+v", b1.drops)
	}
	if len(b2.drops) != 1 || b2.drops[0] != 0 {
		t.Fatalf("expected drop of slot 0 on b2, got: %+v", b2.drops)
	}
}
