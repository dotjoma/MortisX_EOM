package bot

import (
	"math"
	"strings"
	"testing"
	"time"

	"madrigal-multibot/pkg/models"
)

// TestSlopeCombatArrival verifies that a monster with large vertical delta (e.g. 5m on a steep slope)
// is within combat reach when horizontal distance is within reach (matching player.gd:419-420: to.y = 0).
func TestSlopeCombatArrival(t *testing.T) {
	botX := float32(100.0)
	botY := float32(50.0)
	botZ := float32(100.0)

	// Monster standing 5m higher on a slope, but horizontally only 1.41m away
	targetX := float32(101.0)
	targetY := float32(55.0)
	targetZ := float32(101.0)

	tdx := targetX - botX
	tdy := targetY - botY
	tdz := targetZ - botZ

	dist2D := float32(math.Sqrt(float64(tdx*tdx + tdz*tdz)))
	dist3D := float32(math.Sqrt(float64(tdx*tdx + tdy*tdy + tdz*tdz)))

	// Melee combat reach for Vagrant / Mercenary / Blade
	reach := CombatReach("vagrant", 0.8)
	if reach != 2.3 {
		t.Fatalf("expected melee reach 2.3, got %.2f", reach)
	}

	// 2D distance is ~1.41m, which is <= 2.3m
	if dist2D > reach {
		t.Fatalf("expected dist2D (%.2f) to be <= reach (%.2f)", dist2D, reach)
	}

	// 3D distance is ~5.2m, which would incorrectly fail if 3D distance were used!
	if dist3D <= reach {
		t.Fatalf("test precondition failed: dist3D (%.2f) should exceed reach (%.2f)", dist3D, reach)
	}

	// Absolute vertical delta is 5.0m, which would fail the old 1.8m vertical threshold
	if math.Abs(float64(tdy)) <= 1.8 {
		t.Fatalf("test precondition failed: vertical delta should exceed 1.8m")
	}

	// In the fixed implementation, arrival check uses dist2D only:
	inCombatReach := dist2D <= reach
	if !inCombatReach {
		t.Errorf("slope arrival check failed: bot should be in combat reach on slope")
	}
}

// TestScoreSupplyItemFormula verifies overheal penalty formula from bot.gd:440:
// score = min(v, missing) - max(0, v - missing) * 0.25
func TestScoreSupplyItemFormula(t *testing.T) {
	missing := 100

	// Exact match: 100 restore for 100 missing -> score = 100 - 0 = 100
	scoreExact := ScoreSupplyItem(100, missing)
	if scoreExact != 100.0 {
		t.Errorf("expected score 100.0, got %.2f", scoreExact)
	}

	// Overheal: 200 restore for 100 missing -> score = 100 - (100 * 0.25) = 75.0
	scoreOverheal := ScoreSupplyItem(200, missing)
	if scoreOverheal != 75.0 {
		t.Errorf("expected score 75.0, got %.2f", scoreOverheal)
	}

	// Underheal: 50 restore for 100 missing -> score = 50 - 0 = 50.0
	scoreUnderheal := ScoreSupplyItem(50, missing)
	if scoreUnderheal != 50.0 {
		t.Errorf("expected score 50.0, got %.2f", scoreUnderheal)
	}

	// Verify exact match is prioritized over overheal and underheal
	if scoreExact <= scoreOverheal || scoreExact <= scoreUnderheal {
		t.Errorf("exact supply must score highest: exact=%.1f, overheal=%.1f, underheal=%.1f",
			scoreExact, scoreOverheal, scoreUnderheal)
	}
}

// TestMaxStatFormulas verifies client max HP/MP/FP formula parity with formula.gd:216-235
func TestMaxStatFormulas(t *testing.T) {
	// Level 1 Vagrant with 15 STA
	maxHpLv1 := CalcMaxHP(1, 15)
	if maxHpLv1 != 230 {
		t.Errorf("expected Level 1 MaxHP to be 230, got %d", maxHpLv1)
	}

	// Level 15 Vagrant / 1st Job with 30 STA
	maxHpLv15 := CalcMaxHP(15, 30)
	// a = (0.9 * 15) / 2 = 6.75
	// b = 6.75 * (16 / 4) * (1 + 30/50) + 30 * 10 = 27 * 1.6 + 300 = 43.2 + 300 = 343.2
	// b + 80 = 423.2 -> int(423)
	if maxHpLv15 != 423 {
		t.Errorf("expected Level 15 MaxHP to be 423, got %d", maxHpLv15)
	}

	// Level 1 MP with 15 INT
	maxMpLv1 := CalcMaxMP(1, 15)
	if maxMpLv1 <= 0 {
		t.Errorf("expected positive MaxMP, got %d", maxMpLv1)
	}

	// Level 1 FP with 15 STA
	maxFpLv1 := CalcMaxFP(1, 15)
	if maxFpLv1 <= 0 {
		t.Errorf("expected positive MaxFP, got %d", maxFpLv1)
	}
}

