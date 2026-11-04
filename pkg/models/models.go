package models

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type StatAllocationMode string

const (
	StatModePreset    StatAllocationMode = "preset"
	StatModeRatio     StatAllocationMode = "ratio"
	StatModeThreshold StatAllocationMode = "threshold"
	StatModeManual    StatAllocationMode = "manual"
)

type CustomStatRatio struct {
	StrPercent int    `json:"str_pct" multi:"str_pct" min:"0" max:"100"`
	StaPercent int    `json:"sta_pct" multi:"sta_pct" min:"0" max:"100"`
	DexPercent int    `json:"dex_pct" multi:"dex_pct" min:"0" max:"100"`
	IntPercent int    `json:"int_pct" multi:"int_pct" min:"0" max:"100"`
	CapStat    string `json:"cap_stat" multi:"cap_stat" enum:"str,sta,dex,int"`
	CapValue   int    `json:"cap_value" multi:"cap_value" min:"15" max:"300"`
	ThenStat   string `json:"then_stat" multi:"then_stat" enum:"str,sta,dex,int"`
}

type JobProgressionConfig struct {
	AutoJobChange    bool               `json:"auto_job_change" multi:"auto_job_change"`
	Target1stJob     string             `json:"target_1st_job" multi:"target_1st_job" enum:"mercenary,acrobat,assist,magician"`
	Target2ndJob     string             `json:"target_2nd_job" multi:"target_2nd_job" enum:"blade,knight,jester,ranger,ringmaster,billposter,psykeeper,elementor"`
	AutoRebirth      bool               `json:"auto_rebirth" multi:"auto_rebirth"`
	StatMode         StatAllocationMode `json:"stat_mode" multi:"stat_mode" enum:"preset,ratio,threshold,manual"`
	SelectedPreset   string             `json:"selected_preset" multi:"selected_preset" enum:"pure_int,pure_str,pure_dex,pure_sta,dex_cap_43,hybrid_str_dex,hybrid_int_sta,hybrid_sta_str,hybrid_dex_sta"`
	CustomRatio      CustomStatRatio    `json:"custom_ratio" multi:"custom_ratio"`
	AutoEquipStarter bool               `json:"auto_equip_starter" multi:"auto_equip_starter"`
}

func DefaultJobProgressionConfig() JobProgressionConfig {
	return JobProgressionConfig{
		AutoJobChange:    true,
		Target1stJob:     "assist",
		Target2ndJob:     "ringmaster",
		AutoRebirth:      false,
		StatMode:         StatModePreset,
		SelectedPreset:   "pure_int",
		CustomRatio: CustomStatRatio{
			StrPercent: 0,
			StaPercent: 0,
			DexPercent: 0,
			IntPercent: 100,
			CapStat:    "dex",
			CapValue:   43,
			ThenStat:   "str",
		},
		AutoEquipStarter: true,
	}
}

