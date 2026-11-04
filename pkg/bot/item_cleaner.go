package bot

import (
	"strings"

	"madrigal-multibot/pkg/models"
)

const (
	// InvSize is the exact inventory size used by the official game client (scripts/stats.gd:18).
	InvSize = 112
	// DefaultMaxPenya is the safe default penya cap (299k) before routing items to auto-trash.
	DefaultMaxPenya = 299000
	// HardMaxPenya is the maximum penya cap before the server rejects save deltas (scripts/commands.gd:359).
	HardMaxPenya = 2000000000
)

// Standard allowable equipment slots for auto-cleaning/selling normal monster drop gear.
var standardEquipSlots = map[string]bool{
	"weapon": true,
	"shield": true,
	"helmet": true,
	"suit":   true,
	"gloves": true,
	"boots":  true,
}

// Protected item categories (k3 values) that must NEVER be auto-junked or auto-sold.
var protectedK3Categories = map[string]bool{
	"IK3_EGG":           true,
	"IK3_PET":           true,
	"IK3_NEW_PET":       true,
	"IK3_MONSTER_ALBUM": true,
	"IK3_POCKET":        true,
}

// CleanRules specifies the user configuration for automatic inventory cleanup.
type CleanRules struct {
	AutoCleanJunk       bool            `json:"auto_clean_junk"`
	AutoSellBelowRarity int             `json:"auto_sell_below_rarity"` // 0 = disabled, 120 = normal gear
	MaxPenyaCap         int             `json:"max_penya_cap"`          // e.g. 299000 (default 299k)
	Whitelist           map[string]bool `json:"whitelist"`
}

// EffectiveMaxPenya returns the active penya cap clamped between 0 and HardMaxPenya (2B).
func (r CleanRules) EffectiveMaxPenya() int {
	if r.MaxPenyaCap > 0 {
		if r.MaxPenyaCap > HardMaxPenya {
			return HardMaxPenya
		}
		return r.MaxPenyaCap
	}
	return DefaultMaxPenya
}

// CleanAction indicates the decision for an inventory slot.
type CleanAction int

const (
	ActionKeep CleanAction = iota
	ActionTrash
	ActionSell
)

// CleanDecision captures the action and financial delta for a slot.
type CleanDecision struct {
	Action     CleanAction `json:"action"`
	UnitPrice  int         `json:"unit_price"`
	TotalPrice int         `json:"total_price"`
	Count      int         `json:"count"`
	Reason     string      `json:"reason"`
}

// CleanSummary aggregates the results of a clean pass.
type CleanSummary struct {
	SoldCount    int      `json:"sold_count"`
	TrashedCount int      `json:"trashed_count"`
	PenyaGained  int      `json:"penya_gained"`
	SoldItems    []string `json:"sold_items"`
	TrashedItems []string `json:"trashed_items"`
}

// IsEmptySlot returns true if a slot contains no item.
func IsEmptySlot(slot interface{}) bool {
	if slot == nil {
		return true
	}
	m, ok := slot.(map[string]interface{})
	if !ok || len(m) == 0 {
		return true
	}
	id, _ := m["id"].(string)
	return strings.TrimSpace(id) == ""
}

// NewEmptySlot produces an empty JSON object `{}` matching scripts/stats.gd:102.
func NewEmptySlot() map[string]interface{} {
	return map[string]interface{}{}
}

// NormalizeInventory ensures the inventory slice has fixed length 112 (or preserves
// larger slices up to current length) with all empty slots serialized as `{}` and never `nil`.
func NormalizeInventory(invList []interface{}) []interface{} {
	targetSize := InvSize
	if len(invList) > targetSize {
		targetSize = len(invList)
	}
	res := make([]interface{}, targetSize)
	for i := 0; i < targetSize; i++ {
		if i < len(invList) && !IsEmptySlot(invList[i]) {
			if m, ok := invList[i].(map[string]interface{}); ok {
				res[i] = cloneSlotMap(m)
			} else {
				res[i] = invList[i]
			}
		} else {
			res[i] = NewEmptySlot()
		}
	}
	return res
}