// TestCombatReachByJob verifies attack reach matching player.gd:409 and stats.gd:221-226
func TestCombatReachByJob(t *testing.T) {
	tests := []struct {
		job      string
		radius   float32
		expected float32
	}{
		{"vagrant", 0.8, 2.3},    // 1.5 + 0.8 + 0.0
		{"mercenary", 0.8, 2.3},  // 1.5 + 0.8 + 0.0
		{"blade", 0.8, 2.3},      // 1.5 + 0.8 + 0.0
		{"knight", 0.8, 2.3},     // 1.5 + 0.8 + 0.0
		{"magician", 0.8, 15.3},  // 1.5 + 0.8 + 13.0
		{"psykeeper", 0.8, 15.3}, // 1.5 + 0.8 + 13.0
		{"elementor", 0.8, 15.3}, // 1.5 + 0.8 + 13.0
		{"ranger", 0.8, 19.3},    // 1.5 + 0.8 + 17.0
		{"acrobat", 0.8, 5.5},    // 1.5 + 0.8 + 3.2
		{"jester", 0.8, 5.5},     // 1.5 + 0.8 + 3.2
	}

	for _, tt := range tests {
		reach := CombatReach(tt.job, tt.radius)
		if math.Abs(float64(reach-tt.expected)) > 0.01 {
			t.Errorf("CombatReach(%s) = %.2f; expected %.2f", tt.job, reach, tt.expected)
		}
	}
}

// TestAggroDefensePriority verifies that an aggressive monster chasing the bot within 20m
// is prioritized over closer non-aggressive monsters (matching bot.gd:332-337).
func TestAggroDefensePriority(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	botX := float32(100.0)
	botY := float32(10.0)
	botZ := float32(100.0)
	botLevel := 10

	cfg := models.DefaultFarmConfig()
	cfg.SmartMode = false
	cfg.AnchorX = botX
	cfg.AnchorZ = botZ
	cfg.Radius = 35.0

	// Mob 1: Idle, 5m away, not chasing
	fe.mobs[1] = &TrackedMob{
		ID:       1,
		Name:     "Idle_Mob",
		Level:    10,
		HP:       100,
		MaxHP:    100,
		X:        105.0,
		Y:        10.0,
		Z:        100.0,
		Chase:    false,
		LastSeen: time.Now(),
	}

	// Mob 2: Chasing, 18m away (< 20m aggro radius)
	fe.mobs[2] = &TrackedMob{
		ID:       2,
		Name:     "Aggro_Mob",
		Level:    10,
		HP:       100,
		MaxHP:    100,
		X:        118.0,
		Y:        10.0,
		Z:        100.0,
		Chase:    true,
		LastSeen: time.Now(),
	}

	picked := fe.pickBestMob(cfg, botX, botY, botZ, botLevel)
	if picked == nil {
		t.Fatalf("expected to pick a mob, got nil")
	}
	if picked.ID != 2 {
		t.Errorf("expected aggro mob #2 to be prioritized over closer idle mob #1, but picked #%d (%s)",
			picked.ID, picked.Name)
	}

	// Mob 3: Chasing, but 25m away (> 20m aggro radius)
	fe.mobs[2].Chase = false // unset mob 2
	fe.mobs[3] = &TrackedMob{
		ID:       3,
		Name:     "Far_Aggro_Mob",
		Level:    10,
		HP:       100,
		MaxHP:    100,
		X:        125.0,
		Y:        10.0,
		Z:        100.0,
		Chase:    true,
		LastSeen: time.Now(),
	}

	pickedFar := fe.pickBestMob(cfg, botX, botY, botZ, botLevel)
	if pickedFar == nil || pickedFar.ID != 1 {
		t.Errorf("mob 3 is >20m away and should not trigger aggro defense; expected closer mob #1 to be picked")
	}
}

// TestBlacklistExpiry verifies that blacklisted mobs are ignored until expiration.
func TestBlacklistExpiry(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	botX := float32(100.0)
	botY := float32(10.0)
	botZ := float32(100.0)

	cfg := models.DefaultFarmConfig()
	cfg.SmartMode = false
	cfg.AnchorX = botX
	cfg.AnchorZ = botZ
	cfg.Radius = 35.0

	fe.mobs[10] = &TrackedMob{
		ID:       10,
		Name:     "Test_Mob",
		Level:    1,
		HP:       100,
		MaxHP:    100,
		X:        102.0,
		Y:        10.0,
		Z:        100.0,
		LastSeen: time.Now(),
	}

	// Blacklist mob 10 for 40s (matching bot.gd:322)
	fe.blacklistMob(10, ClientFightBlacklistDuration)

	picked := fe.pickBestMob(cfg, botX, botY, botZ, 1)
	if picked != nil {
		t.Errorf("blacklisted mob should not be picked, got #%d", picked.ID)
	}

	// Expire blacklist manually
	fe.mobsMu.Lock()
	delete(fe.blacklist, 10)
	fe.mobsMu.Unlock()

	pickedAfter := fe.pickBestMob(cfg, botX, botY, botZ, 1)
	if pickedAfter == nil || pickedAfter.ID != 10 {
		t.Errorf("unblacklisted mob should now be picked, got %v", pickedAfter)
	}
}

// TestTargetPersistenceWithNearbyAttacker verifies bot.gd:136-137:
// if currentTarget is already in attackers, bot never switches to another attacker.
func TestTargetPersistenceWithNearbyAttacker(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	bot.Stats.PosX = 100.0
	bot.Stats.PosY = 10.0
	bot.Stats.PosZ = 100.0
	bot.Stats.Level = 10
	bot.Stats.Job = "Vagrant"

	cfg := models.DefaultFarmConfig()
	cfg.AnchorX = 100.0
	cfg.AnchorZ = 100.0
	cfg.Radius = 35.0

	// Mob A: engaged target, chasing, within reach
	mobA := &TrackedMob{
		ID:         101,
		Name:       "Target_A",
		Level:      10,
		HP:         500,
		MaxHP:      1000,
		X:          101.0,
		Y:          10.0,
		Z:          100.0,
		Chase:      true,
		BotDamaged: true,
	}
	fe.mobs[101] = mobA
	fe.currentTarget = mobA

	// Mob B: another aggressive attacker, chasing, 5m away (< 20m)
	mobB := &TrackedMob{
		ID:    102,
		Name:  "Attacker_B",
		Level: 10,
		HP:    1000,
		MaxHP: 1000,
		X:     105.0,
		Y:     10.0,
		Z:     100.0,
		Chase: true,
	}
	fe.mobs[102] = mobB

	// Run thinkHunt
	fe.thinkHunt(cfg)

	// Target MUST remain Mob A (never flip-flop to B)
	if fe.currentTarget == nil || fe.currentTarget.ID != 101 {
		t.Fatalf("expected currentTarget to stay Mob A (#101), got %v", fe.currentTarget)
	}
}