type BotPreferences struct {
	BypassLogon        bool                 `json:"bypass_logon" multi:"bypass_logon"`
	AutoReconnect      bool                 `json:"auto_reconnect" multi:"auto_reconnect"`
	AutoReconnectDelay int                  `json:"auto_reconnect_delay,omitempty" multi:"auto_reconnect_delay" min:"1" max:"3600"`
	AutoAcceptParty    bool                 `json:"auto_accept_party" multi:"auto_accept_party"`
	AutoAcceptTrade    bool                 `json:"auto_accept_trade" multi:"auto_accept_trade"`
	AntiAFK            bool                 `json:"anti_afk" multi:"anti_afk"`
	AutoDisconnectOnGM  bool                 `json:"auto_disconnect_on_gm" multi:"auto_disconnect_on_gm"`
	Invisible           bool                 `json:"invisible" multi:"invisible"`
	LegitMode          bool                 `json:"legit_mode" multi:"legit_mode"`
	AutoClaimDailyGifts bool                 `json:"auto_claim_daily_gifts" multi:"auto_claim_daily_gifts"`
	DisableAutoRest    bool                 `json:"disable_auto_rest" multi:"disable_auto_rest"`
	MaxPenyaCap         int                  `json:"max_penya_cap,omitempty" multi:"max_penya_cap"`
	MoveStep            float32              `json:"move_step,omitempty" multi:"move_step" min:"0.2" max:"20"`
	JobConfig          JobProgressionConfig `json:"job_config" multi:"job_config"`

	// Smart Gear & Inventory Automation
	AutoEquipUpgrades bool `json:"autoEquipUpgrades" multi:"autoEquipUpgrades"`
	LockWeaponType    bool `json:"lockWeaponType" multi:"lockWeaponType"`
	AutoEquipArrows   bool `json:"autoEquipArrows" multi:"autoEquipArrows"`
	AutoJunkInferior  bool `json:"autoJunkInferior" multi:"autoJunkInferior"`
	AutoLockRareGear  bool `json:"autoLockRareGear" multi:"autoLockRareGear"`
	AutoManagePet     bool `json:"autoManagePet" multi:"autoManagePet"`
	AutoUsePowerups   bool `json:"autoUsePowerups" multi:"autoUsePowerups"`
	AutoRefuelFlight  bool `json:"autoRefuelFlight" multi:"autoRefuelFlight"`
}

// UnmarshalJSON implements custom JSON unmarshaling supporting both camelCase and snake_case field names.
func (bp *BotPreferences) UnmarshalJSON(data []byte) error {
	type Alias BotPreferences
	aux := &struct {
		*Alias
		SnakeAutoEquipUpgrades *bool `json:"auto_equip_upgrades"`
		SnakeLockWeaponType    *bool `json:"lock_weapon_type"`
		SnakeAutoEquipArrows   *bool `json:"auto_equip_arrows"`
		SnakeAutoJunkInferior  *bool `json:"auto_junk_inferior"`
		SnakeAutoLockRareGear  *bool `json:"auto_lock_rare_gear"`
		SnakeAutoManagePet     *bool `json:"auto_manage_pet"`
		SnakeAutoUsePowerups   *bool `json:"auto_use_powerups"`
		SnakeAutoRefuelFlight  *bool `json:"auto_refuel_flight"`
		CamelDisableAutoRest   *bool `json:"disableAutoRest"`
	}{
		Alias: (*Alias)(bp),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.CamelDisableAutoRest != nil {
		bp.DisableAutoRest = *aux.CamelDisableAutoRest
	}

	if aux.SnakeAutoEquipUpgrades != nil {
		bp.AutoEquipUpgrades = *aux.SnakeAutoEquipUpgrades
	}
	if aux.SnakeLockWeaponType != nil {
		bp.LockWeaponType = *aux.SnakeLockWeaponType
	}
	if aux.SnakeAutoEquipArrows != nil {
		bp.AutoEquipArrows = *aux.SnakeAutoEquipArrows
	}
	if aux.SnakeAutoJunkInferior != nil {
		bp.AutoJunkInferior = *aux.SnakeAutoJunkInferior
	}
	if aux.SnakeAutoLockRareGear != nil {
		bp.AutoLockRareGear = *aux.SnakeAutoLockRareGear
	}
	if aux.SnakeAutoManagePet != nil {
		bp.AutoManagePet = *aux.SnakeAutoManagePet
	}
	if aux.SnakeAutoUsePowerups != nil {
		bp.AutoUsePowerups = *aux.SnakeAutoUsePowerups
	}
	if aux.SnakeAutoRefuelFlight != nil {
		bp.AutoRefuelFlight = *aux.SnakeAutoRefuelFlight
	}

	return nil
}

type NearbyPlayerInfo struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Z    float32 `json:"z"`
}

const (
	SweepModeHub      = "hub"      // Official towns, lodestars, named travel anchors only
	SweepModeCorridor = "corridor" // Hubs plus high-altitude midpoints between them
	SweepModeThorough = "thorough" // Hubs, then hotspot fill, then world grids
)

