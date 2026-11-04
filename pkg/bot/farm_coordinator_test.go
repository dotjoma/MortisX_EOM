package bot

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"madrigal-multibot/pkg/models"
)

func TestCoordinatorTwoBotsOneMob(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Bot A claims Mob 100 on Madrigal channel 2
	okA := fc.Claim("madrigal", 2, 100, "bot_A")
	if !okA {
		t.Fatalf("expected bot_A to successfully claim mob 100")
	}

	// Bot B attempts to claim the same Mob 100 -> must fail
	okB := fc.Claim("madrigal", 2, 100, "bot_B")
	if okB {
		t.Fatalf("expected bot_B claim to fail on already claimed mob 100")
	}

	// Verify IsClaimedByOther
	if fc.IsClaimedByOther("madrigal", 2, 100, "bot_A") {
		t.Fatalf("bot_A is the owner, should not report as claimed by other")
	}
	if !fc.IsClaimedByOther("madrigal", 2, 100, "bot_B") {
		t.Fatalf("bot_B should see mob 100 as claimed by other")
	}

	// Bot A releases mob 100
	fc.Release("madrigal", 2, 100, "bot_A", "target_cleared")

	// Now Bot B should be able to claim mob 100
	okB2 := fc.Claim("madrigal", 2, 100, "bot_B")
	if !okB2 {
		t.Fatalf("expected bot_B to claim mob 100 after bot_A released")
	}
}

func TestCoordinatorSharedClaimSelfDefense(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Bot A claims Mob 200
	fc.Claim("madrigal", 2, 200, "bot_A")

	// Mob 200 aggros onto Bot B -> Bot B self-defends via ClaimShared
	fc.ClaimShared("madrigal", 2, 200, "bot_B")

	// Neither Bot A nor Bot B should see mob 200 as claimed by an unauthorized other
	if fc.IsClaimedByOther("madrigal", 2, 200, "bot_A") {
		t.Fatalf("bot_A is co-owner, should not see mob 200 as claimed by other")
	}
	if fc.IsClaimedByOther("madrigal", 2, 200, "bot_B") {
		t.Fatalf("bot_B is co-owner, should not see mob 200 as claimed by other")
	}
	// A third bot (Bot C) must see it as claimed
	if !fc.IsClaimedByOther("madrigal", 2, 200, "bot_C") {
		t.Fatalf("bot_C should see mob 200 as claimed")
	}

	// Bot A moves away or switches target and releases
	fc.Release("madrigal", 2, 200, "bot_A", "target_switch")

	// Bot B is still an active owner!
	if fc.IsClaimedByOther("madrigal", 2, 200, "bot_B") {
		t.Fatalf("bot_B should still be active owner after bot_A released")
	}
	if !fc.IsClaimedByOther("madrigal", 2, 200, "bot_A") {
		t.Fatalf("bot_A is no longer owner, should see mob 200 as claimed by bot_B")
	}

	// Bot B finishes fighting and releases
	fc.Release("madrigal", 2, 200, "bot_B", "fight_ended")

	// Now mob 200 has no owners and is free
	if fc.IsClaimedByOther("madrigal", 2, 200, "bot_C") {
		t.Fatalf("mob 200 should be completely free after both owners released")
	}
}

func TestCoordinatorReleaseTriggers(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Setup: Bot 1 claims mobs 301, 302, 303
	fc.Claim("madrigal", 2, 301, "bot_1")
	fc.Claim("madrigal", 2, 302, "bot_1")
	fc.Claim("madrigal", 2, 303, "bot_1")

	// Trigger 1: ReleaseMobAllOwners on server event "d" (death) or "l" (despawn)
	fc.ReleaseMobAllOwners("madrigal", 2, 301, "server_d_death")
	if fc.IsClaimedByOther("madrigal", 2, 301, "bot_2") {
		t.Fatalf("mob 301 should be completely free after ReleaseMobAllOwners")
	}

	// Trigger 2: ReleaseAll when bot_1 stops or disconnects
	fc.ReleaseAll("bot_1")

	if fc.IsClaimedByOther("madrigal", 2, 302, "bot_2") {
		t.Fatalf("mob 302 should be released after ReleaseAll(bot_1)")
	}
	if fc.IsClaimedByOther("madrigal", 2, 303, "bot_2") {
		t.Fatalf("mob 303 should be released after ReleaseAll(bot_1)")
	}
}