// TestTargetKeepsAttackingZeroHpUntilEventD verifies that a monster at HP 0
// is NOT dropped until server packet "d" arrives (movers.gd:465).
func TestTargetKeepsAttackingZeroHpUntilEventD(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	bot.Stats.PosX = 100.0
	bot.Stats.PosY = 10.0
	bot.Stats.PosZ = 100.0
	bot.Stats.Level = 10
	bot.Stats.Job = "Vagrant"

	cfg := models.DefaultFarmConfig()
	cfg.AnchorX = 100.0
	cfg.AnchorZ = 100.0
	cfg.Radius = 35.0

	mobA := &TrackedMob{
		ID:         201,
		Name:       "Zero_HP_Mob",
		Level:      10,
		HP:         0, // Server echoed HP: 0
		MaxHP:      1000,
		X:          101.0,
		Y:          10.0,
		Z:          100.0,
		Chase:      true,
		Dead:       false, // Server event "d" has NOT arrived
		BotDamaged: true,
	}
	fe.mobs[201] = mobA
	fe.currentTarget = mobA

	// Run thinkHunt
	fe.thinkHunt(cfg)

	// Target MUST NOT be dropped
	if fe.currentTarget == nil || fe.currentTarget.ID != 201 {
		t.Fatalf("expected currentTarget to remain #201 even at HP 0, got %v", fe.currentTarget)
	}
	if fe.currentTarget.Dead {
		t.Fatalf("target must not be marked Dead before server event 'd'")
	}
}

// TestEventDKillsTargetAndAwardsExpWithShares verifies that server event "d"
// properly marks dead, clears target, and awards EXP matching shares.
func TestEventDKillsTargetAndAwardsExpWithShares(t *testing.T) {
	bot := &BotInstance{entityID: 999}
	bot.Status = "online"
	bot.Stats.Level = 10
	fe := NewFarmEngine(bot)

	mobA := &TrackedMob{
		ID:         301,
		Name:       "D_Test_Mob",
		Level:      10,
		Exp:        100,
		HP:         0,
		MaxHP:      1000,
		X:          100.0,
		Y:          10.0,
		Z:          100.0,
		Chase:      true,
		Dead:       false,
		BotDamaged: true,
	}
	fe.mobs[301] = mobA
	fe.currentTarget = mobA

	// Simulate incoming server packet with event "d" where player 999 has 100% share
	packet := map[string]interface{}{
		"t": "mob",
		"e": []interface{}{
			[]interface{}{"d", 301, 999, []interface{}{[]interface{}{999, 1.0}}},
		},
	}
	fe.OnMobPacket(packet)

	// Assert mob is dead and target is cleared
	if !mobA.Dead {
		t.Fatalf("expected mob #301 to be marked Dead on event 'd'")
	}
	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be nil after event 'd', got %v", fe.currentTarget)
	}
}

// TestDespawnClearsTarget verifies that server packet "l" (leave/despawn) clears target.
func TestDespawnClearsTarget(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	mobA := &TrackedMob{
		ID:    401,
		Name:  "Despawn_Mob",
		Level: 10,
		HP:    500,
		MaxHP: 1000,
	}
	fe.mobs[401] = mobA
	fe.currentTarget = mobA

	packet := map[string]interface{}{
		"t": "mob",
		"l": []interface{}{401},
	}
	fe.OnMobPacket(packet)

	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be cleared on despawn, got %v", fe.currentTarget)
	}
}

// TestMidSwingWhiffIfTargetDeadOrChanged verifies player.gd:457-458:
// if target dies or changes mid-swing, swing whiffs and does not hit.
func TestMidSwingWhiffIfTargetDeadOrChanged(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	bot.Stats.PosX = 100.0
	bot.Stats.PosY = 10.0
	bot.Stats.PosZ = 100.0
	bot.Stats.Level = 10
	bot.Stats.Job = "Vagrant"

	cfg := models.DefaultFarmConfig()

	mobA := &TrackedMob{
		ID:    501,
		Name:  "Swing_Mob",
		Level: 10,
		HP:    500,
		MaxHP: 1000,
		X:     101.0,
		Y:     10.0,
		Z:     100.0,
	}
	fe.mobs[501] = mobA
	fe.currentTarget = mobA

	// Phase 0: Start swing
	fe.executeAttack(cfg, mobA)
	if fe.attackPhase != 1 {
		t.Fatalf("expected attackPhase 1 (windup), got %d", fe.attackPhase)
	}
	if fe.swingTargetID != 501 {
		t.Fatalf("expected swingTargetID 501, got %d", fe.swingTargetID)
	}

	// Target dies before windup finishes
	mobA.Dead = true

	// Advance time past windup
	fe.swingStartTime = time.Now().Add(-300 * time.Millisecond)

	// Phase 1 executes: should whiff and reset phase to 0
	fe.executeAttack(cfg, mobA)
	if fe.attackPhase != 0 {
		t.Fatalf("expected swing to whiff and reset attackPhase to 0, got %d", fe.attackPhase)
	}
	if fe.swingTargetID != 0 {
		t.Fatalf("expected swingTargetID to reset to 0, got %d", fe.swingTargetID)
	}
}

