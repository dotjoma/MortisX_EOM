package models

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed combat_tables.json
var combatTablesJSON []byte

type CombatTablesData struct {
	SourceSHA256 struct {
		GameJSON       string `json:"game_json"`
		AnimEventsJSON string `json:"anim_events_json"`
	} `json:"source_sha256"`
	RowCounts struct {
		Jobs     int `json:"jobs"`
		Monsters int `json:"monsters"`
		Armors   int `json:"armors"`
	} `json:"row_counts"`
	Jobs     map[string]JobCombatMeta     `json:"jobs"`
	Monsters map[string]MonsterCombatMeta `json:"monsters"`
	Armors   map[string]ArmorCombatMeta   `json:"armors"`
}

type JobCombatMeta struct {
	HP    float64 `json:"hp"`
	Def   float64 `json:"def"`
	Block float64 `json:"block"`
}

type MonsterCombatMeta struct {
	Name        string  `json:"name"`
	Level       int     `json:"lv"`
	HR          int     `json:"hr"`
	ER          int     `json:"er"`
	DEX         int     `json:"dex"`
	AtkM        []int   `json:"atkm"`
	Atk1        []int   `json:"atk1"`
	Atk2AB      []int   `json:"atk2_ab,omitempty"`
	Rank        string  `json:"rank"`
	El          int     `json:"el"`
	Ell         int     `json:"ell"`
	Range       float32 `json:"range"`
	Model       string  `json:"model"`
	HitFracAtk1 float64 `json:"hit_frac_atk1"`
	HitFracAtk2 float64 `json:"hit_frac_atk2"`
}

type ArmorCombatMeta struct {
	Name  string `json:"name"`
	Slot  string `json:"slot"`
	AB    []int  `json:"ab"`
	Parry int    `json:"parry"`
}

var (
	combatTablesOnce sync.Once
	combatTablesData CombatTablesData
)

func initCombatTables() {
	combatTablesOnce.Do(func() {
		if len(combatTablesJSON) > 0 {
			_ = json.Unmarshal(combatTablesJSON, &combatTablesData)
		}
	})
}

// GetCombatTables returns the parsed embedded combat tables.
func GetCombatTables() *CombatTablesData {
	initCombatTables()
	return &combatTablesData
}

// GetJobCombatMeta returns combat multipliers for a job, defaulting to Vagrant values if not found.
func GetJobCombatMeta(job string) JobCombatMeta {
	initCombatTables()
	j := strings.ToLower(job)
	if meta, ok := combatTablesData.Jobs[j]; ok {
		return meta
	}
	return JobCombatMeta{HP: 0.9, Def: 1.0, Block: 0.2}
}

// GetMonsterCombatMeta returns combat metadata for a monster ID (e.g. "MI_AIBATT1").
func GetMonsterCombatMeta(mobID string) (MonsterCombatMeta, bool) {
	initCombatTables()
	meta, ok := combatTablesData.Monsters[mobID]
	return meta, ok
}

// GetArmorCombatMeta returns defense and parry attributes for an armor item ID.
func GetArmorCombatMeta(itemID string) (ArmorCombatMeta, bool) {
	initCombatTables()
	meta, ok := combatTablesData.Armors[itemID]
	return meta, ok
}
