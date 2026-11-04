package models

// FarmConfig defines user preferences for the Farm automation engine
type FarmConfig struct {
	Enabled               bool     `json:"enabled" multi:"enabled"`
	Mode                  string   `json:"mode" multi:"mode" enum:"hunt,support"` // "hunt" or "support"
	AnchorX               float32  `json:"anchor_x" multi:"anchor_x"`
	AnchorY               float32  `json:"anchor_y" multi:"anchor_y"`
	AnchorZ               float32  `json:"anchor_z" multi:"anchor_z"`
	Radius                float32  `json:"radius" multi:"radius" min:"10" max:"100"` // default 35m
	FollowTarget          string   `json:"follow_target" multi:"follow_target"` // support mode ally name
	FollowDist            float32  `json:"follow_dist" multi:"follow_dist" min:"1" max:"50"` // default 4m
	HealThreshold         float32  `json:"heal_threshold" multi:"heal_threshold" min:"1" max:"100"` // default 60%
	CircleHeal            bool     `json:"circle_heal" multi:"circle_heal"`
	AutoResurrection      bool     `json:"auto_resurrection" multi:"auto_resurrection"`
	Skills                []string `json:"skills" multi:"-"` // combo skills (e.g. SI_...)
	Buffs                 []string `json:"buffs" multi:"-"` // self buffs
	SupportBuffs          []string `json:"support_buffs" multi:"-"` // party buffs
	AutoAttack            bool     `json:"auto_attack" multi:"auto_attack"`
	AutoLucky             bool     `json:"auto_lucky" multi:"auto_lucky"` // sends {"t": "luck"}
	AutoAmp               bool     `json:"auto_amp" multi:"auto_amp"` // stacks amp scroll up to 5x
	AutoUpcut             bool     `json:"auto_upcut" multi:"auto_upcut"`
	AutoDefUpcut          bool     `json:"auto_def_upcut" multi:"auto_def_upcut"`
	AutoGrilledEel        bool     `json:"auto_grilled_eel" multi:"auto_grilled_eel"`
	AutoPotions           bool     `json:"auto_potions" multi:"auto_potions"`
	HpThreshold           float32  `json:"hp_threshold" multi:"hp_threshold" min:"1" max:"100"` // default 60%
	MpThreshold           float32  `json:"mp_threshold" multi:"mp_threshold" min:"1" max:"100"` // default 30%
	FpThreshold           float32  `json:"fp_threshold" multi:"fp_threshold" min:"1" max:"100"` // default 30%
	AutoRest              bool     `json:"auto_rest" multi:"auto_rest"`
	RestHpThreshold       float32  `json:"rest_hp_threshold" multi:"rest_hp_threshold" min:"1" max:"100"` // default 35%
	AutoLoot              bool     `json:"auto_loot" multi:"auto_loot"`
	LootRarityMin         int      `json:"loot_rarity_min" multi:"loot_rarity_min" min:"0" max:"3"` // 0=All, 1=Uncommon+, 2=Rare+, 3=Unique
	LootPenyaOnly         bool     `json:"loot_penya_only" multi:"loot_penya_only"`
	LootQuestItems        bool     `json:"loot_quest_items" multi:"loot_quest_items"`
	StopWhenBagFull       bool     `json:"stop_when_bag_full" multi:"stop_when_bag_full"`

	// Smart Mode (Optimal EXP & Player Avoidance)
	SmartMode             bool     `json:"smart_mode" multi:"smart_mode"` // true
	SmartMinLevelOffset   int      `json:"smart_min_level_offset" multi:"smart_min_level_offset"` // default 14
	SmartMaxLevelOffset   int      `json:"smart_max_level_offset" multi:"smart_max_level_offset"` // default 14
	AvoidKillSteal        bool     `json:"avoid_kill_steal" multi:"avoid_kill_steal"` // true (skip mobs attacked/damaged by others)
	EvacuateOnPlayerNearby bool    `json:"evacuate_on_player_nearby" multi:"evacuate_on_player_nearby"` // true
	PlayerDetectionRadius float32  `json:"player_detection_radius" multi:"player_detection_radius" min:"30" max:"120"` // default 60m

	// Stealth & Anti-Ban
	GhostModeBlockRecPut  bool     `json:"ghost_mode_block_rec_put" multi:"ghost_mode_block_rec_put"` // true (drops rec_put packet)

	// Exploits (Unsafe / Ban Risk)
	ExploitOneKill        bool     `json:"exploit_one_kill" multi:"exploit_one_kill" risk:"high"`
	ExploitMultiSweep     bool     `json:"exploit_multi_sweep" multi:"exploit_multi_sweep" risk:"high"`
	ExploitGodMode        bool     `json:"exploit_god_mode" multi:"exploit_god_mode" risk:"high"`
	ExploitCapHp1         bool     `json:"exploit_cap_hp1" multi:"exploit_cap_hp1" risk:"high"`

	// Multi-Bot Coordination
	CoordinatedFarm       bool     `json:"coordinated_farm" multi:"coordinated_farm"`
	WatchdogSec           int      `json:"watchdog_sec" multi:"watchdog_sec"` // seconds without HP progress before rebalance (default 90)

	// Webhook Alerts
	DiscordWebhookURL        string `json:"discord_webhook_url" multi:"discord_webhook_url" sensitive:"true"`
	WebhookAlertOnPlayer     bool   `json:"webhook_alert_on_player" multi:"webhook_alert_on_player"`
	WebhookAlertOnDeath      bool   `json:"webhook_alert_on_death" multi:"webhook_alert_on_death"`
	WebhookAlertOnRareDrop   bool   `json:"webhook_alert_on_rare_drop" multi:"webhook_alert_on_rare_drop"`
	WebhookAlertOnBagFull    bool   `json:"webhook_alert_on_bag_full" multi:"webhook_alert_on_bag_full"`
	WebhookAlertOnDisconnect bool   `json:"webhook_alert_on_disconnect" multi:"webhook_alert_on_disconnect"`
}