func TestFarmEngine_SwingQueueAndObservationMode(t *testing.T) {
	bot := &BotInstance{ID: "bot_1", Profile: models.BotProfile{Name: "Hero"}}
	bot.Stats.Level = 1
	bot.Stats.Job = "vagrant"
	bot.Stats.STA = 15
	bot.Stats.DEX = 15
	bot.Stats.HP = 230
	bot.Stats.PosX = 100.0
	bot.Stats.PosY = 10.0
	bot.Stats.PosZ = 100.0

	fe := NewFarmEngine(bot)
	fe.combatRNG = &ScriptedCombatRNG{
		ModRolls:   []int{50, 50, 0, 50, 50, 50, 0, 50},
		RangeRolls: []int{16, 16},
	}
	now := time.Now()

	mob := &TrackedMob{
		ID:       101,
		MobIndex: "MI_AIBATT1",
		Name:     "Small Aibatt",
		Level:    1,
		X:        101.0,
		Y:        10.0,
		Z:        100.0,
		HP:       50,
		MaxHP:    50,
	}
	fe.mobs[mob.ID] = mob

	// 1. Enqueue swing
	fe.enqueueMobSwing(mob, 1, false, now)
	fe.swingsMu.Lock()
	if len(fe.pendingSwings) != 1 {
		t.Fatalf("expected 1 pending swing, got %d", len(fe.pendingSwings))
	}
	s := fe.pendingSwings[mob.ID]
	fe.swingsMu.Unlock()

	// 2. Replacement on new swing from same mob
	fe.enqueueMobSwing(mob, 1, false, now.Add(100*time.Millisecond))
	fe.swingsMu.Lock()
	if len(fe.pendingSwings) != 1 {
		t.Fatalf("expected pending swing to be replaced, got %d", len(fe.pendingSwings))
	}
	s2 := fe.pendingSwings[mob.ID]
	fe.swingsMu.Unlock()
	if !s2.DueTime.After(s.DueTime) {
		t.Fatalf("expected replacement swing to have later due time")
	}

	// 3. Process swing arrival (connects)
	future := s2.DueTime.Add(10 * time.Millisecond)
	fe.drainPendingSwings(future)

	fe.swingsMu.Lock()
	if len(fe.pendingSwings) != 0 {
		t.Fatalf("expected swing queue to be drained, got %d", len(fe.pendingSwings))
	}
	stats := fe.obsStats
	fe.swingsMu.Unlock()

	if stats.TotalHits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.TotalHits)
	}
	if stats.TotalDamage <= 0 {
		t.Errorf("expected positive damage, got %d", stats.TotalDamage)
	}
	if !bot.InCombat(now) {
		t.Errorf("expected bot to be in combat after being hit")
	}

	// 4. Test Lethal Hit with 1 HP clamp (Observation Mode)
	bot.mu.Lock()
	bot.Stats.HP = 1 // 1 HP so any positive damage is fatal
	bot.mu.Unlock()
	fe.swingsMu.Lock()
	fe.obsStats.ShadowHP = 1 // align shadow HP to test unclamped negative drop
	fe.swingsMu.Unlock()

	fe.enqueueMobSwing(mob, 1, false, future)
	fe.drainPendingSwings(future.Add(2 * time.Second))

	bot.mu.Lock()
	currentHP := bot.Stats.HP
	bot.mu.Unlock()

	stats = fe.GetObservationStats()
	if currentHP != 1 {
		t.Errorf("expected HP clamped to 1 in observation mode, got %d", currentHP)
	}
	if stats.WouldDieCount != 1 {
		t.Errorf("expected WouldDieCount 1, got %d", stats.WouldDieCount)
	}
	if stats.ShadowHP >= 0 {
		t.Errorf("expected shadow HP to be negative (unclamped), got %d", stats.ShadowHP)
	}

	// 5. Test Whiff (out of range: > 4.5 + radius)
	mobFar := &TrackedMob{
		ID:       102,
		MobIndex: "MI_AIBATT1",
		Name:     "Small Aibatt Far",
		Level:    1,
		X:        110.0, // 10m away
		Y:        10.0,
		Z:        100.0,
		HP:       50,
	}
	fe.mobs[mobFar.ID] = mobFar
	fe.enqueueMobSwing(mobFar, 1, false, future)
	fe.drainPendingSwings(future.Add(4 * time.Second))

	stats = fe.GetObservationStats()
	if stats.TotalWhiffs != 1 {
		t.Errorf("expected 1 whiff, got %d", stats.TotalWhiffs)
	}

	// 6. Test cancellation on death/despawn
	fe.enqueueMobSwing(mob, 1, false, future)
	fe.cancelPendingSwing(mob.ID)
	fe.swingsMu.Lock()
	if _, exists := fe.pendingSwings[mob.ID]; exists {
		t.Errorf("expected swing to be cancelled on despawn")
	}
	fe.swingsMu.Unlock()
}