type ScanConfig struct {
	RGuaranteedM      float32 `json:"r_guaranteed_m"`      // Minimum verified AOI reach (default: 100m)
	SpacingThoroughM  float32 `json:"spacing_thorough_m"`  // Square grid spacing R/sqrt(2) (default: 140m)
	SpacingFastM      float32 `json:"spacing_fast_m"`      // Fast expansion grid spacing (default: 200m)
	DwellTimeoutMs    int     `json:"dwell_timeout_ms"`    // Max dwell time per probe (default: 800ms)
	EarlyExit         bool    `json:"early_exit"`          // Early-exit on fresh target sighting (default: true)
	AllowWaterNodes   bool    `json:"allow_water_nodes"`   // Allow water node generation (default: false)
	SweepMode         string  `json:"sweep_mode"`          // hub | corridor | thorough
	MinWarpIntervalMs int     `json:"min_warp_interval_ms"`
	BurstSize         int     `json:"burst_size"`
	BurstRestMs       int     `json:"burst_rest_ms"`
	FlightClearanceM  float32 `json:"flight_clearance_m"`
}

func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		RGuaranteedM:      100.0,
		SpacingThoroughM:  140.0,
		SpacingFastM:      200.0,
		DwellTimeoutMs:    800,
		EarlyExit:         true,
		AllowWaterNodes:   false,
		SweepMode:         SweepModeHub,
		MinWarpIntervalMs: 280,
		BurstSize:         4,
		BurstRestMs:       1200,
		FlightClearanceM:  32.0,
	}
}

type MapProfile struct {
	Name     string  `json:"name"`
	MinTileX int     `json:"min_tile_x"`
	MaxTileX int     `json:"max_tile_x"`
	MinTileZ int     `json:"min_tile_z"`
	MaxTileZ int     `json:"max_tile_z"`
	TileSize float32 `json:"tile_size"`
}

func DefaultMainMadrigalProfile() MapProfile {
	return MapProfile{
		Name:     "madrigal",
		MinTileX: 3,
		MaxTileX: 19,
		MinTileZ: 3,
		MaxTileZ: 9,
		TileSize: 512.0,
	}
}

type SweepNode struct {
	X    float32 `json:"x"`
	Z    float32 `json:"z"`
	Tier int     `json:"tier"` // 0 last-known, 1 hub, 2 corridor/fast, 3 thorough
	Name string  `json:"name,omitempty"`
	Fly  bool    `json:"fly,omitempty"`
}

func DefaultPreferences() BotPreferences {
	return BotPreferences{
		BypassLogon:        false,
		AutoReconnect:      true,
		AutoReconnectDelay: 14,
		AutoAcceptParty:    false,
		AntiAFK:            true,
		AutoDisconnectOnGM:  true,
		Invisible:           false,
		LegitMode:          false,
		AutoClaimDailyGifts: true,
		MaxPenyaCap:         299000,
		MoveStep:            1.2,
		JobConfig:          DefaultJobProgressionConfig(),
	}
}

type BotIntervals struct {
	MoveSec   float64 `json:"move_sec"`
	AttackSec float64 `json:"attack_sec"`
	LootSec   float64 `json:"loot_sec"`
	SkillSec  float64 `json:"skill_sec"`
}

func DefaultIntervals() BotIntervals {
	return BotIntervals{
		MoveSec:   0.235,
		AttackSec: 0.150,
		LootSec:   0.100,
		SkillSec:  0.200,
	}
}

type ClassBuildConfig struct {
	Job         string `json:"job"`
	Mode        string `json:"mode"`      // "cap", "pure", "ratio", "manual"
	CapStat     string `json:"cap_stat"`  // "dex", "sta", "str", "int"
	CapValue    int    `json:"cap_value"` // e.g. 43
	ThenStat    string `json:"then_stat"` // e.g. "str"
	AutoStat    int    `json:"auto_stat"` // 0=None, 1=STR, 2=STA, 3=DEX, 4=INT
	Description string `json:"description"`
}

