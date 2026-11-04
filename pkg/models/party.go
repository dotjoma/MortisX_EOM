package models

import (
	"strings"
)

// Supported Party Skill IDs (Echoes of Madrigal / Flyff)
const (
	PartySkillLinkAttack    = "ST_LINKATTACK"    // Linked Attack: dmg * party_size / 10 (+80% at 8/8)
	PartySkillFortuneCircle = "ST_FORTUNECIRCLE" // Lucky Drop: rare & upgraded drop rate
	PartySkillGiftBox       = "ST_GIFTBOX"       // Gift Box: extra monster drop rolls
	PartySkillSphereCircle  = "ST_SPHERECIRCLE"  // Precision: critical chance bonus
	PartySkillStretching    = "ST_STRETCHING"    // Resting: 1.5x - 1.8x recovery speed
)

// Supported Party Scroll Item IDs (Echoes of Madrigal / Flyff)
const (
	PartyScrollSelectedParty = "II_SYS_SYS_SCR_SELPAR"         // Instantly raises party to Lv 10
	PartyScrollSkillFull     = "II_CHR_SYS_SCR_PSKILLFULL"     // Removes 255m skill range cap (world-wide)
	PartyScrollExpUp         = "II_SYS_SYS_SCR_PARTYEXPUP"     // +50% Party EXP gain
	PartyScrollSkillUp       = "II_SYS_SYS_SCR_PARTYSKILLUP"   // Allows early pre-expiry renewal
	PartyScrollSuperLeader   = "II_SYS_SYS_SCR_SUPERLEADERPARTY" // Leader combat enhancement
)

// MaxPartyMembers is the maximum whitelisted member count allowed in a party (excluding leader, max 8 total).
const MaxPartyMembers = 7

// DefaultLeashDistanceM is the default distance threshold in meters before a follower walks back to the leader.
const DefaultLeashDistanceM = 60.0

// ServerExpShareRangeM is the hard server limit in meters for shared kill EXP.
const ServerExpShareRangeM = 64.0

// PartyBoostConfig holds persistent configuration for the Party Boost module.
type PartyBoostConfig struct {
	Enabled         bool            `json:"enabled"`
	LeaderName      string          `json:"leader_name"`
	SelectedMembers []string        `json:"selected_members"`
	AutoReinvite    bool            `json:"auto_reinvite"`
	LeashEnabled    bool            `json:"leash_enabled"`
	LeashDistance   float64         `json:"leash_distance"`
	PartyName       string          `json:"party_name"`
	AutoAdvance     bool            `json:"auto_advance"`
	ExpShareMode    int             `json:"exp_share_mode"` // 0 = Level, 1 = Contribution
	ItemDistMode    int             `json:"item_dist_mode"` // 0 = Individually, 1 = Sequentially, 2 = Manually, 3 = Randomly
	LevelSync       bool            `json:"level_sync"`
	FocusFire       bool            `json:"focus_fire"`   // Party members focus leader's target
	AutoSupport     bool            `json:"auto_support"` // Auto-configure Assist/RM/BP to support leader
	SkillsEnabled   map[string]bool `json:"skills_enabled"`
	ScrollsEnabled  map[string]bool `json:"scrolls_enabled"`
}

// DefaultPartyBoostConfig returns a fresh PartyBoostConfig with safe, optimal defaults.
func DefaultPartyBoostConfig() PartyBoostConfig {
	return PartyBoostConfig{
		Enabled:         false,
		LeaderName:      "",
		SelectedMembers: make([]string, 0),
		AutoReinvite:    true,
		LeashEnabled:    true,
		LeashDistance:   DefaultLeashDistanceM,
		PartyName:       "Mortis Party",
		AutoAdvance:     true,
		ExpShareMode:    1, // Default: Contribution (+10% extra EXP)
		ItemDistMode:    1, // Default: Sequentially (fair distribution)
		LevelSync:       false,
		FocusFire:       true,
		AutoSupport:     true,
		SkillsEnabled: map[string]bool{
			PartySkillLinkAttack:    true,
			PartySkillFortuneCircle: false,
			PartySkillGiftBox:       false,
			PartySkillSphereCircle:  false,
			PartySkillStretching:    false,
		},
		ScrollsEnabled: map[string]bool{
			PartyScrollSelectedParty: false,
			PartyScrollSkillFull:     false,
			PartyScrollExpUp:         false,
			PartyScrollSkillUp:       false,
			PartyScrollSuperLeader:   false,
		},
	}
}