func TestCoordinatorScopeIsolation(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Bot 1 claims mob 400 on (madrigal, 1)
	ok1 := fc.Claim("madrigal", 1, 400, "bot_ch1")
	if !ok1 {
		t.Fatalf("failed to claim on ch1")
	}

	// Bot 2 claims mob 400 on (madrigal, 2) -> DIFFERENT CHANNEL, must succeed!
	ok2 := fc.Claim("madrigal", 2, 400, "bot_ch2")
	if !ok2 {
		t.Fatalf("claims on different channels must not conflict")
	}

	// Bot 3 claims mob 400 on (azria, 1) -> DIFFERENT MAP, must succeed!
	ok3 := fc.Claim("azria", 1, 400, "bot_azria")
	if !ok3 {
		t.Fatalf("claims on different maps must not conflict")
	}

	// Verify isolation in IsClaimedByOther
	if fc.IsClaimedByOther("madrigal", 1, 400, "bot_ch1") {
		t.Fatalf("bot_ch1 is owner on (madrigal, 1)")
	}
	if !fc.IsClaimedByOther("madrigal", 1, 400, "bot_ch2") {
		t.Fatalf("bot_ch2 should see mob 400 claimed by other on (madrigal, 1)")
	}
}

func TestCoordinatorTTLExpiry(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	cc := fc.getChannel("madrigal", 2)
	cc.mu.Lock()
	// Create an artificially expired claim
	cc.claims[500] = &MobClaim{
		MobID: 500,
		Owners: map[string]time.Time{
			"ghost_bot": time.Now().Add(-5 * time.Second), // expired 5 seconds ago
		},
		CreatedAt: time.Now().Add(-25 * time.Second),
	}
	cc.mu.Unlock()

	// New bot checks mob 500 -> expired claim should not block
	if fc.IsClaimedByOther("madrigal", 2, 500, "active_bot") {
		t.Fatalf("expired claim must not block active_bot")
	}

	// Active bot claims mob 500 -> should take over
	ok := fc.Claim("madrigal", 2, 500, "active_bot")
	if !ok {
		t.Fatalf("active_bot should succeed in claiming expired mob 500")
	}
}

func TestCoordinatorConcurrencyStress(t *testing.T) {
	fc := NewFarmCoordinator(nil)
	const numBots = 20
	const numMobs = 100
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(numBots)

	for b := 0; b < numBots; b++ {
		botID := fmt.Sprintf("stress_bot_%d", b)
		go func(id string) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano()))

			for i := 0; i < iterations; i++ {
				mobID := rng.Intn(numMobs) + 1
				channel := (rng.Intn(2)) + 1 // ch 1 or 2
				mapName := "madrigal"

				action := rng.Intn(5)
				switch action {
				case 0:
					_ = fc.Claim(mapName, channel, mobID, id)
				case 1:
					fc.ClaimShared(mapName, channel, mobID, id)
				case 2:
					fc.Renew(mapName, channel, mobID, id)
				case 3:
					fc.Release(mapName, channel, mobID, id, "stress_release")
				case 4:
					_ = fc.IsClaimedByOther(mapName, channel, mobID, id)
				}
			}
			fc.ReleaseAll(id)
		}(botID)
	}

	wg.Wait()
}

