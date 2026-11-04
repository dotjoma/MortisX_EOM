package bot

import (
	"fmt"
	"math"
	"strings"

	"madrigal-multibot/pkg/models"
)

// Helper: check if a weapon is valid for Blade dual-wielding (1H sword or 1H axe)
func isDualOk(item *models.ItemDef) bool {
	if item == nil {
		return false
	}
	return (item.K3 == "IK3_SWD" || item.K3 == "IK3_AXE") && !models.IsTwoHanded(item)
}

// getOrCreateMap safely returns a nested map[string]interface{}
func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if m, ok := parent[key].(map[string]interface{}); ok && m != nil {
		return m
	}
	m := make(map[string]interface{})
	parent[key] = m
	return m
}

// findEmptySlot locates the first unoccupied slot in the inventory array (0..41)
func findEmptySlot(invList []interface{}) int {
	for i := 0; i < len(invList); i++ {
		im, ok := invList[i].(map[string]interface{})
		if !ok || len(im) == 0 || im["id"] == nil || im["id"] == "" {
			return i
		}
	}
	return -1
}

// countEmptySlots returns the number of free slots in the inventory array
func countEmptySlots(invList []interface{}) int {
	cnt := 0
	for i := 0; i < len(invList); i++ {
		im, ok := invList[i].(map[string]interface{})
		if !ok || len(im) == 0 || im["id"] == nil || im["id"] == "" {
			cnt++
		}
	}
	return cnt
}

// calculateMaxHP computes an approximate max HP based on job, level, STA, and gear for clamping
func calculateMaxHP(charData map[string]interface{}) int {
	level := toInt(charData["level"])
	if level <= 0 {
		level = 1
	}
	sta := toInt(charData["sta"])
	if sta <= 0 {
		sta = 15
	}
	baseJob := models.BaseJob(toString(charData["job"]))

	hpFactor := 1.0
	switch baseJob {
	case "knight":
		hpFactor = 2.0
	case "blade", "billposter":
		hpFactor = 1.5
	case "ranger", "jester":
		hpFactor = 1.3
	case "psykeeper", "elementor", "ringmaster":
		hpFactor = 1.2
	default:
		hpFactor = 1.0
	}

	maxHP := int(float64(sta*level)*hpFactor*2.5) + 200
	if maxHP < 50 {
		maxHP = 50
	}
	return maxHP
}