// cloneSlotMap performs a shallow copy of a slot map.
func cloneSlotMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return NewEmptySlot()
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// IsProtectedSlot implements Ruta 1 (Complete Protection Predicate).
// Returns true if the item or slot must NEVER be touched by auto-clean/auto-sell.
func IsProtectedSlot(slot map[string]interface{}, meta *models.ItemDef, charData map[string]interface{}, whitelist map[string]bool) bool {
	if slot == nil || len(slot) == 0 {
		return false
	}
	id, _ := slot["id"].(string)
	if id == "" {
		return false
	}

	// 1. User whitelist check
	if whitelist != nil && (whitelist[id] || whitelist[strings.ToLower(id)]) {
		return true
	}

	if meta == nil {
		meta = models.GetItem(id)
	}
	// Unknown items in database are protected for safety
	if meta == nil {
		return true
	}

	// 2. Slot-level protection attributes
	// Locked
	if locked, _ := slot["lk"].(bool); locked {
		return true
	}
	// Timed / rental / active tickets
	if until := slot["until"]; until != nil {
		return true
	}
	// Pet / caught monster pet
	if pet := slot["pet"]; pet != nil {
		return true
	}
	if cp := slot["cp"]; cp != nil {
		return true
	}
	// Upgraded / refined gear
	if upVal, exists := slot["up"]; exists {
		switch v := upVal.(type) {
		case float64:
			if v > 0 {
				return true
			}
		case int:
			if v > 0 {
				return true
			}
		}
	}
	// Elemental upgrade
	if el, exists := slot["el"]; exists && el != nil && el != "" && el != float64(0) && el != 0 {
		return true
	}
	// Awakenings ("ro")
	if ro, exists := slot["ro"]; exists && ro != nil {
		if roMap, ok := ro.(map[string]interface{}); ok && len(roMap) > 0 {
			return true
		}
		if roArr, ok := ro.([]interface{}); ok && len(roArr) > 0 {
			return true
		}
	}
	// Piercings ("pc")
	if pc, exists := slot["pc"]; exists && pc != nil {
		if pcArr, ok := pc.([]interface{}); ok && len(pcArr) > 0 {
			return true
		}
		if pcMap, ok := pc.(map[string]interface{}); ok && len(pcMap) > 0 {
			return true
		}
	}
	// Ultimate jewels ("uj")
	if uj, exists := slot["uj"]; exists && uj != nil {
		if ujArr, ok := uj.([]interface{}); ok && len(ujArr) > 0 {
			return true
		}
		if ujMap, ok := uj.(map[string]interface{}); ok && len(ujMap) > 0 {
			return true
		}
	}
	// Pity counter ("gu")
	if guVal, exists := slot["gu"]; exists {
		switch v := guVal.(type) {
		case float64:
			if v > 0 {
				return true
			}
		case int:
			if v > 0 {
				return true
			}
		}
	}
	// SwitchEq preset ("eqid")
	if eqid, exists := slot["eqid"]; exists && eqid != nil && eqid != "" && eqid != float64(0) && eqid != 0 {
		return true
	}
	// Level down reduction ("ld")
	if ldVal, exists := slot["ld"]; exists {
		switch v := ldVal.(type) {
		case float64:
			if v != 0 {
				return true
			}
		case int:
			if v != 0 {
				return true
			}
		}
	}
	// Active deployed pets (pet_out, looter_out, catch_out)
	if uid, ok := slot["uid"].(string); ok && uid != "" && charData != nil {
		if petOut, _ := charData["pet_out"].(string); petOut != "" && petOut == uid {
			return true
		}
		if looterOut, _ := charData["looter_out"].(string); looterOut != "" && looterOut == uid {
			return true
		}
		if catchOut, _ := charData["catch_out"].(string); catchOut != "" && catchOut == uid {
			return true
		}
	}

	// 3. Item definition protection rules
	// Non-standard equipment slots: protect ride, wings, cloak, mask, cosmetics, jewelry
	slotName := strings.ToLower(strings.TrimSpace(meta.Slot))
	if slotName != "" && !standardEquipSlots[slotName] {
		return true
	}

	// Unique/Ultimate items, Sets, Rare >= 200
	if meta.Ult != "" || meta.Set != "" || strings.Contains(id, "_SET_") || meta.Rare >= 200 {
		return true
	}

	// Protected K3 categories
	if protectedK3Categories[meta.K3] {
		return true
	}

	// ID prefix protections: pets, unused Azria tickets, monster album cards
	if strings.HasPrefix(id, "II_PET_") ||
		strings.HasPrefix(id, "II_SYS_TICKET_KEBARAS") ||
		strings.HasPrefix(id, "II_MONSTERALBUM_") {
		return true
	}

	return false
}

// IsEligibleForDisposal checks if an item is considered for clean/sell.
// An item must FIRST be eligible before evaluating Ruta 2 or Ruta 3.
func IsEligibleForDisposal(slot map[string]interface{}, meta *models.ItemDef, rules CleanRules) bool {
	if slot == nil || len(slot) == 0 {
		return false
	}
	id, _ := slot["id"].(string)
	if id == "" {
		return false
	}

	// 1. Explicitly marked as junk in inventory slot
	if isJunk, _ := slot["jk"].(bool); isJunk {
		return true
	}

	if meta == nil {
		meta = models.GetItem(id)
	}
	if meta == nil {
		return false
	}

	// 2. AutoCleanJunk enabled: match general materials / monster drops
	if rules.AutoCleanJunk {
		if meta.K1 == "IK1_GENERAL" || meta.K2 == "IK2_MATERIAL" || strings.HasPrefix(id, "II_GEN_MAT_") {
			return true
		}
	}

	// 3. AutoSellBelowRarity enabled: match standard gear with 1 <= Rare <= threshold
	if rules.AutoSellBelowRarity > 0 {
		slotName := strings.ToLower(strings.TrimSpace(meta.Slot))
		if standardEquipSlots[slotName] && meta.Rare > 0 && meta.Rare <= rules.AutoSellBelowRarity {
			return true
		}
	}

	return false
}