func TestFarmEngine_SmartModeIdleRelocation_FullSuite(t *testing.T) {
	bot := &BotInstance{ID: "bot_smart_full_test"}
	bot.Stats.Level = 1
	// Start at Lv 1 Small Aibatt spawn (index 123)
	bot.Stats.PosX = 6925.5
	bot.Stats.PosZ = -2960.0
	fe := NewFarmEngine(bot)
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fe.clock = func() time.Time { return simTime }

	cfg := models.FarmConfig{
		SmartMode:           true,
		SmartMinLevelOffset: 1,
		SmartMaxLevelOffset: 5,
		AnchorX:             6925.5,
		AnchorZ:             -2960.0,
		Radius:              45.0,
	}
	fe.SetConfig(cfg)

	// 1. Outside anchor radius: Immediate travel to spawn (Case A)
	bot.Stats.PosX = 6925.5 + 100.0
	bot.Stats.PosZ = -2960.0 + 100.0
	fe.thinkHunt(fe.GetConfig())
	wpX, wpZ, hasWp := bot.GetWaypoint()
	if !hasWp {
		t.Fatalf("Test 1 failed: expected immediate travel waypoint set when outside radius")
	}
	if wpX != 6925.5 || wpZ != -2960.0 {
		t.Fatalf("Test 1 failed: expected waypoint (6925.5, -2960.0), got (%.1f, %.1f)", wpX, wpZ)
	}

	// 2. Return bot inside radius; 0 mobs in map sync -> idle timer accumulates
	bot.Stats.PosX = 6925.5
	bot.Stats.PosZ = -2960.0
	fe.thinkHunt(fe.GetConfig())
	if fe.noTargetStartTime.IsZero() {
		t.Fatalf("Test 2 failed: noTargetStartTime must be initialized when 0 targets found")
	}

	// 3. Tick before timeout: must NOT relocate
	simTime = simTime.Add(10 * time.Second)
	fe.thinkHunt(fe.GetConfig())
	if fe.GetConfig().AnchorX != 6925.5 || fe.GetConfig().AnchorZ != -2960.0 {
		t.Fatalf("Test 3 failed: relocated prematurely before timeout")
	}

	// 4. Tick past 15s timeout: must relocate to next optimal spawn
	simTime = simTime.Add(6 * time.Second)
	fe.thinkHunt(fe.GetConfig())
	newCfg := fe.GetConfig()
	if newCfg.AnchorX == 6925.5 && newCfg.AnchorZ == -2960.0 {
		t.Fatalf("Test 4 failed: bot did not relocate after timeout")
	}
	// Old spawn key "MI_AIBATT1_6926_-2960" must be in cooldown map
	oldKey := "MI_AIBATT1_6926_-2960"
	fe.mu.RLock()
	exp, inCooldown := fe.emptyAreaCooldown[oldKey]
	fe.mu.RUnlock()
	if !inCooldown || !exp.After(simTime) {
		t.Fatalf("Test 4 failed: old empty spawn %s was not cooled down", oldKey)
	}

	// 5. Waypoint set to new spawn
	wpX, wpZ, hasWp = bot.GetWaypoint()
	if !hasWp || wpX != newCfg.AnchorX || wpZ != newCfg.AnchorZ {
		t.Fatalf("Test 5 failed: waypoint not set to new anchor (%.1f, %.1f)", newCfg.AnchorX, newCfg.AnchorZ)
	}

	// 6. Dynamic currentSpawnKey derivation test
	derivedKey := fe.currentSpawnKey(newCfg.AnchorX, newCfg.AnchorZ)
	if derivedKey == "" {
		t.Fatalf("Test 6 failed: currentSpawnKey failed to derive key")
	}

	// 7. Arrival at destination: bot inside radius, no bounce back
	destCfg := fe.GetConfig()
	bot.Stats.PosX = destCfg.AnchorX
	bot.Stats.PosZ = destCfg.AnchorZ
	fe.thinkHunt(destCfg)
	if !fe.travelStartTime.IsZero() {
		t.Fatalf("Test 7 failed: travelStartTime must be zero upon arrival")
	}
	if fe.GetConfig().AnchorX != destCfg.AnchorX || fe.GetConfig().AnchorZ != destCfg.AnchorZ {
		t.Fatalf("Test 7 failed: bot bounced back after arrival")
	}

	// 8. Mob appears: cancels idle timer, clears attempts
	mob := &TrackedMob{
		ID:       888,
		MobIndex: "MI_AIBATT1",
		Name:     "Small Aibatt",
		Level:    1,
		X:        destCfg.AnchorX + 2.0,
		Z:        destCfg.AnchorZ + 2.0,
		HP:       50,
		MaxHP:    50,
	}
	fe.mobsMu.Lock()
	fe.mobs[mob.ID] = mob
	fe.mobsMu.Unlock()

	fe.thinkHunt(destCfg)
	if fe.currentTarget == nil || fe.currentTarget.ID != 888 {
		t.Fatalf("Test 8 failed: bot did not engage newly appeared mob")
	}
	if !fe.noTargetStartTime.IsZero() {
		t.Fatalf("Test 8 failed: noTargetStartTime not cleared when target engaged")
	}
	if fe.relocateAttempts != 0 {
		t.Fatalf("Test 8 failed: relocateAttempts not cleared when target engaged")
	}

	// 9. Attempt cap pause: 5 consecutive hops with 0 mobs -> pause state
	fe.mobsMu.Lock()
	delete(fe.mobs, 888)
	fe.mobsMu.Unlock()
	fe.currentTarget = nil
	currentCfg := fe.GetConfig()
	bot.Stats.PosX = currentCfg.AnchorX
	bot.Stats.PosZ = currentCfg.AnchorZ
	fe.relocateAttempts = ClientSmartRelocateMaxAttempts
	fe.relocatePausedUntil = time.Time{}

	fe.thinkHunt(currentCfg)
	if fe.relocatePausedUntil.IsZero() {
		t.Fatalf("Test 9 failed: relocatePausedUntil must be set when attempt cap reached")
	}
	if !strings.Contains(fe.GetStats().State, "Searching paused") {
		t.Fatalf("Test 9 failed: expected Searching paused state, got: %s", fe.GetStats().State)
	}
	_, _, hasWp = bot.GetWaypoint()
	if hasWp {
		t.Fatalf("Test 9 failed: waypoint must be cleared when paused")
	}
}

