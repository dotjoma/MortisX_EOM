package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
)

type TransferController interface {
	GetInventoryData() (map[string]interface{}, error)
	SendMail(to string, title string, text string, slotIndex int, count int, penya int) (map[string]interface{}, error)
	StartTrade(target string, slotIndex int, count int, penya int) (map[string]interface{}, error)
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Automation" }
func (m *Module) ID() string    { return "transfer" }
func (m *Module) Title() string { return "Transfer" }
func (m *Module) Order() int    { return 2 }
func (m *Module) Icon() string  { return "swap_horiz" }

type MailPayload struct {
	FocusedID string `json:"focused_id,omitempty"`
	To        string `json:"to"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	SlotIndex int    `json:"slot_index"`
	ItemID    string `json:"item_id"`
	Refine    int    `json:"refine"`
	Count     int    `json:"count"`
	Penya     int    `json:"penya"`
}

type TradePayload struct {
	FocusedID string `json:"focused_id,omitempty"`
	Target    string `json:"target"`
	SlotIndex int    `json:"slot_index"`
	ItemID    string `json:"item_id"`
	Refine    int    `json:"refine"`
	Count     int    `json:"count"`
	Penya     int    `json:"penya"`
}

func resolveSlot(inst TransferController, b multi.Bot, focusedID string, slotIndex int, itemID string, refine int) (int, error) {
	if slotIndex < 0 && itemID == "" {
		return -1, nil // Penya-only transfer
	}
	if focusedID != "" && b.ID() == focusedID && slotIndex >= 0 {
		return slotIndex, nil
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
		if !strings.EqualFold(id, itemID) {
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
		if refine > 0 && ref != refine {
			continue
		}
		return idx, nil
	}

	if refine > 0 {
		return -1, fmt.Errorf("no item %s (+%d) found in inventory", itemID, refine)
	}
	return -1, fmt.Errorf("no item %s found in inventory", itemID)
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Send In-Game Mail Command
	r.Command(multi.CommandDef{
		Key:          "automation.transfer.send_mail",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      100 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p MailPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			to := strings.TrimSpace(p.To)
			if to == "" {
				return errors.New("recipient character name cannot be empty")
			}
			if strings.EqualFold(b.Name(), to) {
				return errors.New("cannot send mail to yourself")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p MailPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(TransferController)
			if !ok || inst == nil {
				return nil, errors.New("instance unavailable")
			}
			slot, err := resolveSlot(inst, b, p.FocusedID, p.SlotIndex, p.ItemID, p.Refine)
			if err != nil {
				return nil, err
			}
			return inst.SendMail(p.To, p.Title, p.Text, slot, p.Count, p.Penya)
		},
	})

	// 2. Start Direct Trade Command
	r.Command(multi.CommandDef{
		Key:          "automation.transfer.start_trade",
		Scope:        multi.AllSelected,
		Mode:         multi.Sequential,
		Timeout:      20 * time.Second,
		Exclusive:    "trade",
		LockRefs:     true,
		AllowOffline: false,
		RefsBots: func(payload json.RawMessage) []multi.BotRef {
			var p TradePayload
			_ = json.Unmarshal(payload, &p)
			tgt := strings.TrimSpace(p.Target)
			if tgt != "" {
				return []multi.BotRef{{Name: tgt}}
			}
			return nil
		},
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p TradePayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			tgt := strings.TrimSpace(p.Target)
			if tgt == "" {
				return errors.New("target player name cannot be empty")
			}
			if strings.EqualFold(b.Name(), tgt) {
				return errors.New("cannot initiate trade with yourself")
			}
			return nil
		},
		ValidateRef: func(sender, ref multi.Bot, payload json.RawMessage) error {
			if ref == nil || !ref.Online() {
				return fmt.Errorf("trade partner %s is offline", ref.Name())
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p TradePayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(TransferController)
			if !ok || inst == nil {
				return nil, errors.New("instance unavailable")
			}
			slot, err := resolveSlot(inst, b, p.FocusedID, p.SlotIndex, p.ItemID, p.Refine)
			if err != nil {
				return nil, err
			}
			return inst.StartTrade(p.Target, slot, p.Count, p.Penya)
		},
	})
}