func DefaultFarmConfig() FarmConfig {
	return FarmConfig{
		Enabled:               false,
		Mode:                  "hunt",
		Radius:                35.0,
		FollowDist:            4.0,
		HealThreshold:         60.0,
		CircleHeal:            true,
		AutoResurrection:      true,
		AutoAttack:            true,
		AutoLucky:             true,
		AutoAmp:               false,
		AutoUpcut:             false,
		AutoDefUpcut:          false,
		AutoGrilledEel:        false,
		AutoPotions:           true,
		HpThreshold:           60.0,
		MpThreshold:           30.0,
		FpThreshold:           30.0,
		AutoRest:              true,
		RestHpThreshold:       35.0,
		AutoLoot:              true,
		LootRarityMin:         0,
		LootPenyaOnly:         false,
		LootQuestItems:        true,
		StopWhenBagFull:       true,
		SmartMode:             true,
		SmartMinLevelOffset:   14,
		SmartMaxLevelOffset:   14,
		AvoidKillSteal:        false,
		EvacuateOnPlayerNearby: true,
		PlayerDetectionRadius: 60.0,
		GhostModeBlockRecPut:  true,
		ExploitOneKill:        false,
		ExploitMultiSweep:     false,
		ExploitGodMode:        false,
		ExploitCapHp1:         false,
		CoordinatedFarm:       true,
		WatchdogSec:           90,
		WebhookAlertOnPlayer:  true,
		WebhookAlertOnDeath:   true,
		WebhookAlertOnRareDrop: true,
		WebhookAlertOnBagFull: true,
		WebhookAlertOnDisconnect: true,
	}
}

type FarmLootFeedItem struct {
	Timestamp string `json:"timestamp"`
	ItemName  string `json:"item_name"`
	ItemID    string `json:"item_id"`
	Count     int    `json:"count"`
	Rarity    int    `json:"rarity"`
	IsPenya   bool   `json:"is_penya"`
}

type FarmActiveBuff struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	SecondsLeft float32 `json:"seconds_left"`
	Icon        string  `json:"icon"`
}

type FarmStats struct {
	Running        bool               `json:"running"`
	State          string             `json:"state"`
	TargetName     string             `json:"target_name"`
	TargetHp       int                `json:"target_hp"`
	TargetMaxHp    int                `json:"target_max_hp"`
	Kills          int                `json:"kills"`
	KillsPerHour   float32            `json:"kills_per_hour"`
	ExpGained      int64              `json:"exp_gained"`
	ExpPerHour     int64              `json:"exp_per_hour"`
	TimeToLevelSec int64              `json:"time_to_level_sec"`
	PenyaGained    int64              `json:"penya_gained"`
	PenyaPerHour   int64              `json:"penya_per_hour"`
	SessionSec     int64              `json:"session_sec"`
	RecentLoot     []FarmLootFeedItem `json:"recent_loot"`
	ActiveBuffs    []FarmActiveBuff   `json:"active_buffs"`

	// Multi-Bot Coordination Telemetry
	CoordClaimedMob int    `json:"coord_claimed_mob"`
	CoordZone       string `json:"coord_zone"`
	CoordZoneMobs   string `json:"coord_zone_mobs"`
	CoordAssignedAt string `json:"coord_assigned_at"`
}