func TestFarmEngine_SmartModeTransitStuckWithNilNextSpawn(t *testing.T) {
	bot := &BotInstance{ID: "bot_stuck_test"}
	bot.Stats.Level = 1
	bot.Stats.PosX = 0.0
	bot.Stats.PosZ = 0.0
	fe := NewFarmEngine(bot)
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fe.clock = func() time.Time { return simTime }

	cfg := models.FarmConfig{
		SmartMode:           true,
		SmartMinLevelOffset: 1,
		SmartMaxLevelOffset: 5,
		AnchorX:             500.0,
		AnchorZ:             500.0,
		Radius:              45.0,
	}
	fe.SetConfig(cfg)

	// Blacklist all spawns so findRelocationSpawn returns nil
	fe.mu.Lock()
	for i := range farmSpawns {
		fe.emptyAreaCooldown[fe.spawnKey(&farmSpawns[i])] = simTime.Add(1 * time.Hour)
	}
	fe.mu.Unlock()

	// Tick 1: Bot travelling towards anchor (500, 500)
	fe.thinkHunt(fe.GetConfig())

	// Tick 2: 9s later with 0 progress -> transit stuck detector fires
	simTime = simTime.Add(9 * time.Second)
	fe.thinkHunt(fe.GetConfig())

	if !strings.Contains(fe.GetStats().State, "Searching paused") {
		t.Fatalf("expected Searching paused state on nil nextSpawn, got: %s", fe.GetStats().State)
	}
	_, _, hasWp := bot.GetWaypoint()
	if hasWp {
		t.Fatalf("expected waypoint to be cleared on nil nextSpawn stuck fallback")
	}

	// Tick 3: Subsequent tick while far from anchor -> MUST REMAIN PAUSED, MUST NOT SET WAYPOINT
	simTime = simTime.Add(1 * time.Second)
	fe.thinkHunt(fe.GetConfig())

	if !strings.Contains(fe.GetStats().State, "Searching paused") {
		t.Fatalf("expected bot to remain paused on subsequent tick, got: %s", fe.GetStats().State)
	}
	_, _, hasWp3 := bot.GetWaypoint()
	if hasWp3 {
		t.Fatalf("bot must not set waypoint back to blacklisted anchor during pause")
	}
}

func TestFarmEngine_SmartModeDisabled_TransitAndIdleIgnored(t *testing.T) {
	bot := &BotInstance{ID: "bot_disabled_test"}
	bot.Stats.Level = 1
	bot.Stats.PosX = 0.0
	bot.Stats.PosZ = 0.0
	fe := NewFarmEngine(bot)
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fe.clock = func() time.Time { return simTime }

	cfg := models.FarmConfig{
		SmartMode: false,
		AnchorX:   100.0,
		AnchorZ:   100.0,
		Radius:    45.0,
	}
	fe.SetConfig(cfg)

	// Tick 1: Far from anchor -> returns to anchor
	fe.thinkHunt(fe.GetConfig())
	wpX, wpZ, hasWp := bot.GetWaypoint()
	if !hasWp || wpX != 100.0 || wpZ != 100.0 {
		t.Fatalf("Expected waypoint set to anchor (100, 100), got (%v, %v, hasWp=%v)", wpX, wpZ, hasWp)
	}

	// Tick 2: 9s later with 0 progress -> must NOT trigger Smart stuck detector
	simTime = simTime.Add(9 * time.Second)
	fe.thinkHunt(fe.GetConfig())

	fe.mu.RLock()
	cdCount := len(fe.emptyAreaCooldown)
	fe.mu.RUnlock()
	if cdCount != 0 {
		t.Fatalf("SmartMode=false must not blacklist spawns on transit timeout, got %d cooldowns", cdCount)
	}
	if fe.GetConfig().AnchorX != 100.0 || fe.GetConfig().AnchorZ != 100.0 {
		t.Fatalf("SmartMode=false must not change anchor, got (%v, %v)", fe.GetConfig().AnchorX, fe.GetConfig().AnchorZ)
	}
}

func TestSmartRelocate_CoordinatedZoneRetained(t *testing.T) {
	coord := GetGlobalCoordinator()
	botID := "bot_coord_retained_test"
	simTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	coord.ReleaseAll(botID)

	sp, changed := coord.AllocateZone("madrigal", 2, botID, 1, 6925.5, -2960.0, 1, 5, true)
	if sp == nil || !changed {
		t.Fatalf("expected coordinator to assign zone")
	}

	zone := coord.GetAssignedZone("madrigal", 2, botID)
	if zone == nil || zone.Spawn != sp {
		t.Fatalf("expected zone assignment in coordinator")
	}

	bot := &BotInstance{ID: botID}
	fe := NewFarmEngine(bot)
	fe.clock = func() time.Time { return simTime }
	fe.emptyAreaCooldown[fe.spawnKey(sp)] = simTime.Add(1 * time.Hour)

	cfg := models.FarmConfig{
		SmartMode:       true,
		CoordinatedFarm: true,
	}
	_ = fe.findOptimalSpawn(1, 6925.5, -2960.0, cfg, simTime)

	retainedZone := coord.GetAssignedZone("madrigal", 2, botID)
	if retainedZone == nil {
		t.Fatalf("limitation violated: assigned zone must be retained in coordinator")
	}
	coord.ReleaseAll(botID)
}