func TestFarmEngineCoordinationPickTarget(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")

	botA := &BotInstance{ID: "bot_1", Profile: models.BotProfile{Server: "Channel 2"}}
	botB := &BotInstance{ID: "bot_2", Profile: models.BotProfile{Server: "Channel 2"}}

	feA := NewFarmEngine(botA)
	feB := NewFarmEngine(botB)

	cfg := models.DefaultFarmConfig()
	cfg.SmartMode = false
	cfg.AnchorX = 100.0
	cfg.AnchorZ = 100.0
	cfg.Radius = 35.0
	cfg.CoordinatedFarm = true
	feA.SetConfig(cfg)
	feB.SetConfig(cfg)

	// Monster 500: closer (102, 10, 100) -> distance 2m
	// Monster 501: farther (105, 10, 100) -> distance 5m
	mob500 := &TrackedMob{ID: 500, Name: "Aibat_A", Level: 5, HP: 100, MaxHP: 100, X: 102.0, Y: 10.0, Z: 100.0, LastSeen: time.Now()}
	mob501 := &TrackedMob{ID: 501, Name: "Aibat_B", Level: 5, HP: 100, MaxHP: 100, X: 105.0, Y: 10.0, Z: 100.0, LastSeen: time.Now()}

	feA.mobs[500] = mob500
	feA.mobs[501] = mob501
	feB.mobs[500] = mob500
	feB.mobs[501] = mob501

	// Bot A picks best mob: should be mob 500 (closer)
	pickedA := feA.pickBestMob(cfg, 100.0, 10.0, 100.0, 5)
	if pickedA == nil || pickedA.ID != 500 {
		t.Fatalf("expected Bot A to pick closer mob 500, got %v", pickedA)
	}

	// Bot A switches target and claims mob 500
	okA := feA.switchTarget(pickedA, "pick", 100.0, 100.0)
	if !okA {
		t.Fatalf("expected Bot A switchTarget to succeed")
	}

	// Bot B now picks best mob: mob 500 is claimed by Bot A, so Bot B MUST pick mob 501!
	pickedB := feB.pickBestMob(cfg, 100.0, 10.0, 100.0, 5)
	if pickedB == nil || pickedB.ID != 501 {
		t.Fatalf("expected Bot B to skip claimed mob 500 and pick mob 501, got %v", pickedB)
	}

	okB := feB.switchTarget(pickedB, "pick", 100.0, 100.0)
	if !okB {
		t.Fatalf("expected Bot B switchTarget to succeed")
	}

	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")
}

func TestFarmEngineCoordinationReleaseTriggers(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")

	botA := &BotInstance{ID: "bot_1", Profile: models.BotProfile{Server: "Channel 2"}}
	feA := NewFarmEngine(botA)

	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	feA.SetConfig(cfg)

	mob := &TrackedMob{ID: 550, Name: "Aibat_Dead", Level: 5, HP: 100, MaxHP: 100, X: 102.0, Y: 10.0, Z: 100.0, LastSeen: time.Now()}
	feA.mobs[550] = mob

	feA.switchTarget(mob, "pick", 100.0, 100.0)

	// Verify mob 550 is claimed by bot_1
	if !GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 550, "bot_2") {
		t.Fatalf("expected mob 550 to be claimed by bot_1")
	}

	// Server sends event "d" (death) in OnMobPacket
	deathPkt := map[string]interface{}{
		"t": "mob",
		"e": []interface{}{
			[]interface{}{"d", float64(550), float64(1)},
		},
	}
	feA.OnMobPacket(deathPkt)

	// Claim must now be released for all owners
	if GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 550, "bot_2") {
		t.Fatalf("expected mob 550 claim to be released after server 'd' event")
	}
}

func TestFarmEngineCoordinationDespawnRelease(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")

	botA := &BotInstance{ID: "bot_1", Profile: models.BotProfile{Server: "Channel 2"}}
	botB := &BotInstance{ID: "bot_2", Profile: models.BotProfile{Server: "Channel 2"}}
	feA := NewFarmEngine(botA)
	feB := NewFarmEngine(botB)

	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	feA.SetConfig(cfg)
	feB.SetConfig(cfg)

	mob := &TrackedMob{ID: 600, Name: "Despawn_Mob", Level: 5, HP: 100, MaxHP: 100, X: 102.0, Y: 10.0, Z: 100.0, LastSeen: time.Now()}
	feA.mobs[600] = mob
	feB.mobs[600] = mob

	feA.switchTarget(mob, "pick", 100.0, 100.0)

	// Mob 600 claimed by bot_1
	if !GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 600, "bot_2") {
		t.Fatalf("expected mob 600 to be claimed")
	}

	// Server sends despawn packet "l" observed by Bot B
	despawnPkt := map[string]interface{}{
		"t": "mob",
		"l": []interface{}{float64(600)},
	}
	feB.OnMobPacket(despawnPkt)

	// Mob 600 must be released process-wide
	if GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 600, "bot_1") {
		t.Fatalf("expected mob 600 to be released across all bots on channel despawn")
	}
}