// Sanitize normalizes and validates the config bounds.
func (c *PartyBoostConfig) Sanitize() {
	if c.LeashDistance <= 0 {
		c.LeashDistance = DefaultLeashDistanceM
	}

	if c.ExpShareMode < 0 || c.ExpShareMode > 1 {
		c.ExpShareMode = 1
	}

	if c.ItemDistMode < 0 || c.ItemDistMode > 3 {
		c.ItemDistMode = 1
	}

	c.LeaderName = strings.TrimSpace(c.LeaderName)

	if c.SkillsEnabled == nil {
		c.SkillsEnabled = make(map[string]bool)
	}
	defaultSkills := []string{
		PartySkillLinkAttack, PartySkillFortuneCircle, PartySkillGiftBox,
		PartySkillSphereCircle, PartySkillStretching,
	}
	for _, s := range defaultSkills {
		if _, exists := c.SkillsEnabled[s]; !exists {
			c.SkillsEnabled[s] = (s == PartySkillLinkAttack)
		}
	}

	if c.ScrollsEnabled == nil {
		c.ScrollsEnabled = make(map[string]bool)
	}
	defaultScrolls := []string{
		PartyScrollSelectedParty, PartyScrollSkillFull, PartyScrollExpUp,
		PartyScrollSkillUp, PartyScrollSuperLeader,
	}
	for _, s := range defaultScrolls {
		if _, exists := c.ScrollsEnabled[s]; !exists {
			c.ScrollsEnabled[s] = false
		}
	}

	// Filter and cap whitelisted members
	leaderLower := strings.ToLower(c.LeaderName)
	seen := make(map[string]bool)
	filtered := make([]string, 0, len(c.SelectedMembers))
	for _, m := range c.SelectedMembers {
		trimmed := strings.TrimSpace(m)
		if trimmed == "" {
			continue
		}
		tLower := strings.ToLower(trimmed)
		if tLower == leaderLower || seen[tLower] {
			continue
		}
		seen[tLower] = true
		filtered = append(filtered, trimmed)
		if len(filtered) >= MaxPartyMembers {
			break
		}
	}
	c.SelectedMembers = filtered
}

// PartyMember represents an individual member entry parsed from the server party packet (party.m).
type PartyMember struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Level int     `json:"level"`
	Job   int     `json:"job"`
	HP    int     `json:"hp"`
	X     float32 `json:"x"`
	Z     float32 `json:"z"`
	World int     `json:"world"`
}

// PartyRosterEntry represents a discovered bot instance in the Multibot pool for Party Boost coordination.
type PartyRosterEntry struct {
	BotID            string  `json:"bot_id"`
	AccountName      string  `json:"account_name"`
	CharacterName    string  `json:"character_name"`
	Level            uint32  `json:"level"`
	Job              string  `json:"job"`
	Channel          string  `json:"channel"`
	Status           string  `json:"status"` // "online", "disconnected", etc.
	InParty          bool    `json:"in_party"`
	PartyID          int     `json:"party_id,omitempty"`
	PartyName        string  `json:"party_name,omitempty"`
	PartyLeader      string  `json:"party_leader,omitempty"`
	PartyLevel       int     `json:"party_level,omitempty"`
	PartyKind        int     `json:"party_kind,omitempty"`
	PartyShare       int     `json:"party_share,omitempty"`
	PartyDist        int                `json:"party_dist,omitempty"`
	PartyPoints      int                `json:"party_points,omitempty"`
	PartyScrollFull  bool               `json:"party_scroll_full,omitempty"`
	PartyScrollExpUp bool               `json:"party_scroll_exp_up,omitempty"`
	PartyScrollSkillUp bool             `json:"party_scroll_skill_up,omitempty"`
	PartyModes       map[string]float64 `json:"party_modes,omitempty"`
	PartySkillCD     map[string]float64 `json:"party_skill_cd,omitempty"`
	IsLeader         bool               `json:"is_leader"`
	DistanceToLeader float64 `json:"distance_to_leader"` // In meters
	InShareRange     bool    `json:"in_share_range"`      // distance <= 64.0m
	LevelSyncActive  bool    `json:"level_sync_active,omitempty"`
	SyncLevel        int     `json:"sync_level,omitempty"`
}