// EquipFromInventory performs an atomic, client-authoritative equip swap matching FStats.equip_from.
// Handles parallel metadata (up, el, pc, ro, uj, gu, eqid, ld), 2H weapon vs shield conflicts,
// Blade dual-wielding, dual ring/earring slots, and HP clamping.
func EquipFromInventory(charData map[string]interface{}, slotIdx int, wantSlot string) (string, error) {
	invList, ok := charData["inv"].([]interface{})
	if !ok || slotIdx < 0 || slotIdx >= len(invList) {
		return "", fmt.Errorf("invalid slot index %d", slotIdx)
	}

	itemMap, ok := invList[slotIdx].(map[string]interface{})
	if !ok || len(itemMap) == 0 {
		return "", fmt.Errorf("slot %d is empty", slotIdx)
	}

	itemID := toString(itemMap["id"])
	if itemID == "" {
		return "", fmt.Errorf("item has no ID")
	}

	itemDef := models.GetItem(itemID)
	if itemDef == nil || !models.IsEquip(itemDef) {
		return "", fmt.Errorf("item %s is not equippable", itemID)
	}

	playerJob := toString(charData["job"])
	baseJob := models.BaseJob(playerJob)
	playerGender := toString(charData["gender"])
	playerLevel := toInt(charData["level"])
	levelDelta := toInt(itemMap["ld"])

	equippedMap := getOrCreateMap(charData, "equip")
	currWeaponID := toString(equippedMap["weapon"])
	currWeaponDef := models.GetItem(currWeaponID)

	can, reason := models.CanEquip(playerJob, playerGender, playerLevel, itemDef, levelDelta, currWeaponDef)
	if !can {
		return "", fmt.Errorf("%s", reason)
	}

	// Target Slot Resolution
	targetSlot := wantSlot
	if targetSlot == "" {
		switch itemDef.Slot {
		case "ring":
			r1 := toString(equippedMap["ring1"])
			r2 := toString(equippedMap["ring2"])
			if r1 != "" && r2 == "" {
				targetSlot = "ring2"
			} else if r1 == "" {
				targetSlot = "ring1"
			} else {
				// Both occupied: replace the lower-scoring ring
				s1 := getEquippedSlotScore(charData, "ring1", playerJob)
				s2 := getEquippedSlotScore(charData, "ring2", playerJob)
				if s1 <= s2 {
					targetSlot = "ring1"
				} else {
					targetSlot = "ring2"
				}
			}
		case "earring":
			e1 := toString(equippedMap["earring1"])
			e2 := toString(equippedMap["earring2"])
			if e1 != "" && e2 == "" {
				targetSlot = "earring2"
			} else if e1 == "" {
				targetSlot = "earring1"
			} else {
				// Both occupied: replace the lower-scoring earring
				s1 := getEquippedSlotScore(charData, "earring1", playerJob)
				s2 := getEquippedSlotScore(charData, "earring2", playerJob)
				if s1 <= s2 {
					targetSlot = "earring1"
				} else {
					targetSlot = "earring2"
				}
			}
		default:
			if baseJob == "blade" && itemDef.K1 == "IK1_WEAPON" && isDualOk(itemDef) {
				if isDualOk(currWeaponDef) && toString(equippedMap["shield"]) == "" {
					targetSlot = "shield"
				} else {
					targetSlot = itemDef.Slot
				}
			} else {
				targetSlot = itemDef.Slot
			}
		}
	}

	if targetSlot == "" {
		return "", fmt.Errorf("unable to resolve target slot for %s", itemID)
	}

	// 2H Weapon Conflict: If equipping 2H weapon into "weapon", unequip "shield" if present
	if targetSlot == "weapon" && models.IsTwoHanded(itemDef) {
		shieldID := toString(equippedMap["shield"])
		if shieldID != "" {
			oldWeaponID := toString(equippedMap["weapon"])
			freeSlots := countEmptySlots(invList)
			// If swapping with an existing weapon, slotIdx will be used by old weapon, so we need at least 1 OTHER free slot
			if oldWeaponID != "" && freeSlots < 1 {
				return "", fmt.Errorf("inventory is full, cannot unequip offhand shield")
			}
			if err := UnequipToInventory(charData, "shield"); err != nil {
				return "", fmt.Errorf("failed to unequip shield: %w", err)
			}
		}
	}

	// Parallel maps
	equipUp := getOrCreateMap(charData, "equip_up")
	equipEl := getOrCreateMap(charData, "equip_el")
	equipPc := getOrCreateMap(charData, "equip_pc")
	equipRo := getOrCreateMap(charData, "equip_ro")
	equipX := getOrCreateMap(charData, "equip_x")

	// Extract candidate item metadata
	newUp := toInt(itemMap["up"])
	newEl := toString(itemMap["el"])
	newPc, _ := itemMap["pc"].([]interface{})
	newRo, _ := itemMap["ro"].([]interface{})

	newX := make(map[string]interface{})
	for _, k := range []string{"uj", "gu", "eqid", "ld"} {
		if v, has := itemMap[k]; has && v != nil {
			newX[k] = v
		}
	}

	// Handle swap with previously equipped item
	prevID := toString(equippedMap[targetSlot])
	invList[slotIdx] = map[string]interface{}{}

	if prevID != "" {
		swappedItem := map[string]interface{}{
			"id": prevID,
			"n":  1,
		}
		if upVal := toInt(equipUp[targetSlot]); upVal > 0 {
			swappedItem["up"] = upVal
		}
		if elVal := toString(equipEl[targetSlot]); elVal != "" {
			swappedItem["el"] = elVal
		}
		if pcVal, ok := equipPc[targetSlot].([]interface{}); ok && len(pcVal) > 0 {
			swappedItem["pc"] = pcVal
		}
		if roVal, ok := equipRo[targetSlot].([]interface{}); ok && len(roVal) > 0 {
			swappedItem["ro"] = roVal
		}
		if xVal, ok := equipX[targetSlot].(map[string]interface{}); ok {
			for xk, xv := range xVal {
				swappedItem[xk] = xv
			}
		}
		invList[slotIdx] = swappedItem
	}

	// Assign new item to slot
	equippedMap[targetSlot] = itemID

	if newUp > 0 {
		equipUp[targetSlot] = newUp
	} else {
		delete(equipUp, targetSlot)
	}

	if newEl != "" {
		equipEl[targetSlot] = newEl
	} else {
		delete(equipEl, targetSlot)
	}

	if len(newPc) > 0 {
		equipPc[targetSlot] = newPc
	} else {
		delete(equipPc, targetSlot)
	}

	if len(newRo) > 0 {
		equipRo[targetSlot] = newRo
	} else {
		delete(equipRo, targetSlot)
	}

	if len(newX) > 0 {
		equipX[targetSlot] = newX
	} else {
		delete(equipX, targetSlot)
	}

	// Clean up offhand if weapon equipped is 2H or non-blade
	if targetSlot == "weapon" {
		if models.IsTwoHanded(itemDef) || baseJob != "blade" {
			offhandID := toString(equippedMap["shield"])
			if offhandDef := models.GetItem(offhandID); offhandDef != nil && offhandDef.K1 == "IK1_WEAPON" {
				_ = UnequipToInventory(charData, "shield")
			}
		}
	}

	// Clamp HP to prevent overflow
	maxHP := calculateMaxHP(charData)
	curHP := toInt(charData["hp"])
	if curHP > maxHP {
		charData["hp"] = maxHP
	}

	return targetSlot, nil
}