func TestFarmEngineCoordinationAggroSwitch(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")
	GetGlobalCoordinator().ReleaseAll("bot_3")

	botA := &BotInstance{ID: "bot_1", entityID: 1, Profile: models.BotProfile{Server: "Channel 2"}}
	botB := &BotInstance{ID: "bot_2", entityID: 2, Profile: models.BotProfile{Server: "Channel 2"}}
	botC := &BotInstance{ID: "bot_3", entityID: 3, Profile: models.BotProfile{Server: "Channel 2"}}
	feA := NewFarmEngine(botA)
	feB := NewFarmEngine(botB)
	feC := NewFarmEngine(botC)

	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	feA.SetConfig(cfg)
	feB.SetConfig(cfg)
	feC.SetConfig(cfg)

	// Bot A claims mob 700 (initially targeting Bot A)
	mob := &TrackedMob{ID: 700, Name: "Aggro_Mob", Level: 5, HP: 100, MaxHP: 100, X: 102.0, Y: 10.0, Z: 100.0, Chase: true, TargetPlayerID: 1, LastSeen: time.Now()}
	feA.mobs[700] = mob
	feB.mobs[700] = mob
	feC.mobs[700] = mob

	feA.switchTarget(mob, "pick", 100.0, 100.0)

	// Mob 700 aggros onto Bot B -> TargetPlayerID updates to 2
	mob.TargetPlayerID = 2
	okB := feB.switchTarget(mob, "aggro-switch", 101.0, 100.0)
	if !okB {
		t.Fatalf("expected Bot B self-defense aggro-switch to succeed")
	}

	// Both Bot A and Bot B co-own mob 700
	if GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 700, "bot_1") {
		t.Fatalf("bot_1 is co-owner, should not see claimed by other")
	}
	if GetGlobalCoordinator().IsClaimedByOther("madrigal", 2, 700, "bot_2") {
		t.Fatalf("bot_2 is co-owner, should not see claimed by other")
	}

	// Bot C (roaming) must see it as claimed and skip it in pickBestMob
	pickedC := feC.pickBestMob(cfg, 100.0, 10.0, 100.0, 5)
	if pickedC != nil {
		t.Fatalf("expected Bot C to skip co-owned mob 700, got %v", pickedC)
	}

	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")
	GetGlobalCoordinator().ReleaseAll("bot_3")
}

func TestFarmEngineCoordinationConfigFlagDisabled(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")

	botA := &BotInstance{ID: "bot_1", Profile: models.BotProfile{Server: "Channel 2"}}
	botB := &BotInstance{ID: "bot_2", Profile: models.BotProfile{Server: "Channel 2"}}
	feA := NewFarmEngine(botA)
	feB := NewFarmEngine(botB)

	// Coordination explicitly disabled (parity mode)
	cfg := models.DefaultFarmConfig()
	cfg.SmartMode = false
	cfg.AnchorX = 100.0
	cfg.AnchorZ = 100.0
	cfg.Radius = 35.0
	cfg.CoordinatedFarm = false
	feA.SetConfig(cfg)
	feB.SetConfig(cfg)

	mob := &TrackedMob{ID: 800, Name: "Aibat_Shared", Level: 5, HP: 100, MaxHP: 100, X: 102.0, Y: 10.0, Z: 100.0, LastSeen: time.Now()}
	feA.mobs[800] = mob
	feB.mobs[800] = mob

	feA.switchTarget(mob, "pick", 100.0, 100.0)

	// When CoordinatedFarm is false, Bot B will still pick mob 800
	pickedB := feB.pickBestMob(cfg, 100.0, 10.0, 100.0, 5)
	if pickedB == nil || pickedB.ID != 800 {
		t.Fatalf("with CoordinatedFarm=false, expected Bot B to pick mob 800, got %v", pickedB)
	}

	GetGlobalCoordinator().ReleaseAll("bot_1")
	GetGlobalCoordinator().ReleaseAll("bot_2")
}