type StatsJsonMap map[string]ClassBuildConfig

type WaypointCoord struct {
	X float32 `json:"x"`
	Z float32 `json:"z"`
}

type BotRuntimeStats struct {
	PingMs        uint32         `json:"ping_ms"`
	PlayedSeconds uint64         `json:"played_seconds"`
	Deaths        uint32         `json:"deaths"`
	Zone          string         `json:"zone"`
	Level         uint32         `json:"level"`
	Exp           uint64         `json:"exp"`
	MaxExp        uint64         `json:"max_exp"`
	Gold          uint64         `json:"gold"`
	PosX          float32        `json:"pos_x"`
	PosY          float32        `json:"pos_y"`
	PosZ          float32        `json:"pos_z"`
	Job           string         `json:"job"`
	HP            uint32         `json:"hp"`
	MP            uint32         `json:"mp"`
	FP            uint32         `json:"fp"`
	STR           uint32         `json:"str"`
	STA           uint32         `json:"sta"`
	DEX           uint32         `json:"dex"`
	INT           uint32         `json:"int"`
	StatPoints    uint32         `json:"stat_points"`
	AutoStat      uint32         `json:"auto_stat"`
	Waypoint      *WaypointCoord `json:"waypoint,omitempty"`
}

func DefaultStats() BotRuntimeStats {
	return BotRuntimeStats{
		PingMs:        0,
		PlayedSeconds: 0,
		Deaths:        0,
		Zone:          "Flarine",
		Level:         1,
		Exp:           0,
		MaxExp:        14,
		Gold:          0,
		PosX:          0,
		PosY:          0,
		PosZ:          0,
		Job:           "Vagrant",
		STR:           15,
		STA:           15,
		DEX:           15,
		INT:           15,
		StatPoints:    0,
		AutoStat:      0,
	}
}

type BotProfile struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Server        string         `json:"server"`
	CharacterName string         `json:"character_name"`
	Preferences   BotPreferences `json:"preferences"`
	Intervals     BotIntervals   `json:"intervals"`
	Password      string         `json:"password,omitempty"`
}

type BotSummary struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Server        string          `json:"server"`
	CharacterName string          `json:"character_name"`
	Status        string          `json:"status"` // "disconnected", "connecting", "online", "paused", "error", "server_issue", "duplicate_login", "account_banned", "channel_full"
	StatusMessage string          `json:"status_message,omitempty"`
	Stats         BotRuntimeStats `json:"stats"`
	Preferences   BotPreferences  `json:"preferences"`
	Intervals     BotIntervals    `json:"intervals"`
	Password      string          `json:"password,omitempty"`
}

type ChannelInfo struct {
	ID     uint32 `json:"id"`
	Name   string `json:"name"`
	Online uint32 `json:"online"`
	Max    uint32 `json:"max"`
	PK     bool   `json:"pk"`
	Market bool   `json:"market"`
}

type ServerHelloResponse struct {
	Channels []ChannelInfo `json:"channels"`
}

type AuthRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type AccountData struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Slots []string `json:"slots"`
	Auth  uint32   `json:"auth"`
	DP    uint32   `json:"dp"`
	VP    uint32   `json:"vp"`
	VIP   uint32   `json:"vip"`
	Pin   uint32   `json:"pin"`
}

type AuthResponse struct {
	Account AccountData       `json:"account"`
	Token   string            `json:"token"`
	Files   map[string]string `json:"files"`
}

type CharacterFileStats struct {
	Name     string    `json:"name"`
	Level    uint32    `json:"level"`
	Exp      uint64    `json:"exp"`
	Penya    uint64    `json:"penya"`
	Job      string    `json:"job"`
	HP       uint32    `json:"hp"`
	MP       uint32    `json:"mp"`
	FP       uint32    `json:"fp"`
	STR      uint32    `json:"str"`
	STA      uint32    `json:"sta"`
	DEX      uint32    `json:"dex"`
	INT      uint32    `json:"int"`
	Points   uint32    `json:"points"`
	AutoStat uint32    `json:"auto_stat"`
	Pos      []float32 `json:"pos"`
	Map      string    `json:"map"`
}