// UnequipToInventory removes an equipped item and places it into the first free inventory slot.
func UnequipToInventory(charData map[string]interface{}, slotKey string) error {
	equippedMap, ok := charData["equip"].(map[string]interface{})
	if !ok || equippedMap == nil {
		return fmt.Errorf("no equipment map found")
	}

	itemID := toString(equippedMap[slotKey])
	if itemID == "" {
		return fmt.Errorf("slot %s is empty", slotKey)
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok {
		invList = make([]interface{}, 0, 42)
	}

	freeIdx := findEmptySlot(invList)
	if freeIdx < 0 {
		if len(invList) < 42 {
			freeIdx = len(invList)
			invList = append(invList, map[string]interface{}{})
			charData["inv"] = invList
		} else {
			return fmt.Errorf("inventory is full")
		}
	}

	equipUp := getOrCreateMap(charData, "equip_up")
	equipEl := getOrCreateMap(charData, "equip_el")
	equipPc := getOrCreateMap(charData, "equip_pc")
	equipRo := getOrCreateMap(charData, "equip_ro")
	equipX := getOrCreateMap(charData, "equip_x")

	restoredItem := map[string]interface{}{
		"id": itemID,
		"n":  1,
	}

	if upVal := toInt(equipUp[slotKey]); upVal > 0 {
		restoredItem["up"] = upVal
	}
	if elVal := toString(equipEl[slotKey]); elVal != "" {
		restoredItem["el"] = elVal
	}
	if pcVal, ok := equipPc[slotKey].([]interface{}); ok && len(pcVal) > 0 {
		restoredItem["pc"] = pcVal
	}
	if roVal, ok := equipRo[slotKey].([]interface{}); ok && len(roVal) > 0 {
		restoredItem["ro"] = roVal
	}
	if xVal, ok := equipX[slotKey].(map[string]interface{}); ok {
		for xk, xv := range xVal {
			restoredItem[xk] = xv
		}
	}

	invList[freeIdx] = restoredItem
	delete(equippedMap, slotKey)
	delete(equipUp, slotKey)
	delete(equipEl, slotKey)
	delete(equipPc, slotKey)
	delete(equipRo, slotKey)
	delete(equipX, slotKey)

	maxHP := calculateMaxHP(charData)
	curHP := toInt(charData["hp"])
	if curHP > maxHP {
		charData["hp"] = maxHP
	}

	return nil
}

// getEquippedSlotScore computes the gear score of currently equipped gear in a slot
func getEquippedSlotScore(charData map[string]interface{}, slotKey string, playerJob string) float64 {
	equippedMap := getOrCreateMap(charData, "equip")
	itemID := toString(equippedMap[slotKey])
	if itemID == "" {
		return 0.0
	}
	itemDef := models.GetItem(itemID)
	if itemDef == nil {
		return 0.0
	}

	equipUp := getOrCreateMap(charData, "equip_up")
	equipEl := getOrCreateMap(charData, "equip_el")
	equipPc := getOrCreateMap(charData, "equip_pc")
	equipRo := getOrCreateMap(charData, "equip_ro")

	up := toInt(equipUp[slotKey])
	elem := toString(equipEl[slotKey])
	pc, _ := equipPc[slotKey].([]interface{})
	ro, _ := equipRo[slotKey].([]interface{})

	var pcStrs []string
	for _, p := range pc {
		if ps := toString(p); ps != "" {
			pcStrs = append(pcStrs, ps)
		}
	}
	var roPairs [][]interface{}
	for _, r := range ro {
		if rArr, ok := r.([]interface{}); ok && len(rArr) >= 2 {
			roPairs = append(roPairs, rArr)
		}
	}

	return models.CalculateGearScore(itemDef, up, elem, roPairs, pcStrs, playerJob)
}

// RunSmartAutomationTick executes the automated gear upgrade and smart inventory loops
func (b *BotInstance) RunSmartAutomationTick() {
	b.mu.Lock()
	if b.Status != "online" || b.ws == nil || !b.ws.IsConnected() {
		b.mu.Unlock()
		return
	}
	prefs := b.Profile.Preferences
	b.mu.Unlock()

	// Skip if all automation toggles are disabled
	if !prefs.AutoEquipUpgrades &&
		!prefs.AutoEquipArrows &&
		!prefs.AutoJunkInferior &&
		!prefs.AutoLockRareGear &&
		!prefs.AutoManagePet &&
		!prefs.AutoUsePowerups &&
		!prefs.AutoRefuelFlight {
		return
	}

	charData, charFile, token, err := b.loadCharData()
	if err != nil || charData == nil {
		return
	}

	invList, ok := charData["inv"].([]interface{})
	if !ok || len(invList) == 0 {
		return
	}

	playerJob := toString(charData["job"])
	baseJob := models.BaseJob(playerJob)
	playerGender := toString(charData["gender"])
	playerLevel := toInt(charData["level"])
	equippedMap := getOrCreateMap(charData, "equip")

	dirty := false
	equipChanged := false

	// 1. AutoLockRareGear: Lock gear with up >= 6 or positive awakenings
	if prefs.AutoLockRareGear {
		for i := 0; i < len(invList); i++ {
			im, ok := invList[i].(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			id := toString(im["id"])
			if id == "" {
				continue
			}
			isLocked := toBool(im["lk"])
			if isLocked {
				continue
			}
			up := toInt(im["up"])
			ro, _ := im["ro"].([]interface{})

			shouldLock := up >= 6
			if !shouldLock && len(ro) > 0 {
				for _, r := range ro {
					if rArr, ok := r.([]interface{}); ok && len(rArr) >= 2 {
						if toFloat64(rArr[1]) > 0 {
							shouldLock = true
							break
						}
					}
				}
			}

			if shouldLock {
				im["lk"] = true
				invList[i] = im
				dirty = true
				itemDef := models.GetItem(id)
				name := id
				if itemDef != nil && itemDef.Name != "" {
					name = itemDef.Name
				}
				b.emitLog("action", fmt.Sprintf("[AUTO-LOCK] Protected rare item: %s (+%d / Awakened)", name, up), "info")
			}
		}
	}

	// 2. AutoEquipUpgrades: Upgrade gear if better item found in inventory
	if prefs.AutoEquipUpgrades {
		currWeaponID := toString(equippedMap["weapon"])
		currWeaponDef := models.GetItem(currWeaponID)
		currWeaponK3 := ""
		if currWeaponDef != nil {
			currWeaponK3 = currWeaponDef.K3
		}

		// Find best upgrade candidate
		bestSlotIdx := -1
		bestTargetSlot := ""
		bestScoreDiff := 0.0
		var bestCandidateDef *models.ItemDef

		for i := 0; i < len(invList); i++ {
			im, ok := invList[i].(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			id := toString(im["id"])
			if id == "" {
				continue
			}
			itemDef := models.GetItem(id)
			if itemDef == nil || !models.IsEquip(itemDef) {
				continue
			}

			levelDelta := toInt(im["ld"])
			can, _ := models.CanEquip(playerJob, playerGender, playerLevel, itemDef, levelDelta, currWeaponDef)
			if !can {
				continue
			}

			// LockWeaponType: Do not swap weapon class (e.g. Bow vs Yoyo)
			if (itemDef.Slot == "weapon" || itemDef.K1 == "IK1_WEAPON") && prefs.LockWeaponType && currWeaponK3 != "" {
				if itemDef.K3 != currWeaponK3 {
					continue
				}
			}

			up := toInt(im["up"])
			elem := toString(im["el"])
			pc, _ := im["pc"].([]interface{})
			ro, _ := im["ro"].([]interface{})

			var pcStrs []string
			for _, p := range pc {
				if ps := toString(p); ps != "" {
					pcStrs = append(pcStrs, ps)
				}
			}
			var roPairs [][]interface{}
			for _, r := range ro {
				if rArr, ok := r.([]interface{}); ok && len(rArr) >= 2 {
					roPairs = append(roPairs, rArr)
				}
			}

			candidateScore := models.CalculateGearScore(itemDef, up, elem, roPairs, pcStrs, playerJob)

			// Determine current slot and score
			targetSlot := itemDef.Slot
			if targetSlot == "ring" {
				s1 := getEquippedSlotScore(charData, "ring1", playerJob)
				s2 := getEquippedSlotScore(charData, "ring2", playerJob)
				if s1 <= s2 {
					targetSlot = "ring1"
				} else {
					targetSlot = "ring2"
				}
			} else if targetSlot == "earring" {
				s1 := getEquippedSlotScore(charData, "earring1", playerJob)
				s2 := getEquippedSlotScore(charData, "earring2", playerJob)
				if s1 <= s2 {
					targetSlot = "earring1"
				} else {
					targetSlot = "earring2"
				}
			} else if baseJob == "blade" && itemDef.K1 == "IK1_WEAPON" && isDualOk(itemDef) && isDualOk(currWeaponDef) {
				if toString(equippedMap["shield"]) == "" {
					targetSlot = "shield"
				}
			}

			currentScore := getEquippedSlotScore(charData, targetSlot, playerJob)
			diff := candidateScore - currentScore

			// Strict improvement threshold (+0.5 score)
			if diff > 0.5 && diff > bestScoreDiff {
				bestScoreDiff = diff
				bestSlotIdx = i
				bestTargetSlot = targetSlot
				bestCandidateDef = itemDef
			}
		}

		if bestSlotIdx >= 0 && bestTargetSlot != "" {
			oldItemName := toString(equippedMap[bestTargetSlot])
			if oldDef := models.GetItem(oldItemName); oldDef != nil && oldDef.Name != "" {
				oldItemName = oldDef.Name
			}
			newItemName := bestCandidateDef.Name

			swappedSlot, err := EquipFromInventory(charData, bestSlotIdx, bestTargetSlot)
			if err == nil {
				dirty = true
				equipChanged = true
				b.emitLog("action", fmt.Sprintf("[AUTO-EQUIP] Upgraded %s: equipped %s (score +%.1f)", swappedSlot, newItemName, bestScoreDiff), "info")
			}
		}
	}

	// 3. AutoEquipArrows: If using Bow and arrow slot is empty, equip arrows
	if prefs.AutoEquipArrows {
		currWpnID := toString(equippedMap["weapon"])
		currWpnDef := models.GetItem(currWpnID)
		if currWpnDef != nil && currWpnDef.K3 == "IK3_BOW" {
			bulletID := toString(equippedMap["bullet"])
			if bulletID == "" {
				for i := 0; i < len(invList); i++ {
					im, ok := invList[i].(map[string]interface{})
					if !ok || len(im) == 0 {
						continue
					}
					id := toString(im["id"])
					if it := models.GetItem(id); it != nil && it.K3 == "IK3_ARROW" {
						_, err := EquipFromInventory(charData, i, "bullet")
						if err == nil {
							dirty = true
							equipChanged = true
							b.emitLog("action", fmt.Sprintf("[AUTO-EQUIP] Equipped %s into arrow slot", it.Name), "info")
							break
						}
					}
				}
			}
		}
	}

	// 4. AutoJunkInferior: Mark strictly inferior gear as junk
	if prefs.AutoJunkInferior {
		for i := 0; i < len(invList); i++ {
			im, ok := invList[i].(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			id := toString(im["id"])
			if id == "" {
				continue
			}
			isLocked := toBool(im["lk"])
			isJunk := toBool(im["jk"])
			if isLocked || isJunk {
				continue
			}

			itemDef := models.GetItem(id)
			if itemDef == nil || !models.IsEquip(itemDef) {
				continue
			}

			targetSlot := itemDef.Slot
			if targetSlot == "ring" {
				targetSlot = "ring1"
			} else if targetSlot == "earring" {
				targetSlot = "earring1"
			}

			equippedScore := getEquippedSlotScore(charData, targetSlot, playerJob)
			if equippedScore <= 0 {
				continue
			}

			up := toInt(im["up"])
			elem := toString(im["el"])
			pc, _ := im["pc"].([]interface{})
			ro, _ := im["ro"].([]interface{})
			var pcStrs []string
			for _, p := range pc {
				if ps := toString(p); ps != "" {
					pcStrs = append(pcStrs, ps)
				}
			}
			var roPairs [][]interface{}
			for _, r := range ro {
				if rArr, ok := r.([]interface{}); ok && len(rArr) >= 2 {
					roPairs = append(roPairs, rArr)
				}
			}

			invScore := models.CalculateGearScore(itemDef, up, elem, roPairs, pcStrs, playerJob)

			// If inv item is strictly inferior by > 3 score points and not rare (+6 / awake)
			if invScore < (equippedScore - 3.0) && up < 6 && len(ro) == 0 {
				im["jk"] = true
				invList[i] = im
				dirty = true
			}
		}
	}

	// 5. AutoManagePet: Keep pets summoned & feed when low energy
	if prefs.AutoManagePet {
		looterOut := toInt(charData["looter_out"])
		petOut := toInt(charData["pet_out"])

		// Auto summon looter
		if looterOut == 0 {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				id := toString(im["id"])
				if it := models.GetItem(id); it != nil && it.K3 == "IK3_PET" {
					uid := toInt(im["uid"])
					if uid == 0 {
						uid = 100000000 + i
						im["uid"] = uid
					}
					charData["looter_out"] = uid
					dirty = true
					b.emitLog("action", fmt.Sprintf("[PET] Auto-summoned looter pet (%s)", it.Name), "info")
					break
				}
			}
		}

		// Auto summon raised stat pet
		if petOut == 0 {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				id := toString(im["id"])
				if id == "II_PET_EGG" || strings.HasPrefix(id, "II_PET_") {
					uid := toInt(im["uid"])
					if uid == 0 {
						uid = 200000000 + i
						im["uid"] = uid
					}
					charData["pet_out"] = uid
					dirty = true
					itName := "Pet"
					if it := models.GetItem(id); it != nil && it.Name != "" {
						itName = it.Name
					}
					b.emitLog("action", fmt.Sprintf("[PET] Auto-summoned raised pet (%s)", itName), "info")
					break
				}
			}
		}

		// Auto feed pet when energy is low (< 30)
		for i := 0; i < len(invList); i++ {
			im, ok := invList[i].(map[string]interface{})
			if !ok || len(im) == 0 {
				continue
			}
			petData, ok := im["pet"].(map[string]interface{})
			if !ok || len(petData) == 0 {
				continue
			}
			curEnergy := toInt(petData["energy"])
			if curEnergy < 30 {
				// Search for Pet Feed in inventory
				for j := 0; j < len(invList); j++ {
					feedMap, ok := invList[j].(map[string]interface{})
					if !ok || len(feedMap) == 0 {
						continue
					}
					if toString(feedMap["id"]) == "II_SYS_SYS_FEED_01" {
						feedN := toInt(feedMap["n"])
						if feedN > 1 {
							feedMap["n"] = feedN - 1
						} else {
							invList[j] = map[string]interface{}{}
						}
						petData["energy"] = int(math.Min(100, float64(curEnergy+50)))
						dirty = true
						b.emitLog("action", "[PET] Auto-fed pet with Pet Feed (+50 Energy)", "info")
						break
					}
				}
			}
		}
	}

	// 6. AutoUsePowerups: Maintain active buff powerups
	if prefs.AutoUsePowerups {
		buffsMap := getOrCreateMap(charData, "buffs")

		// Upcut Stone (+20% Attack)
		if _, active := buffsMap["II_CHR_SYS_SCR_UPCUTSTONE"]; !active {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				if toString(im["id"]) == "II_CHR_SYS_SCR_UPCUTSTONE" {
					consumeOneFromSlot(invList, i)
					buffsMap["II_CHR_SYS_SCR_UPCUTSTONE"] = map[string]interface{}{
						"left":   3600.0,
						"params": [][]interface{}{{"DST_ATKPOWER_RATE", 20}},
					}
					dirty = true
					b.emitLog("action", "[BUFF] Activated Upcut Stone (+20% ATK)", "info")
					break
				}
			}
		}

		// Def-Upcut Stone (+20% Defense)
		if _, active := buffsMap["II_SYS_SYS_SCR_DEUPCUT"]; !active {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				if toString(im["id"]) == "II_SYS_SYS_SCR_DEUPCUT" {
					consumeOneFromSlot(invList, i)
					buffsMap["II_SYS_SYS_SCR_DEUPCUT"] = map[string]interface{}{
						"left":   900.0,
						"params": [][]interface{}{{"DST_ADJDEF_RATE", 20}},
					}
					dirty = true
					b.emitLog("action", "[BUFF] Activated Def-Upcut Stone (+20% DEF)", "info")
					break
				}
			}
		}

		// Grilled Eel (+50% Max HP)
		if _, active := buffsMap["II_CHR_FOO_COO_GRILLEDEEL"]; !active {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				if toString(im["id"]) == "II_CHR_FOO_COO_GRILLEDEEL" {
					consumeOneFromSlot(invList, i)
					buffsMap["II_CHR_FOO_COO_GRILLEDEEL"] = map[string]interface{}{
						"left":   3600.0,
						"params": [][]interface{}{{"DST_HP_MAX", 50}},
					}
					dirty = true
					b.emitLog("action", "[BUFF] Consumed Grilled Eel (+50% Max HP)", "info")
					break
				}
			}
		}

		// Scroll of Amplification (+50% EXP)
		ampActive := false
		for k := range buffsMap {
			if strings.HasPrefix(k, "II_SYS_SYS_SCR_AMP") {
				ampActive = true
				break
			}
		}
		if !ampActive {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				id := toString(im["id"])
				if it := models.GetItem(id); it != nil && it.K3 == "IK3_EXP_RATE" {
					consumeOneFromSlot(invList, i)
					buffsMap[id] = map[string]interface{}{
						"left":   3600.0,
						"params": [][]interface{}{},
					}
					dirty = true
					b.emitLog("action", fmt.Sprintf("[BUFF] Activated %s (+50%% EXP)", it.Name), "info")
					break
				}
			}
		}
	}

	// 7. AutoRefuelFlight: Refuel flying turbo when fuel is low
	if prefs.AutoRefuelFlight {
		accFuel := toFloat64(charData["acc_fuel"])
		b.waypointMu.RLock()
		isFly := b.waypointFlyMode
		b.waypointMu.RUnlock()

		if (isFly || accFuel < 10.0) && accFuel < 30.0 {
			for i := 0; i < len(invList); i++ {
				im, ok := invList[i].(map[string]interface{})
				if !ok || len(im) == 0 {
					continue
				}
				id := toString(im["id"])
				it := models.GetItem(id)
				if it != nil && (it.Fuel > 0 || it.K2 == "IK2_AIRFUEL") {
					addFuel := it.Fuel
					if addFuel <= 0 {
						addFuel = 12.0
					}
					consumeOneFromSlot(invList, i)
					charData["acc_fuel"] = accFuel + addFuel
					dirty = true
					b.emitLog("action", fmt.Sprintf("[FLIGHT] Refueled flying turbo (+%.0fs)", addFuel), "info")
					break
				}
			}
		}
	}

	// Persist changes and broadcast look if dirty
	if dirty {
		if err := b.persistCharData(charData, charFile, token); err == nil {
			if equipChanged {
				go b.broadcastLook()
			}
		}
	}
}

func consumeOneFromSlot(invList []interface{}, slotIdx int) {
	if slotIdx < 0 || slotIdx >= len(invList) {
		return
	}
	im, ok := invList[slotIdx].(map[string]interface{})
	if !ok || len(im) == 0 {
		return
	}
	n := toInt(im["n"])
	if n > 1 {
		im["n"] = n - 1
	} else {
		invList[slotIdx] = map[string]interface{}{}
	}
}

func toString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	}
	return fmt.Sprintf("%v", val)
}

