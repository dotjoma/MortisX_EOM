package models

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// ItemDef represents the static metadata definition of an item from items_raw.json / game.json.
type ItemDef struct {
	ID     string                   `json:"id,omitempty"`
	Name   string                   `json:"name"`
	K1     string                   `json:"k1"`
	K2     string                   `json:"k2"`
	K3     string                   `json:"k3"`
	LV     int                      `json:"lv"`
	Req    int                      `json:"req"`
	Sex    string                   `json:"sex"`
	Cost   int                      `json:"cost"`
	Stack  int                      `json:"stack"`
	Slot   string                   `json:"slot"`
	AB     []int                    `json:"ab"`
	ASPD   float64                  `json:"aspd"`
	Params [][]interface{}          `json:"params"`
	FX     [][]interface{}          `json:"fx"`
	Acc    [][][]interface{}        `json:"acc,omitempty"`
	Handed string                   `json:"handed"`
	Rare   int                      `json:"rare"`
	Job    string                   `json:"job"`
	Fuel   float64                  `json:"fuel,omitempty"`
	Time   float64                  `json:"time,omitempty"`
	CTG    int                      `json:"ctg,omitempty"`
	Ready  int                      `json:"ready,omitempty"`
	Fly    map[string]interface{}   `json:"fly,omitempty"`
	Ult    string                   `json:"ult,omitempty"`
	Set    string                   `json:"set,omitempty"`
}

// ReqLevel returns the minimum character level required to equip or use the item.
func (it *ItemDef) ReqLevel() int {
	if it == nil {
		return 0
	}
	if it.Req > 0 {
		return it.Req
	}
	if it.LV > 0 {
		return it.LV
	}
	return 1
}

// ItemDatabase caches and indexes all item definitions loaded from game.json / items_raw.json.
type ItemDatabase struct {
	items map[string]*ItemDef
	mu    sync.RWMutex
}

var (
	globalItemDBOnce sync.Once
	globalItemDB     *ItemDatabase
)

// ResolveDataFilePath attempts to locate a relative data path across execution and source directories.
func ResolveDataFilePath(relPath string) string {
	candidates := []string{
		relPath,
		filepath.Join("..", relPath),
		filepath.Join("..", "..", relPath),
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, relPath),
			filepath.Join(exeDir, "..", relPath),
			filepath.Join(exeDir, "..", "..", relPath),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return relPath
}

// GetItemDB returns the singleton ItemDatabase instance.
func GetItemDB() *ItemDatabase {
	globalItemDBOnce.Do(func() {
		globalItemDB = &ItemDatabase{
			items: make(map[string]*ItemDef, 14000),
		}
		globalItemDB.load()
	})
	return globalItemDB
}

func (db *ItemDatabase) load() {
	gamePath := ResolveDataFilePath(filepath.Join("madrigal_dump", "world", "game.json"))
	bytes, err := os.ReadFile(gamePath)
	if err == nil {
		var root struct {
			Items map[string]*ItemDef `json:"items"`
		}
		if err := json.Unmarshal(bytes, &root); err == nil && len(root.Items) > 0 {
			db.items = root.Items
			for k, v := range db.items {
				if v.ID == "" {
					v.ID = k
				}
			}
			return
		}
	}

	// Fallback: check if items_raw.json and items_idx.json can be parsed
	rawPath := ResolveDataFilePath(filepath.Join("madrigal_dump", "world", "items_raw.json"))
	idxPath := ResolveDataFilePath(filepath.Join("madrigal_dump", "world", "items_idx.json"))
	rawBytes, errRaw := os.ReadFile(rawPath)
	idxBytes, errIdx := os.ReadFile(idxPath)
	if errRaw == nil && errIdx == nil {
		var idx struct {
			IDs []string `json:"ids"`
			Off []int    `json:"off"`
		}
		if err := json.Unmarshal(idxBytes, &idx); err == nil && len(idx.IDs)*2 <= len(idx.Off) {
			for i, id := range idx.IDs {
				o1 := idx.Off[i*2]
				o2 := idx.Off[i*2+1]
				if o1 >= 0 && o2 <= len(rawBytes) && o1 < o2 {
					var it ItemDef
					if err := json.Unmarshal(rawBytes[o1:o2], &it); err == nil {
						it.ID = id
						db.items[id] = &it
					}
				}
			}
		}
	}
}