// DecideCleanAction determines whether a slot is Kept, Trashed (0 Penya), or Sold.
func DecideCleanAction(slot map[string]interface{}, meta *models.ItemDef, currentPenya int, rules CleanRules, charData map[string]interface{}) CleanDecision {
	if IsEmptySlot(slot) {
		return CleanDecision{Action: ActionKeep, Reason: "empty"}
	}

	id, _ := slot["id"].(string)
	if meta == nil {
		meta = models.GetItem(id)
	}

	// Ruta 1: Protected
	if IsProtectedSlot(slot, meta, charData, rules.Whitelist) {
		return CleanDecision{Action: ActionKeep, Reason: "protected"}
	}

	// Must be eligible for disposal
	if !IsEligibleForDisposal(slot, meta, rules) {
		return CleanDecision{Action: ActionKeep, Reason: "not eligible"}
	}

	count := 1
	if nVal, exists := slot["n"]; exists {
		switch v := nVal.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		}
	}
	if count <= 0 {
		count = 1
	}

	cost := 0
	if meta != nil {
		cost = meta.Cost
	}

	// Official formula: integer division (it.cost / 4)
	unitSellPrice := cost / 4
	totalPrice := unitSellPrice * count

	isBound, _ := slot["bd"].(bool)
	isQuest := meta != nil && meta.K3 == "IK3_QUEST"

	effectiveCap := rules.EffectiveMaxPenya()

	// Ruta 2: Auto Trash (Zero Penya Delta)
	// Triggered if Cost < 4 (no resale value), bound, quest item, or penya cap overflow
	if cost < 4 || isBound || isQuest || (currentPenya+totalPrice > effectiveCap) {
		reason := "zero cost"
		if cost >= 4 {
			if isBound {
				reason = "bound"
			} else if isQuest {
				reason = "quest"
			} else if currentPenya+totalPrice > effectiveCap {
				reason = "penya cap"
			}
		}
		return CleanDecision{
			Action:     ActionTrash,
			UnitPrice:  0,
			TotalPrice: 0,
			Count:      count,
			Reason:     reason,
		}
	}

	// Ruta 3: Auto Sell (Valid Penya Delta)
	return CleanDecision{
		Action:     ActionSell,
		UnitPrice:  unitSellPrice,
		TotalPrice: totalPrice,
		Count:      count,
		Reason:     "valid sell",
	}
}

// ProcessAutoCleanPass runs an atomic in-memory clean pass on charData.
// It normalizes the inventory to fixed size 112, respects blacklist, and returns a summary.
func ProcessAutoCleanPass(charData map[string]interface{}, rules CleanRules, sessionBlacklist map[string]bool) CleanSummary {
	var summary CleanSummary
	if charData == nil {
		return summary
	}

	invRaw, ok := charData["inv"].([]interface{})
	if !ok {
		return summary
	}

	invList := NormalizeInventory(invRaw)
	currentPenya := 0
	if pVal, exists := charData["penya"]; exists {
		switch v := pVal.(type) {
		case float64:
			currentPenya = int(v)
		case int:
			currentPenya = v
		}
	}

	for i := 0; i < len(invList); i++ {
		slot, ok := invList[i].(map[string]interface{})
		if !ok || IsEmptySlot(slot) {
			continue
		}

		id, _ := slot["id"].(string)
		if id == "" {
			continue
		}

		// Skip blacklisted items that previously caused save refusal
		if sessionBlacklist != nil && sessionBlacklist[id] {
			continue
		}

		meta := models.GetItem(id)
		decision := DecideCleanAction(slot, meta, currentPenya, rules, charData)

		switch decision.Action {
		case ActionTrash:
			invList[i] = NewEmptySlot()
			summary.TrashedCount += decision.Count
			summary.TrashedItems = append(summary.TrashedItems, id)

		case ActionSell:
			currentPenya += decision.TotalPrice
			summary.PenyaGained += decision.TotalPrice
			summary.SoldCount += decision.Count
			summary.SoldItems = append(summary.SoldItems, id)
			invList[i] = NewEmptySlot()

		case ActionKeep:
			// Do nothing
		}
	}

	charData["inv"] = invList
	charData["penya"] = float64(currentPenya)
	return summary
}