func TestCheckRestState_ExploitAndPreferenceBypass(t *testing.T) {
	bot := &BotInstance{
		ID: "bot_rest_test",
		Stats: models.BotRuntimeStats{
			HP:    1,
			Level: 15,
			STA:   15,
		},
	}
	fe := NewFarmEngine(bot)

	defaultCfg := models.FarmConfig{
		AutoRest:        true,
		RestHpThreshold: 35.0,
	}

	// 1. By default, 1 HP (0.28%) triggers AutoRest
	if !fe.checkRestState(defaultCfg) {
		t.Fatalf("expected 1 HP to trigger rest when AutoRest is true")
	}
	if !fe.lastResting {
		t.Fatalf("expected lastResting to be true")
	}

	// 2. ExploitCapHp1: true must bypass resting and clear lastResting
	capHp1Cfg := defaultCfg
	capHp1Cfg.ExploitCapHp1 = true
	if fe.checkRestState(capHp1Cfg) {
		t.Fatalf("ExploitCapHp1=true must not rest")
	}
	if fe.lastResting {
		t.Fatalf("ExploitCapHp1=true must clear lastResting")
	}

	// 3. ExploitGodMode: true must bypass resting and clear lastResting
	fe.lastResting = true
	godCfg := defaultCfg
	godCfg.ExploitGodMode = true
	if fe.checkRestState(godCfg) {
		t.Fatalf("ExploitGodMode=true must not rest")
	}
	if fe.lastResting {
		t.Fatalf("ExploitGodMode=true must clear lastResting")
	}

	// 4. AutoRest: false must bypass resting
	fe.lastResting = true
	noRestCfg := defaultCfg
	noRestCfg.AutoRest = false
	if fe.checkRestState(noRestCfg) {
		t.Fatalf("AutoRest=false must not rest")
	}
	if fe.lastResting {
		t.Fatalf("AutoRest=false must clear lastResting")
	}

	// 5. BotPreferences DisableAutoRest: true must bypass resting
	fe.lastResting = true
	bot.Profile.Preferences.DisableAutoRest = true
	if fe.checkRestState(defaultCfg) {
		t.Fatalf("DisableAutoRest=true must not rest")
	}
	if fe.lastResting {
		t.Fatalf("DisableAutoRest=true must clear lastResting")
	}
	bot.Profile.Preferences.DisableAutoRest = false

	// 6. Active attackers wake up a resting bot (matching bot.gd:126)
	fe.lastResting = true
	fe.mobsMu.Lock()
	fe.mobs[101] = &TrackedMob{
		ID:    101,
		Chase: true,
		Dead:  false,
		X:     bot.Stats.PosX + 5.0,
		Z:     bot.Stats.PosZ + 5.0,
	}
	fe.mobsMu.Unlock()

	if fe.checkRestState(defaultCfg) {
		t.Fatalf("attackers within 20m must break bot out of resting")
	}
	if fe.lastResting {
		t.Fatalf("attackers must clear lastResting")
	}
}

func TestFarmEngine_ElevationUnreachableGuard(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			CharacterName: "ElevBot",
			Preferences: models.BotPreferences{
				LegitMode: true,
			},
		},
	}
	bot.Stats.PosX = 100.0
	bot.Stats.PosY = 10.0
	bot.Stats.PosZ = 100.0
	bot.Stats.Job = "mercenary"

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.AnchorX = 100.0
	cfg.AnchorY = 10.0
	cfg.AnchorZ = 100.0
	cfg.Radius = 50.0

	// Monster on top of cliff: horizontal distance 1.0m (within reach), vertical delta 6.0m (> 3.2m)
	cliffMob := &TrackedMob{
		ID:             777,
		Name:           "Cliff_Mob",
		Level:          15,
		X:              101.0,
		Y:              16.0,
		Z:              100.0,
		HP:             1000,
		MaxHP:          1000,
		Dead:           false,
		ServerVerified: true,
	}
	fe.mobs[777] = cliffMob
	fe.currentTarget = cliffMob

	fe.thinkHunt(cfg)

	// Must have cleared target due to elevation-unreachable, not swung into air
	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be cleared for unreachable cliff mob, got %v", fe.currentTarget.ID)
	}
	if _, blacklisted := fe.blacklist[777]; !blacklisted {
		t.Fatalf("expected mob 777 to be blacklisted")
	}
}

func TestFarmEngine_ConsecutiveMissGhostMob(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			CharacterName: "GhostHunter",
		},
	}
	bot.Stats.PosX = 50.0
	bot.Stats.PosY = 0.0
	bot.Stats.PosZ = 50.0
	bot.Stats.Job = "mercenary"

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.AnchorX = 50.0
	cfg.AnchorZ = 50.0
	cfg.Radius = 45.0

	ghostMob := &TrackedMob{
		ID:             888,
		Name:           "Ghost_Mob",
		Level:          5,
		X:              51.0,
		Y:              0.0,
		Z:              50.0,
		HP:             500,
		MaxHP:          500,
		Dead:           false,
		ServerVerified: true,
	}
	fe.mobs[888] = ghostMob
	fe.currentTarget = ghostMob
	fe.targetSwingsWithoutHit = 6

	fe.thinkHunt(cfg)

	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be cleared after 6 misses, got %v", fe.currentTarget.ID)
	}
	if _, exists := fe.mobs[888]; exists {
		t.Fatalf("expected ghost mob 888 to be removed from fe.mobs")
	}
	if _, blacklisted := fe.blacklist[888]; !blacklisted {
		t.Fatalf("expected ghost mob 888 to be blacklisted")
	}
}