func TestCoordinatorZoneAllocationSeparation(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Both Bot 1 and Bot 2 start at Flaris (x=6800, z=-3600) at Level 5
	sp1, ok1 := fc.AllocateZone("madrigal", 2, "bot_1", 5, 6800.0, -3600.0, 1, 14, false)
	if !ok1 || sp1 == nil {
		t.Fatalf("expected bot_1 to be allocated a spawn")
	}

	sp2, ok2 := fc.AllocateZone("madrigal", 2, "bot_2", 5, 6800.0, -3600.0, 1, 14, false)
	if !ok2 || sp2 == nil {
		t.Fatalf("expected bot_2 to be allocated a spawn")
	}

	// Anchors must be separated by at least MinAnchorSeparation (30.0m)
	dx := sp1.CX - sp2.CX
	dz := sp1.CZ - sp2.CZ
	dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))

	if dist < MinAnchorSeparation {
		t.Fatalf("expected anchor separation >= %.1fm, but got %.1fm (sp1=(%.1f, %.1f), sp2=(%.1f, %.1f))",
			MinAnchorSeparation, dist, sp1.CX, sp1.CZ, sp2.CX, sp2.CZ)
	}

	// Verify assignments tracked in coordinator
	asgn1 := fc.GetAssignedZone("madrigal", 2, "bot_1")
	asgn2 := fc.GetAssignedZone("madrigal", 2, "bot_2")
	if asgn1 == nil || asgn2 == nil {
		t.Fatalf("expected active assignments for both bots")
	}
}

func TestCoordinatorZoneAllocationHysteresis(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Bot 1 allocates a zone
	sp1, ok1 := fc.AllocateZone("madrigal", 2, "bot_1", 5, 6800.0, -3600.0, 1, 14, false)
	if !ok1 || sp1 == nil {
		t.Fatalf("expected initial zone allocation")
	}

	// Immediate re-check without forceRebalance: must retain identical spawn
	sp2, changed := fc.AllocateZone("madrigal", 2, "bot_1", 5, 6800.0, -3600.0, 1, 14, false)
	if changed {
		t.Fatalf("expected no change under hysteresis dwell time")
	}
	if sp1.CX != sp2.CX || sp1.CZ != sp2.CZ {
		t.Fatalf("expected identical spawn under hysteresis")
	}
}

func TestCoordinatorZoneLevelBracketRebalance(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	// Bot 1 starts at Level 2 (Aibats/Mushpangs)
	spLow, _ := fc.AllocateZone("madrigal", 2, "bot_1", 2, 6800.0, -3600.0, 1, 14, false)
	if spLow == nil {
		t.Fatalf("expected initial low level zone allocation")
	}

	// Bot 1 levels up to 25 -> must bypass dwell time because mob is now out of level bracket (+1..+14)
	spHigh, changed := fc.AllocateZone("madrigal", 2, "bot_1", 25, 6800.0, -3600.0, 1, 14, false)
	if !changed || spHigh == nil {
		t.Fatalf("expected level bracket bypass to allocate a higher level zone")
	}

	metaHigh := farmMonsters[spHigh.MobID]
	if metaHigh.Level < 26 {
		t.Fatalf("expected monster level >= 26 for bot lv 25, got lv %d (%s)", metaHigh.Level, metaHigh.Name)
	}
}

func TestCoordinatorZoneReleaseOnStop(t *testing.T) {
	fc := NewFarmCoordinator(nil)

	sp1, _ := fc.AllocateZone("madrigal", 2, "bot_1", 5, 6800.0, -3600.0, 1, 14, false)
	if sp1 == nil {
		t.Fatalf("expected zone allocation")
	}

	// Release all for bot_1
	fc.ReleaseAll("bot_1")

	// Zone assignment should now be deleted
	if asgn := fc.GetAssignedZone("madrigal", 2, "bot_1"); asgn != nil {
		t.Fatalf("expected zone assignment to be cleared after ReleaseAll")
	}
}

func TestCoordinatorLiveSpawnConfirmation(t *testing.T) {
	fc := NewFarmCoordinator(nil)
	initFarmDatabase()
	if len(farmSpawns) == 0 {
		t.Fatalf("expected farmSpawns to be loaded")
	}
	firstSp := farmSpawns[0]

	// Live sighting of a monster near the first known spawn
	fc.ConfirmLiveSpawn("madrigal", 2, firstSp.MobID, firstSp.CX, firstSp.CZ)

	cc := fc.getChannel("madrigal", 2)
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if len(cc.confirmedSpawns) == 0 {
		t.Fatalf("expected live confirmed spawn to be recorded")
	}
}

