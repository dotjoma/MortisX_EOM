package models

import (
	_ "embed"
	"encoding/json"
	"log"
	"strings"
	"sync"
)

//go:embed item_stacks.json
var itemStacksJSON []byte

var (
	itemStackOnce   sync.Once
	itemStackLimits map[string]int
)

func initItemStackLimits() {
	itemStackOnce.Do(func() {
		itemStackLimits = make(map[string]int, 14000)
		if len(itemStacksJSON) > 0 {
			_ = json.Unmarshal(itemStacksJSON, &itemStackLimits)
		}
	})
}

// GetItemStackLimit returns the maximum stack limit for an item from embedded data,
// or safe fallback heuristics matching FlyFF / Madrigal rules.
func GetItemStackLimit(itemID string) int {
	initItemStackLimits()
	if limit, exists := itemStackLimits[itemID]; exists && limit > 0 {
		return limit
	}
	// Fallback heuristics: equipment, weapons, suits, mounts, pets, cloaks, jewelry have stack limit 1.
	if strings.HasPrefix(itemID, "II_ARM_") ||
		strings.HasPrefix(itemID, "II_WEA_") ||
		strings.HasPrefix(itemID, "II_GEN_JEW_") ||
		strings.HasPrefix(itemID, "II_PET_") ||
		strings.HasPrefix(itemID, "II_CLO_") ||
		strings.HasPrefix(itemID, "II_CHR_") ||
		(strings.HasPrefix(itemID, "II_RID_") && !strings.HasPrefix(itemID, "II_RID_AFU_")) {
		return 1
	}
	if strings.HasPrefix(itemID, "II_GEN_") || strings.HasPrefix(itemID, "II_SYS_") || strings.HasPrefix(itemID, "II_RID_AFU_") {
		return 9999
	}
	return 1
}

// ValidateAndRepairInventory scans every slot in invList.
// If any slot has count > maxStack (e.g. non-stackable equipment x2), it clamps the slot to
// maxStack and redistributes the excess into available empty slots (up to 240 slots).
// When no empty slot exists, it logs the dropped item ID and count.
// Returns true if any repairs were made.
func ValidateAndRepairInventory(invList []interface{}) bool {
	if invList == nil {
		return false
	}
	repaired := false
	for i := 0; i < len(invList); i++ {
		im, ok := invList[i].(map[string]interface{})
		if !ok || len(im) == 0 {
			continue
		}
		id, _ := im["id"].(string)
		if id == "" {
			continue
		}
		curN := 1
		if nVal, hasN := im["n"]; hasN {
			switch v := nVal.(type) {
			case float64:
				curN = int(v)
			case int:
				curN = v
			}
		}
		if curN <= 0 {
			curN = 1
			im["n"] = 1
			repaired = true
		}

		maxStack := GetItemStackLimit(id)
		if curN > maxStack {
			repaired = true
			im["n"] = maxStack
			invList[i] = im
			excess := curN - maxStack

			// Distribute excess into empty slots
			for excess > 0 {
				freeIdx := -1
				for j := 0; j < len(invList); j++ {
					jm, ok := invList[j].(map[string]interface{})
					if !ok || len(jm) == 0 || jm["id"] == nil || jm["id"] == "" {
						freeIdx = j
						break
					}
				}
				put := excess
				if put > maxStack {
					put = maxStack
				}
				newSlot := map[string]interface{}{
					"id": id,
					"n":  put,
				}
				if upVal, hasUp := im["up"]; hasUp {
					newSlot["up"] = upVal
				}
				if freeIdx >= 0 {
					invList[freeIdx] = newSlot
				} else if len(invList) < 240 {
					invList = append(invList, newSlot)
				} else {
					// Cannot fit further excess, log warning and clamp
					log.Printf("[INVENTORY WARN] Cannot split stack for %s: inventory full (240 slots). Dropped/clamped excess count: %d", id, excess)
					break
				}
				excess -= put
			}
		}
	}
	return repaired
}