// GetItem retrieves an ItemDef by ID from the database.
func (db *ItemDatabase) GetItem(id string) *ItemDef {
	if db == nil {
		return nil
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.items[id]
}

// GetItem retrieves an item from the global item database.
func GetItem(id string) *ItemDef {
	return GetItemDB().GetItem(id)
}

// IsEquip returns true if the item has an equipment slot defined.
func IsEquip(item *ItemDef) bool {
	return item != nil && strings.TrimSpace(item.Slot) != ""
}

// IsTwoHanded returns true if the weapon occupies two hands.
func IsTwoHanded(item *ItemDef) bool {
	if item == nil {
		return false
	}
	if item.Handed == "two" {
		return true
	}
	switch item.K3 {
	case "IK3_BOW", "IK3_YOYO", "IK3_STAFF":
		return true
	}
	return false
}

// BaseJob strips master/hero suffixes from a job name.
func BaseJob(job string) string {
	j := strings.ToLower(strings.TrimSpace(job))
	j = strings.TrimSuffix(j, "_master")
	j = strings.TrimSuffix(j, "_hero")
	return j
}

// JobLine returns the lineage of eligible job names up to Vagrant.
func JobLine(job string) []string {
	base := BaseJob(job)
	switch base {
	case "blade", "knight":
		return []string{"vagrant", "mercenary", base}
	case "jester", "ranger":
		return []string{"vagrant", "acrobat", base}
	case "ringmaster", "billposter":
		return []string{"vagrant", "assist", base}
	case "psykeeper", "elementor":
		return []string{"vagrant", "magician", base}
	case "mercenary", "acrobat", "assist", "magician":
		return []string{"vagrant", base}
	default:
		return []string{"vagrant"}
	}
}

// CanEquip verifies whether a player meets all client-side pre-equip requirements for an item.
func CanEquip(playerJob, playerGender string, playerLevel int, item *ItemDef, levelDelta int, equippedWeapon *ItemDef) (bool, string) {
	if item == nil || !IsEquip(item) {
		return false, "That can't be equipped."
	}

	// 1. Job restriction
	if item.Job != "" {
		allowed := false
		for _, j := range JobLine(playerJob) {
			if strings.EqualFold(j, item.Job) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("Only a %s can use that.", item.Job)
		}
	}

	// 2. Shield specific checks
	if item.Slot == "shield" {
		if BaseJob(playerJob) == "vagrant" {
			return false, "Vagrants can't use shields."
		}
		if equippedWeapon != nil && IsTwoHanded(equippedWeapon) {
			return false, "You can't use a shield with a two-handed weapon."
		}
	}

	// 3. Gender check
	cleanGender := strings.ToLower(strings.TrimSpace(playerGender))
	if item.Sex == "m" && cleanGender != "male" {
		return false, "That is for the other gender."
	}
	if item.Sex == "f" && cleanGender != "female" {
		return false, "That is for the other gender."
	}

	// 4. Level requirement (with level down reduction offset)
	req := item.ReqLevel() + levelDelta
	if req < 0 {
		req = 0
	}
	if req > playerLevel {
		return false, fmt.Sprintf("You need level %d for that.", req)
	}

	return true, ""
}

// CalculateGearScore computes a float64 gear evaluation score based on base stats, upgrade, element, awakenings, and class weights.
func CalculateGearScore(item *ItemDef, up int, elem string, ro [][]interface{}, pc []string, playerJob string) float64 {
	if item == nil {
		return 0.0
	}

	baseJob := BaseJob(playerJob)
	up = int(math.Max(0, float64(up)))
	upMult := 1.0 + 0.08*float64(up)

	// Parse elemental upgrade level
	elemLv := 0
	if elem != "" && strings.Contains(elem, ":") {
		parts := strings.Split(elem, ":")
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				elemLv = n
			}
		}
	}

	// Extract stat bonuses map from params, awakenings (ro), and cards (pc)
	statsMap := make(map[string]float64)

	// 1. Base params from item definition
	for _, p := range item.Params {
		if len(p) >= 2 {
			statName, _ := p[0].(string)
			val := toFloat64(p[1])
			if statName != "" {
				statsMap[statName] += val
			}
		}
	}

	// 2. Accessory upgrade table (for jewelry rings, earrings, necklaces)
	if len(item.Acc) > 0 {
		accIdx := up
		if accIdx >= len(item.Acc) {
			accIdx = len(item.Acc) - 1
		}
		for _, p := range item.Acc[accIdx] {
			if len(p) >= 2 {
				statName, _ := p[0].(string)
				val := toFloat64(p[1])
				if statName != "" {
					statsMap[statName] += val
				}
			}
		}
	}

	// 3. Random options / awakenings (ro)
	for _, r := range ro {
		if len(r) >= 2 {
			statName, _ := r[0].(string)
			val := toFloat64(r[1])
			if statName != "" {
				// Special case: DST_ATTACKSPEED in awakenings is represented in tenths of a percent
				if statName == "DST_ATTACKSPEED" {
					val = val / 10.0
				}
				statsMap[statName] += val
			}
		}
	}

	// Calculate weighted stats based on job archetype
	statScore := 0.0
	isMelee := baseJob == "blade" || baseJob == "mercenary" || baseJob == "knight" || baseJob == "billposter"
	isRanged := baseJob == "acrobat" || baseJob == "ranger" || baseJob == "jester"
	isMagic := baseJob == "magician" || baseJob == "psykeeper" || baseJob == "elementor" || baseJob == "ringmaster"

	for k, v := range statsMap {
		switch k {
		case "DST_STR":
			if isMelee {
				statScore += v * 2.5
			} else {
				statScore += v * 1.0
			}
		case "DST_DEX":
			if isRanged {
				statScore += v * 2.5
			} else if baseJob == "blade" {
				statScore += v * 1.5
			} else {
				statScore += v * 0.8
			}
		case "DST_INT":
			if isMagic {
				statScore += v * 2.5
			} else {
				statScore += v * 0.5
			}
		case "DST_STA":
			statScore += v * 1.5
		case "DST_ATKPOWER_RATE":
			statScore += v * 3.0
		case "DST_ATKPOWER":
			statScore += v * 1.5
		case "DST_CRIT_PROB":
			if isRanged || baseJob == "blade" {
				statScore += v * 2.5
			} else {
				statScore += v * 1.2
			}
		case "DST_ADJDEF_RATE":
			statScore += v * 2.5
		case "DST_ADJDEF":
			statScore += v * 1.2
		case "DST_HP_MAX":
			statScore += v * 0.05
		case "DST_MP_MAX", "DST_FP_MAX":
			statScore += v * 0.02
		case "DST_SPEED":
			statScore += v * 1.0
		case "DST_ATTACKSPEED":
			statScore += v * 1.2
		default:
			if v > 0 {
				statScore += v * 0.5
			}
		}
	}

	// Slot-specific scoring
	slot := strings.ToLower(item.Slot)
	if slot == "weapon" || item.K1 == "IK1_WEAPON" {
		baseDmg := 0.0
		if len(item.AB) >= 2 {
			baseDmg = float64(item.AB[0]+item.AB[1]) / 2.0
		} else if len(item.AB) == 1 {
			baseDmg = float64(item.AB[0])
		}
		elemBonus := float64(elemLv) * 3.0
		aspdBonus := item.ASPD * 120.0
		twoHandScale := 1.0
		if IsTwoHanded(item) {
			twoHandScale = 1.15 // compensates for inability to wear shield
		}
		return (baseDmg * upMult * twoHandScale) + elemBonus + aspdBonus + statScore
	}

	if slot == "helmet" || slot == "suit" || slot == "gloves" || slot == "boots" || slot == "shield" {
		baseDef := 0.0
		if len(item.AB) >= 2 {
			baseDef = float64(item.AB[0]+item.AB[1]) / 2.0
		} else if len(item.AB) == 1 {
			baseDef = float64(item.AB[0])
		}
		elemBonus := float64(elemLv) * 2.0
		return (baseDef * upMult) + elemBonus + statScore
	}

	if slot == "ring" || slot == "earring" || slot == "necklace" ||
		slot == "ring1" || slot == "ring2" || slot == "earring1" || slot == "earring2" {
		return 10.0 + (float64(up) * 4.0) + statScore
	}

	if item.K3 == "IK3_ARROW" {
		if len(item.AB) > 0 {
			return float64(item.AB[0])
		}
		return 1.0
	}

	// Cloaks, masks, wings, fashion
	return 5.0 + statScore
}

func toFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0.0
}