func TestFarmEngineWatchdogTriggersOnZeroProgress(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_wd")
	bot := &BotInstance{ID: "bot_wd", Profile: models.BotProfile{Server: "Channel 2"}}
	bot.Stats.PosX = 6800.0
	bot.Stats.PosZ = -3600.0
	bot.Stats.Level = 5

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	cfg.WatchdogSec = 1
	cfg.SmartMode = true
	fe.SetConfig(cfg)

	mob := &TrackedMob{ID: 901, Name: "Aibat_Stall", Level: 5, HP: 100, MaxHP: 100, X: 6802.0, Y: 10.0, Z: -3600.0, LastSeen: time.Now()}
	fe.mobs[901] = mob
	fe.switchTarget(mob, "pick", 6800.0, -3600.0)

	// Simulate 2 seconds passed with zero HP progress (threshold = 1s)
	fe.mu.Lock()
	fe.lastHpProgressTime = time.Now().Add(-2 * time.Second)
	fe.mu.Unlock()

	fe.thinkHunt(cfg)

	// Watchdog must clear stalled target and blacklist it
	fe.mu.RLock()
	cur := fe.currentTarget
	fe.mu.RUnlock()

	if cur != nil {
		t.Fatalf("expected watchdog to clear target, got %v", cur)
	}

	fe.mobsMu.RLock()
	_, blacklisted := fe.blacklist[901]
	fe.mobsMu.RUnlock()

	if !blacklisted {
		t.Fatalf("expected mob 901 to be blacklisted after watchdog stall")
	}

	GetGlobalCoordinator().ReleaseAll("bot_wd")
}

func TestFarmEngineWatchdogDoesNotTriggerWhenDamageProgressing(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_dmg")
	bot := &BotInstance{ID: "bot_dmg", entityID: 55, Profile: models.BotProfile{Server: "Channel 2"}}
	bot.Stats.PosX = 6800.0
	bot.Stats.PosZ = -3600.0
	bot.Stats.Level = 5

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	cfg.WatchdogSec = 2
	fe.SetConfig(cfg)

	mob := &TrackedMob{ID: 902, Name: "Aibat_Dmg", Level: 5, HP: 100, MaxHP: 100, X: 6802.0, Y: 10.0, Z: -3600.0, LastSeen: time.Now()}
	fe.mobs[902] = mob
	fe.switchTarget(mob, "pick", 6800.0, -3600.0)

	// Bot hits mob: packet "h" arrives from server
	fe.OnMobPacket(map[string]interface{}{
		"e": []interface{}{
			[]interface{}{"h", 902, 55, 25, 0},
		},
	})

	// Run thinkHunt within threshold
	fe.thinkHunt(cfg)

	fe.mu.RLock()
	cur := fe.currentTarget
	fe.mu.RUnlock()

	if cur == nil || cur.ID != 902 {
		t.Fatalf("expected target 902 to remain engaged, got %v", cur)
	}

	GetGlobalCoordinator().ReleaseAll("bot_dmg")
}

func TestFarmEngineCoordinationStatsExposure(t *testing.T) {
	GetGlobalCoordinator().ReleaseAll("bot_stat")
	bot := &BotInstance{ID: "bot_stat", Profile: models.BotProfile{Server: "Channel 2"}}
	bot.Stats.PosX = 6800.0
	bot.Stats.PosZ = -3600.0
	bot.Stats.Level = 5

	fe := NewFarmEngine(bot)
	cfg := models.DefaultFarmConfig()
	cfg.CoordinatedFarm = true
	fe.SetConfig(cfg)

	coord := GetGlobalCoordinator()
	coord.AllocateZone("madrigal", 2, "bot_stat", 5, 6800.0, -3600.0, 1, 14, false)
	coord.Claim("madrigal", 2, 903, "bot_stat")

	fe.updateStats()
	stats := fe.GetStats()

	if stats.CoordClaimedMob != 903 {
		t.Fatalf("expected CoordClaimedMob=903, got %d", stats.CoordClaimedMob)
	}
	if stats.CoordZone == "" {
		t.Fatalf("expected CoordZone to be populated, got empty string")
	}
	if stats.CoordZoneMobs == "" {
		t.Fatalf("expected CoordZoneMobs to be populated, got empty string")
	}
	if stats.CoordAssignedAt == "" {
		t.Fatalf("expected CoordAssignedAt to be populated, got empty string")
	}

	coord.ReleaseAll("bot_stat")
}
