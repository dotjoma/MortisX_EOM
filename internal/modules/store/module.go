package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
)

// ShopController abstracts store and merchant actions on a bot instance.
type ShopController interface {
	GetInventoryData() (map[string]interface{}, error)
	BuyShopItem(itemID string, itemName string, count int, unitCost int) (map[string]interface{}, error)
	SellShopItem(slotIndex int, count int, unitPrice int) (map[string]interface{}, error)
	SellJunkItems() (map[string]interface{}, error)
	RedeemCode(code string) (map[string]interface{}, error)
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Store" }
func (m *Module) ID() string    { return "shop" }
func (m *Module) Title() string { return "Shop & Vendors" }
func (m *Module) Order() int    { return 8 }
func (m *Module) Icon() string  { return "storefront" }

type BuyItemPayload struct {
	ItemID   string `json:"item_id"`
	ItemName string `json:"item_name"`
	Count    int    `json:"count"`
	UnitCost int    `json:"unit_cost"`
}

type SellItemPayload struct {
	FocusedID string `json:"focused_id,omitempty"`
	SlotIndex int    `json:"slot_index"`
	ItemID    string `json:"item_id"`
	Refine    int    `json:"refine"`
	Count     int    `json:"count"`
	UnitPrice int    `json:"unit_price"`
}

type AutoBuyRule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Cost         int    `json:"cost"`
	Qty          int    `json:"qty"`
	RestockBelow int    `json:"restock_below"`
}

type AutoBuyPayload struct {
	Rules           []AutoBuyRule `json:"rules"`
	PenyaReserve    int           `json:"penya_reserve"`
	MaxSpendPerRun  int           `json:"max_spend_per_run"`
	StopBuyingBelow int           `json:"stop_buying_below"`
}

type RedeemPayload struct {
	Code string `json:"code"`
}

func resolveSlot(inst ShopController, b multi.Bot, p SellItemPayload) (int, error) {
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
	// 1. Buy Shop Item
	r.Command(multi.CommandDef{
		Key:          "store.shop.buy",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p BuyItemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if p.ItemID == "" {
				return errors.New("item_id cannot be empty")
			}
			if p.Count <= 0 {
				return errors.New("count must be greater than 0")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p BuyItemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.BuyShopItem(p.ItemID, p.ItemName, p.Count, p.UnitCost)
		},
	})

	// 2. Sell Shop Item
	r.Command(multi.CommandDef{
		Key:          "store.shop.sell",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		RiskFor: func(b multi.Bot, payload any) multi.Risk {
			var p SellItemPayload
			if raw, ok := payload.(json.RawMessage); ok {
				_ = json.Unmarshal(raw, &p)
			}
			if p.Refine > 0 {
				return multi.High
			}
			return multi.Low
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p SellItemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			_, err := resolveSlot(inst, b, p)
			return err
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p SellItemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			slot, err := resolveSlot(inst, b, p)
			if err != nil {
				return nil, err
			}
			return inst.SellShopItem(slot, p.Count, p.UnitPrice)
		},
	})

	// 3. Sell All Junk
	r.Command(multi.CommandDef{
		Key:          "store.shop.sell_junk",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			type customCleaner interface {
				CleanInventoryWithCap(maxCap int) (map[string]interface{}, error)
			}
			var p struct {
				MaxPenyaCap int `json:"max_penya_cap"`
			}
			if len(payload) > 0 {
				_ = json.Unmarshal(payload, &p)
			}
			if p.MaxPenyaCap > 0 {
				if cc, ok := b.Raw().(customCleaner); ok {
					return cc.CleanInventoryWithCap(p.MaxPenyaCap)
				}
			}
			return inst.SellJunkItems()
		},
	})

	// 4. Auto-Buy Restock
	r.Command(multi.CommandDef{
		Key:          "store.shop.auto_buy_restock",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p AutoBuyPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}

			data, err := inst.GetInventoryData()
			if err != nil {
				return nil, err
			}
			inv, ok := data["inv"].([]interface{})
			if !ok {
				return nil, errors.New("inventory data not found")
			}

			counts := make(map[string]int)
			for _, raw := range inv {
				if im, ok := raw.(map[string]interface{}); ok && len(im) > 0 {
					if id, ok := im["id"].(string); ok {
						n := 1
						if nVal, exists := im["n"]; exists {
							switch v := nVal.(type) {
							case float64:
								n = int(v)
							case int:
								n = v
							}
						}
						counts[id] += n
					}
				}
			}

			currentPenya := 0
			if pVal, exists := data["penya"]; exists {
				switch v := pVal.(type) {
				case float64:
					currentPenya = int(v)
				case int:
					currentPenya = v
				}
			}

			spent := 0
			totalBought := 0

			for _, rule := range p.Rules {
				if rule.Cost <= 0 {
					continue
				}
				cur := counts[rule.ID]
				if cur <= rule.RestockBelow {
					needed := rule.Qty - cur
					if needed <= 0 {
						continue
					}

					if p.MaxSpendPerRun > 0 && spent+(rule.Cost*needed) > p.MaxSpendPerRun {
						needed = (p.MaxSpendPerRun - spent) / rule.Cost
					}
					if p.PenyaReserve > 0 && (currentPenya-spent-(rule.Cost*needed)) < p.PenyaReserve {
						needed = (currentPenya - spent - p.PenyaReserve) / rule.Cost
					}
					if needed <= 0 {
						break
					}

					_, buyErr := inst.BuyShopItem(rule.ID, rule.Name, needed, rule.Cost)
					if buyErr != nil {
						return totalBought, buyErr
					}
					spent += rule.Cost * needed
					totalBought += needed
					counts[rule.ID] += needed
				}
			}
			return map[string]interface{}{"bought": totalBought, "spent": spent}, nil
		},
	})

	// 5. Redeem Promo Code
	r.Command(multi.CommandDef{
		Key:          "store.shop.redeem",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      100 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p RedeemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if strings.TrimSpace(p.Code) == "" {
				return errors.New("code cannot be empty")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p RedeemPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(ShopController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.RedeemCode(p.Code)
		},
	})
}
