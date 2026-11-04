package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
)

// InventoryController abstracts inventory operations on a bot instance.
type InventoryController interface {
	GetInventoryData() (map[string]interface{}, error)
	ToggleItemLock(slotIndex int) (map[string]interface{}, error)
	ToggleItemJunk(slotIndex int) (map[string]interface{}, error)
	EquipItem(slotIndex int, targetSlot string) (map[string]interface{}, error)
	UnequipItem(slotKey string) (map[string]interface{}, error)
	DropItem(slotIndex int) (map[string]interface{}, error)
	UseInventoryItem(slotIndex int) (map[string]interface{}, error)
	SortInventory(by string, keepLocked bool) (map[string]interface{}, error)
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Inventory" }
func (m *Module) ID() string    { return "item" }
func (m *Module) Title() string { return "Inventory & Gear" }
func (m *Module) Order() int    { return 7 }
func (m *Module) Icon() string  { return "inventory_2" }

type ItemActionPayload struct {
	FocusedID  string `json:"focused_id,omitempty"`
	SlotIndex  int    `json:"slot_index"`
	ItemID     string `json:"item_id"`
	Refine     int    `json:"refine"`
	TargetSlot string `json:"target_slot,omitempty"`
	SlotKey    string `json:"slot_key,omitempty"`
	By         string `json:"by,omitempty"`
	KeepLocked bool   `json:"keep_locked,omitempty"`
}

// resolveSlot resolves the bag slot index according to the Revised Inventory Item Adapter:
// 1. Focused bot honors slot_index directly if valid.
// 2. Other bots search bag for matching itemID and exact refine level.
func resolveSlot(inst InventoryController, b multi.Bot, p ItemActionPayload) (int, error) {
	if p.FocusedID != "" && b.ID() == p.FocusedID && p.SlotIndex >= 0 {
		return p.SlotIndex, nil
	}

	data, err := inst.GetInventoryData()
	if err != nil {
		return -1, err
	}
	inv, ok := data["inv"].([]interface{})
	if !ok {
		return -1, errors.New("inventory data not found")
	}

	for idx, itemRaw := range inv {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok || len(itemMap) == 0 {
			continue
		}
		id, _ := itemMap["id"].(string)
		if !strings.EqualFold(id, p.ItemID) {
			continue
		}

		ref := 0
		if rVal, exists := itemMap["refine"]; exists {
			switch v := rVal.(type) {
			case float64:
				ref = int(v)
			case int:
				ref = v
			}
		}
		if p.Refine > 0 && ref != p.Refine {
			continue
		}

		return idx, nil
	}

	if p.Refine > 0 {
		return -1, fmt.Errorf("no item %s (+%d) found in inventory", p.ItemID, p.Refine)
	}
	return -1, fmt.Errorf("no item %s found in inventory", p.ItemID)
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Lock/Unlock Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.lock",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		RiskFor: func(b multi.Bot, payload any) multi.Risk {
			var p ItemActionPayload
			if raw, ok := payload.(json.RawMessage); ok {
				_ = json.Unmarshal(raw, &p)
			}
			if p.Refine > 0 {
				return multi.High
			}
			return multi.Low
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.ToggleItemLock(slot)
		},
	})

	// 2. Junk/Keep Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.junk",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		RiskFor: func(b multi.Bot, payload any) multi.Risk {
			var p ItemActionPayload
			if raw, ok := payload.(json.RawMessage); ok {
				_ = json.Unmarshal(raw, &p)
			}
			if p.Refine > 0 {
				return multi.High
			}
			return multi.Low
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.ToggleItemJunk(slot)
		},
	})

	// 3. Equip Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.equip",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		RiskFor: func(b multi.Bot, payload any) multi.Risk {
			var p ItemActionPayload
			if raw, ok := payload.(json.RawMessage); ok {
				_ = json.Unmarshal(raw, &p)
			}
			if p.Refine > 0 {
				return multi.High
			}
			return multi.Low
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.EquipItem(slot, p.TargetSlot)
		},
	})

	// 4. Unequip Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.unequip",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if p.SlotKey == "" {
				return errors.New("slot key is required for unequip")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.UnequipItem(p.SlotKey)
		},
	})

	// 5. Use Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.use",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		RiskFor: func(b multi.Bot, payload any) multi.Risk {
			var p ItemActionPayload
			if raw, ok := payload.(json.RawMessage); ok {
				_ = json.Unmarshal(raw, &p)
			}
			if p.Refine > 0 {
				return multi.High
			}
			return multi.Low
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.UseInventoryItem(slot)
		},
	})

	// 6. Drop / Destroy Item
	r.Command(multi.CommandDef{
		Key:          "inventory.item.drop",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Risk:         multi.High, // Always high risk for destroying items
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.DropItem(slot)
		},
	})

	// 7. Sort Inventory
	r.Command(multi.CommandDef{
		Key:          "inventory.item.sort",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p ItemActionPayload
			if len(payload) > 0 {
				_ = json.Unmarshal(payload, &p)
			}
			by := p.By
			if by == "" {
				by = "type"
			}
			inst, ok := b.Raw().(InventoryController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.SortInventory(by, p.KeepLocked)
		},
	})
}