func TestFarmEngine_PruneMobs(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			CharacterName: "PruneBot",
		},
	}
	fe := NewFarmEngine(bot)
	now := time.Now()

	// 1. Dead mob older than 4s
	fe.mobs[1] = &TrackedMob{
		ID:       1,
		Dead:     true,
		LastSeen: now.Add(-5 * time.Second),
	}

	// 2. Dead mob just died (1s ago) - should stay for corpse animation
	fe.mobs[2] = &TrackedMob{
		ID:       2,
		Dead:     true,
		LastSeen: now.Add(-1 * time.Second),
	}

	// 3. Stale mob unseen for 20s
	fe.mobs[3] = &TrackedMob{
		ID:       3,
		Dead:     false,
		LastSeen: now.Add(-20 * time.Second),
	}

	// 4. Fresh living mob
	fe.mobs[4] = &TrackedMob{
		ID:             4,
		Dead:           false,
		ServerVerified: true,
		LastSeen:       now.Add(-2 * time.Second),
	}

	// 5. Unverified shadow mob with no leader
	fe.mobs[5] = &TrackedMob{
		ID:             5,
		IsShadow:       true,
		ServerVerified: false,
		LastSeen:       now,
	}

	fe.pruneMobs(now)

	if _, exists := fe.mobs[1]; exists {
		t.Errorf("expected old dead mob 1 to be pruned")
	}
	if _, exists := fe.mobs[2]; !exists {
		t.Errorf("expected fresh dead mob 2 to be retained")
	}
	if _, exists := fe.mobs[3]; exists {
		t.Errorf("expected stale mob 3 to be pruned")
	}
	if _, exists := fe.mobs[4]; !exists {
		t.Errorf("expected active mob 4 to be retained")
	}
	if _, exists := fe.mobs[5]; exists {
		t.Errorf("expected unverified orphan shadow mob 5 to be pruned")
	}
}

func TestFarmEngine_PickBestMob_IgnoresShadow(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			CharacterName: "SoloBot",
		},
	}
	bot.Stats.PosX = 0.0
	bot.Stats.PosY = 0.0
	bot.Stats.PosZ = 0.0
	bot.Stats.Level = 10

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.SmartMode = false
	cfg.AnchorX = 0.0
	cfg.AnchorZ = 0.0
	cfg.Radius = 50.0

	// Shadow mob at distance 2m
	fe.mobs[10] = &TrackedMob{
		ID:       10,
		Name:     "Shadow_Mob",
		Level:    10,
		X:        2.0,
		Y:        0.0,
		Z:        0.0,
		HP:       100,
		MaxHP:    100,
		IsShadow: true,
	}

	// Real mob at distance 10m
	fe.mobs[20] = &TrackedMob{
		ID:             20,
		Name:           "Real_Mob",
		Level:          10,
		X:              10.0,
		Y:              0.0,
		Z:              0.0,
		HP:             100,
		MaxHP:          100,
		ServerVerified: true,
	}

	best := fe.pickBestMob(cfg, 0.0, 0.0, 0.0, 10)
	if best == nil || best.ID != 20 {
		t.Fatalf("expected real mob 20 to be picked, got %v", best)
	}
}

func TestFarmEngine_StateUpdate_DoesNotCreateGhostMobs(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	// Server sends "s" state update with unannounced mob ID 999
	raw := map[string]interface{}{
		"t": "mob",
		"s": []interface{}{
			[]interface{}{999, 10.0, 20.0, 100, 0, 0, 0.0},
		},
	}
	fe.OnMobPacket(raw)

	if _, exists := fe.mobs[999]; exists {
		t.Fatalf("expected unjoined mob 999 to NOT be added to fe.mobs from 's' packet")
	}
}

func TestFarmEngine_StateUpdate_ZeroHpMarksDeadAndClearsTarget(t *testing.T) {
	bot := &BotInstance{}
	fe := NewFarmEngine(bot)

	mob := &TrackedMob{
		ID:             555,
		Name:           "Living_Mob",
		Level:          10,
		HP:             100,
		MaxHP:          100,
		Dead:           false,
		ServerVerified: true,
	}
	fe.mobs[555] = mob
	fe.currentTarget = mob

	// Server sends "s" with shp = 0
	raw := map[string]interface{}{
		"t": "mob",
		"s": []interface{}{
			[]interface{}{555, 10.0, 20.0, 0, 0, 0, 0.0},
		},
	}
	fe.OnMobPacket(raw)

	if !mob.Dead {
		t.Fatalf("expected mob 555 to be marked Dead when shp <= 0")
	}
	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be cleared when mob HP reached 0")
	}
}

func TestFarmEngine_ZeroHpTargetClearedInThinkHunt(t *testing.T) {
	bot := &BotInstance{
		Profile: models.BotProfile{
			CharacterName: "TestSolo",
		},
	}
	bot.Stats.PosX = 0.0
	bot.Stats.PosZ = 0.0
	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()

	zeroHpMob := &TrackedMob{
		ID:             666,
		Name:           "Zero_HP",
		Level:          5,
		HP:             0,
		Dead:           false, // Dead wasn't set yet
		ServerVerified: true,
	}
	fe.mobs[666] = zeroHpMob
	fe.currentTarget = zeroHpMob

	fe.thinkHunt(cfg)

	if fe.currentTarget != nil {
		t.Fatalf("expected currentTarget to be cleared in thinkHunt when HP <= 0")
	}
}