func FormatJobName(job string) string {
	trimmed := strings.TrimSpace(job)
	if trimmed == "" {
		return "Vagrant"
	}
	return strings.ToUpper(trimmed[:1]) + strings.ToLower(trimmed[1:])
}

func DetermineZone(mapName string, x, z float32) string {
	lm := strings.ToLower(strings.TrimSpace(mapName))
	if lm != "" && lm != "madrigal" && lm != "flarine" {
		return strings.ToUpper(lm[:1]) + lm[1:]
	}

	if x > 0 || z != 0 {
		if x >= 7800.0 {
			return "Saint Morning"
		} else if x >= 6300.0 && z >= -3400.0 {
			return "Northern Flaris"
		} else if x >= 6300.0 {
			return "Flarine"
		} else if x >= 4900.0 {
			return "Darkon 1 & 2"
		} else if x > 1000.0 {
			return "Darkon 3"
		}
	}
	return "Flarine"
}

func ParseCharacterStats(files map[string]string, charName string) *CharacterFileStats {
	lowerChar := strings.ToLower(strings.TrimSpace(charName))
	var candidates []*CharacterFileStats

	for fn, content := range files {
		lfn := strings.ToLower(fn)
		if strings.HasPrefix(lfn, "chars/") &&
			!strings.HasSuffix(lfn, "_ach.json") &&
			!strings.HasSuffix(lfn, "_records.json") {
			var stats CharacterFileStats
			if err := json.Unmarshal([]byte(content), &stats); err == nil {
				if stats.Level == 0 {
					stats.Level = 1
				}
				if stats.Name != "" {
					if lowerChar != "" && (strings.EqualFold(stats.Name, lowerChar) || strings.Contains(strings.ToLower(stats.Name), lowerChar)) {
						return &stats
					}
					candidates = append(candidates, &stats)
				}
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}
	return nil
}

func FindCharacterFileName(files map[string]string, charName string, defaultAccKey string) string {
	lowerChar := strings.ToLower(strings.TrimSpace(charName))
	for fn, content := range files {
		lfn := strings.ToLower(fn)
		if strings.HasPrefix(lfn, "chars/") &&
			!strings.HasSuffix(lfn, "_ach.json") &&
			!strings.HasSuffix(lfn, "_records.json") {
			var stats CharacterFileStats
			if err := json.Unmarshal([]byte(content), &stats); err == nil {
				if stats.Name != "" && lowerChar != "" && (strings.EqualFold(stats.Name, lowerChar) || strings.Contains(strings.ToLower(stats.Name), lowerChar)) {
					return fn
				}
			}
		}
	}
	return fmt.Sprintf("chars/%s_0.json", defaultAccKey)
}

func CreateDefaultCharacterSave(name string, gender string) string {
	g := "male"
	if strings.EqualFold(gender, "female") {
		g = "female"
	}
	s := "M"
	if g == "female" {
		s = "F"
	}
	invSlots := make([]interface{}, 240)
	for i := range invSlots {
		invSlots[i] = map[string]interface{}{}
	}
	hotbarSlots := make([]string, 104)
	hotbarSlots[0] = "II_GEN_FOO_INS_LOLLIPOP"
	hotbarSlots[1] = "II_GEN_FOO_ICE_ORANGEJUIICE"
	pockets := [][]interface{}{
		make([]interface{}, 6),
		make([]interface{}, 24),
		make([]interface{}, 24),
	}
	for i := range pockets {
		for j := range pockets[i] {
			pockets[i][j] = map[string]interface{}{}
		}
	}

	saveData := map[string]interface{}{
		"name":         name,
		"gender":       g,
		"face":         1,
		"hair":         1,
		"hair_color":   "5a381f",
		"level":        1,
		"exp":          0,
		"points":       0,
		"auto_stat":    0,
		"str":          15,
		"sta":          15,
		"dex":          15,
		"int":          15,
		"hp":           150,
		"mp":           100,
		"fp":           100,
		"penya":        200,
		"acc_fuel":     0.0,
		"inv":          invSlots,
		"equip": map[string]string{
			"weapon": "II_WEA_SWO_WOODEN",
			"suit":   fmt.Sprintf("II_ARM_%s_VAG_SUIT01", s),
			"gloves": fmt.Sprintf("II_ARM_%s_VAG_GAUNTLET01", s),
			"boots":  fmt.Sprintf("II_ARM_%s_VAG_BOOTS01", s),
		},
		"equip_up":     map[string]int{},
		"equip_el":     map[string]int{},
		"equip_pc":     map[string]int{},
		"equip_ro":     map[string]int{},
		"equip_x":      map[string]int{},
		"hotbar":       hotbarSlots,
		"job":          "vagrant",
		"skill_points": 0,
		"top_lv":       1,
		"skills":       map[string]int{},
		"buffs":        map[string]interface{}{},
		"pockets":      pockets,
		"pocket_until": []float64{0, 0, 0},
		"quests":       map[string]interface{}{},
		"quests_done":  []string{},
		"return_pos":   []interface{}{},
		"pos":          []float32{6950.0, 100.0, -3200.0, 3.14159},
		"dungeon_cd":   map[string]interface{}{},
		"album":        map[string]interface{}{},
		"guide":        []interface{}{},
		"pet_out":      0,
		"looter_out":   0,
		"catch_out":    0,
		"loot_filter":  map[string]interface{}{},
		"auto_loot":    false,
		"hide":         map[string]interface{}{},
	}
	b, _ := json.Marshal(saveData)
	return string(b)
}

func ResolveNamedPlace(name string) (float32, float32, bool) {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, " ", "")
	switch clean {
	case "flarine", "flaris", "spawn", "town":
		return 6950.0, -3200.0, true
	case "saintmorning", "saint", "sm":
		return 8445.0, -3609.0, true
	case "darken", "darkon", "darkon2":
		return 3836.0, -4450.0, true
	case "darkon1":
		return 5250.0, -4000.0, true
	case "darkon3":
		return 3564.0, -3293.0, true
	case "rhisis", "garden", "gardenofrhisis":
		return 9178.0, -4122.0, true
	case "pumpkin":
		return 8205.0, -2740.0, true
	case "desert":
		return 8150.0, -1967.0, true
	case "madren":
		return 7346.0, -3657.0, true
	case "eillun":
		return 7244.0, -9135.0, true
	case "volcane":
		return 3341.0, -1882.0, true
	case "shaduwar":
		return 5950.0, -5400.0, true
	case "valley", "risen":
		return 7066.0, -6569.0, true
	case "bahara":
		return 6054.0, -9427.0, true
	case "collect", "collecting":
		return 6942.0, -3104.0, true
	default:
		return 0, 0, false
	}
}

type ScanWaypoint struct {
	Name string  `json:"name"`
	X    float32 `json:"x"`
	Z    float32 `json:"z"`
}

var DefaultScanWaypoints = []ScanWaypoint{
	{"Flarine Center", 6970.0, -3337.0},
	{"Flarine East / Bridge", 7150.0, -3250.0},
	{"Flarine North / Bank", 6950.0, -3200.0},
	{"Saint Morning Town", 8396.0, -3673.0},
	{"Saint Morning Pumpkin", 8205.0, -2740.0},
	{"Darkon 2 (Town)", 3818.0, -4425.0},
	{"Darkon 1 (Industrial)", 5573.0, -3905.0},
	{"Darkon 3 (Erons)", 3860.0, -3243.0},
	{"Garden of Rhisis", 9271.0, -3703.0},
	{"Collecting Area", 6942.0, -3104.0},
	{"Madren Town", 7346.5, -3657.0},
	{"Eillun", 7244.0, -9135.0},
	{"Valley of the Risen", 7066.0, -6569.0},
	{"Volcane", 3341.0, -1882.0},
	{"Shaduwar", 5950.0, -5400.0},
	{"Bahara Desert", 6054.0, -9427.0},
}

var StarterClassWeapons = map[string]string{
	"mercenary": "II_WEA_SWO_PETAL",
	"acrobat":   "II_WEA_YOY_ITUN",
	"assist":    "II_WEA_CHEE_PLIM",
	"magician":  "II_WEA_WAN_SEAL",
}

var SecondJobsMap = map[string][]string{
	"mercenary": {"blade", "knight"},
	"acrobat":   {"jester", "ranger"},
	"assist":    {"ringmaster", "billposter"},
	"magician":  {"psykeeper", "elementor"},
}

func IsFirstJob(job string) bool {
	j := strings.ToLower(strings.TrimSpace(job))
	return j == "mercenary" || j == "acrobat" || j == "assist" || j == "magician"
}

func IsSecondJob(job string) bool {
	j := strings.ToLower(strings.TrimSpace(job))
	return j == "blade" || j == "knight" || j == "jester" || j == "ranger" ||
		j == "ringmaster" || j == "billposter" || j == "psykeeper" || j == "elementor"
}

func ValidateJobProgressionConfig(cfg JobProgressionConfig) error {
	// 1. Lineage consistency
	if cfg.Target1stJob != "" && cfg.Target2ndJob != "" {
		allowed, ok := SecondJobsMap[strings.ToLower(strings.TrimSpace(cfg.Target1stJob))]
		if !ok {
			return fmt.Errorf("unknown 1st job %q", cfg.Target1stJob)
		}
		found := false
		for _, j := range allowed {
			if strings.EqualFold(j, cfg.Target2ndJob) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("target 2nd job %q is not a valid progression of 1st job %q", cfg.Target2ndJob, cfg.Target1stJob)
		}
	}

	// 2. Ratio sum rule
	if cfg.StatMode == StatModeRatio {
		total := cfg.CustomRatio.StrPercent + cfg.CustomRatio.StaPercent + cfg.CustomRatio.DexPercent + cfg.CustomRatio.IntPercent
		if total != 100 {
			return fmt.Errorf("custom stat ratios must sum to 100%%, got %d%%", total)
		}
	}

	// 3. Threshold cap rule
	if cfg.StatMode == StatModeThreshold {
		if cfg.CustomRatio.CapStat != "" && strings.EqualFold(cfg.CustomRatio.CapStat, cfg.CustomRatio.ThenStat) {
			return fmt.Errorf("cap stat and then stat cannot be the same (%s)", cfg.CustomRatio.CapStat)
		}
		if cfg.CustomRatio.CapValue < 15 {
			return fmt.Errorf("cap value must be at least 15, got %d", cfg.CustomRatio.CapValue)
		}
	}

	return nil
}

func DetermineAutoStat(cfg JobProgressionConfig) int {
	if cfg.StatMode == StatModePreset {
		switch strings.ToLower(cfg.SelectedPreset) {
		case "pure_str":
			return 1 // STR
		case "pure_sta":
			return 2 // STA
		case "pure_dex":
			return 3 // DEX
		case "pure_int":
			return 4 // INT
		default:
			return 0
		}
	} else if cfg.StatMode == StatModeRatio {
		if cfg.CustomRatio.StrPercent == 100 {
			return 1
		} else if cfg.CustomRatio.StaPercent == 100 {
			return 2
		} else if cfg.CustomRatio.DexPercent == 100 {
			return 3
		} else if cfg.CustomRatio.IntPercent == 100 {
			return 4
		}
	}
	return 0
}

func CalculateStatDistribution(
	mode StatAllocationMode,
	preset string,
	custom CustomStatRatio,
	curStr, curSta, curDex, curInt, freePoints int,
) (addStr, addSta, addDex, addInt int) {
	if freePoints <= 0 {
		return 0, 0, 0, 0
	}

	switch mode {
	case StatModePreset:
		switch strings.ToLower(preset) {
		case "pure_str":
			return freePoints, 0, 0, 0
		case "pure_sta":
			return 0, freePoints, 0, 0
		case "pure_dex":
			return 0, 0, freePoints, 0
		case "pure_int":
			return 0, 0, 0, freePoints
		case "dex_cap_43", "blade_43_dex":
			needDex := 43 - curDex
			if needDex < 0 {
				needDex = 0
			}
			if freePoints <= needDex {
				return 0, 0, freePoints, 0
			}
			return freePoints - needDex, 0, needDex, 0
		case "hybrid_str_dex": // 75% STR, 25% DEX
			dex := int(math.Round(float64(freePoints) * 0.25))
			return freePoints - dex, 0, dex, 0
		case "hybrid_dex_sta": // 80% DEX, 20% STA
			sta := int(math.Round(float64(freePoints) * 0.20))
			return 0, sta, freePoints - sta, 0
		case "hybrid_int_sta": // 70% INT, 30% STA
			sta := int(math.Round(float64(freePoints) * 0.30))
			return 0, sta, 0, freePoints - sta
		case "hybrid_sta_str": // 70% STA, 30% STR
			str := int(math.Round(float64(freePoints) * 0.30))
			return str, freePoints - str, 0, 0
		default:
			return freePoints, 0, 0, 0
		}

	case StatModeThreshold:
		capStat := strings.ToLower(strings.TrimSpace(custom.CapStat))
		thenStat := strings.ToLower(strings.TrimSpace(custom.ThenStat))
		if thenStat == "" {
			thenStat = "str"
		}
		capVal := custom.CapValue
		if capVal <= 0 {
			capVal = 43
		}

		curVal := 0
		switch capStat {
		case "str":
			curVal = curStr
		case "sta":
			curVal = curSta
		case "dex":
			curVal = curDex
		case "int":
			curVal = curInt
		default:
			curVal = curDex
			capStat = "dex"
		}

		needed := capVal - curVal
		if needed < 0 {
			needed = 0
		}

		addCap := 0
		if freePoints <= needed {
			addCap = freePoints
		} else {
			addCap = needed
		}
		rem := freePoints - addCap

		switch capStat {
		case "str":
			addStr = addCap
		case "sta":
			addSta = addCap
		case "dex":
			addDex = addCap
		case "int":
			addInt = addCap
		}

		switch thenStat {
		case "str":
			addStr += rem
		case "sta":
			addSta += rem
		case "dex":
			addDex += rem
		case "int":
			addInt += rem
		default:
			addStr += rem
		}
		return addStr, addSta, addDex, addInt

	case StatModeRatio:
		total := custom.StrPercent + custom.StaPercent + custom.DexPercent + custom.IntPercent
		if total <= 0 {
			return freePoints, 0, 0, 0
		}
		pStr := int(math.Floor(float64(freePoints) * float64(custom.StrPercent) / float64(total)))
		pSta := int(math.Floor(float64(freePoints) * float64(custom.StaPercent) / float64(total)))
		pDex := int(math.Floor(float64(freePoints) * float64(custom.DexPercent) / float64(total)))
		pInt := int(math.Floor(float64(freePoints) * float64(custom.IntPercent) / float64(total)))

		rem := freePoints - (pStr + pSta + pDex + pInt)
		if rem > 0 {
			maxPct := custom.StrPercent
			best := &pStr
			if custom.StaPercent > maxPct {
				maxPct = custom.StaPercent
				best = &pSta
			}
			if custom.DexPercent > maxPct {
				maxPct = custom.DexPercent
				best = &pDex
			}
			if custom.IntPercent > maxPct {
				best = &pInt
			}
			*best += rem
		}
		return pStr, pSta, pDex, pInt

	case StatModeManual:
		return 0, 0, 0, 0
	}

	return freePoints, 0, 0, 0
}
