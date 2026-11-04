package bot

import (
	"testing"
)

func TestMonsterAttack_Vectors(t *testing.T) {
	defaultPlayer := PlayerCombatInput{
		Level:    1,
		STA:      15,
		DEX:      15,
		Job:      "vagrant",
		ArmorMin: 8,
		ArmorMax: 10,
	}

	// Vector 1: Small Aibatt (MI_AIBATT1, Lv 1) -> 14 dmg
	t.Run("Vector 1: Small Aibatt Normal Hit", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 1,
			HR:    40,
			DEX:   15,
			AtkM:  []int{15, 16},
			Atk1:  []int{1, 1},
			Rank:  "RANK_LOW",
		}
		// RNG Order:
		// 1. RandMod(100): Hit roll = 50 (rate 98 -> hit)
		// 2. RandMod(100): Crit roll = 50 (dex/10 = 1 -> no crit)
		// 3. RandRange(16, 17): Atk roll = 16
		// 4. RandMod(2): Def roll = 0 (by_item = 8 -> def = 2)
		// 5. RandMod(80): Block roll = 50 (no block)
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 50, 0, 50},
			RangeRolls: []int{16},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if res.Missed {
			t.Fatalf("expected attack to hit")
		}
		if res.Blocked {
			t.Fatalf("expected attack not to be blocked")
		}
		if res.Damage != 14 {
			t.Errorf("Vector 1 damage mismatch: got %d, want 14", res.Damage)
		}
	})

	// Vector A1: Captain Burudeng (MI_BURUDENG3, Lv 6) Non-Crit -> 35 dmg
	t.Run("Vector A1: Captain Burudeng Non-Crit", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 6,
			HR:    40,
			DEX:   25,
			AtkM:  []int{28, 30},
			Atk1:  []int{1, 1},
			Rank:  "RANK_CAPTAIN",
		}
		// Atk roll = 30. +5% rule: 30 * (1 + 0.05*5) = 37. Def = 2 -> 35 dmg
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 50, 0, 50},
			RangeRolls: []int{30},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if res.Missed || res.Blocked {
			t.Fatalf("unexpected miss or block: %+v", res)
		}
		if res.Damage != 35 {
			t.Errorf("Vector A1 damage mismatch: got %d, want 35", res.Damage)
		}
	})

	// Vector A2: Captain Burudeng (MI_BURUDENG3, Lv 6) Crit -> 54 dmg
	t.Run("Vector A2: Captain Burudeng Crit", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 6,
			HR:    40,
			DEX:   25,
			AtkM:  []int{28, 30},
			Atk1:  []int{1, 1},
			Rank:  "RANK_CAPTAIN",
		}
		// Crit roll = 1 (< 2 -> crit). Scaled range: mn = int(29*1.4)=40, mx = int(31*1.8)=55.
		// Atk roll = 45. +5% rule: 45 * 1.25 = 56. Def = 2 -> 54 dmg
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 1, 0, 50},
			RangeRolls: []int{45},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if res.Missed || res.Blocked {
			t.Fatalf("unexpected miss or block: %+v", res)
		}
		if res.Damage != 54 {
			t.Errorf("Vector A2 damage mismatch: got %d, want 54", res.Damage)
		}
	})

	// Vector B1: 1st Class Fefern (MI_FEFERN1, Lv 15) Non-Crit -> 181 dmg
	t.Run("Vector B1: 1st Class Fefern Non-Crit", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 15,
			HR:    27,
			DEX:   43,
			AtkM:  []int{104, 110},
			Atk1:  []int{1, 1},
			Rank:  "RANK_LOW",
		}
		// Atk roll = 108. +5% rule: 108 * (1 + 0.05*14) = 183. Def = 2 -> 181 dmg
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 50, 0, 50},
			RangeRolls: []int{108},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if res.Missed || res.Blocked {
			t.Fatalf("unexpected miss or block: %+v", res)
		}
		if res.Damage != 181 {
			t.Errorf("Vector B1 damage mismatch: got %d, want 181", res.Damage)
		}
	})

	// Vector B2: 1st Class Fefern (MI_FEFERN1, Lv 15) Crit -> 287 dmg
	t.Run("Vector B2: 1st Class Fefern Crit", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 15,
			HR:    27,
			DEX:   43,
			AtkM:  []int{104, 110},
			Atk1:  []int{1, 1},
			Rank:  "RANK_LOW",
		}
		// Crit roll = 1 (< 4 -> crit). Scaled range: mn = int(105*1.4)=147, mx = int(111*1.8)=199.
		// Atk roll = 170. +5% rule: 170 * 1.70 = 289. Def = 2 -> 287 dmg
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 1, 0, 50},
			RangeRolls: []int{170},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if res.Missed || res.Blocked {
			t.Fatalf("unexpected miss or block: %+v", res)
		}
		if res.Damage != 287 {
			t.Errorf("Vector B2 damage mismatch: got %d, want 287", res.Damage)
		}
	})

	// Block Tests: Crit Block (r >= 75)
	t.Run("Block: Crit Block 90% reduction", func(t *testing.T) {
		mon := MonsterCombatInput{
			Level: 6,
			HR:    40,
			DEX:   25,
			AtkM:  []int{28, 30},
			Atk1:  []int{1, 1},
			Rank:  "RANK_CAPTAIN",
		}
		// Block roll = 76 (>= 75 -> crit block, mult = 0.1).
		// Raw dmg = 35. 35 * 0.1 = 3. Floor check = max(3, int(37*0.1)=3) = 3.
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 50, 0, 76},
			RangeRolls: []int{30},
		}

		res := MonsterAttack(defaultPlayer, mon, 1, false, rng)
		if !res.Blocked {
			t.Fatalf("expected attack to be blocked")
		}
		if res.Damage != 3 {
			t.Errorf("crit block damage mismatch: got %d, want 3", res.Damage)
		}
	})

	// Floor Test: 10% atk minimum floor
	t.Run("Floor: High defense capped by 10% atk", func(t *testing.T) {
		tankPlayer := defaultPlayer
		tankPlayer.BuffAdjDef = 500 // enormous defense

		mon := MonsterCombatInput{
			Level: 6,
			HR:    40,
			DEX:   25,
			AtkM:  []int{28, 30},
			Atk1:  []int{1, 1},
			Rank:  "RANK_CAPTAIN",
		}
		// Atk = 37. Def = 502. Raw dmg = 0.
		// Min floor = int(37 * 0.1) = 3.
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{50, 50, 0, 50},
			RangeRolls: []int{30},
		}

		res := MonsterAttack(tankPlayer, mon, 1, false, rng)
		if res.Damage != 3 {
			t.Errorf("min floor damage mismatch: got %d, want 3", res.Damage)
		}
	})

	// DELTA_FACTOR Tests: defender_lv > attacker_lv
	t.Run("DELTA_FACTOR: Level difference reduction", func(t *testing.T) {
		highLvPlayer := defaultPlayer
		highLvPlayer.Level = 6 // d = 6 - 1 = 5 -> DELTA_FACTOR[5] = 0.882

		mon := MonsterCombatInput{
			Level: 1,
			HR:    40,
			DEX:   15,
			AtkM:  []int{15, 16},
			Atk1:  []int{1, 1},
			Rank:  "RANK_LOW",
		}
		// Rate for ml=1, lv=6: int((60/47) * 2 * (0.5/2.8) * 100) = 45.
		// Hit roll = 10 (< 45 -> hit).
		// Atk = 16. Def for Lv 6 STA 15 Vagrant = 5. Raw dmg = 11.
		// DELTA_FACTOR[5] = 0.882 -> int(11 * 0.882) = 9.
		rng := &ScriptedCombatRNG{
			ModRolls:   []int{10, 50, 0, 50},
			RangeRolls: []int{16},
		}

		res := MonsterAttack(highLvPlayer, mon, 1, false, rng)
		factor := CalcLevelFactor(6, 1)
		if factor != 0.882 {
			t.Errorf("level factor for d=5 mismatch: got %f, want 0.882", factor)
		}
		if res.Damage != 9 {
			t.Errorf("delta factor damage mismatch: got %d, want 9", res.Damage)
		}
	})
}

func TestCalcMaxHPForJob_GeneratedTable(t *testing.T) {
	// Reference test vectors against formula.gd:216-224:
	// Lv 1, STA 15: Vagrant = 230, Mercenary = 230, Assist = 230
	// Lv 5, STA 15: Vagrant = 234, Mercenary = 237, Assist = 236
	// Lv 10, STA 15: Vagrant = 246, Mercenary = 256, Assist = 255
	// Lv 15, STA 15: Vagrant = 265, Mercenary = 288, Assist = 284
	// Lv 15, STA 50: Vagrant = 634, Mercenary = 670, Assist = 664

	tests := []struct {
		job   string
		level int
		sta   int
		want  int
	}{
		{"vagrant", 1, 15, 230},
		{"mercenary", 1, 15, 230},
		{"assist", 1, 15, 230},

		{"vagrant", 5, 15, 234},
		{"mercenary", 5, 15, 237},
		{"assist", 5, 15, 236},

		{"vagrant", 10, 15, 246},
		{"mercenary", 10, 15, 256},
		{"assist", 10, 15, 255},

		{"vagrant", 15, 15, 265},
		{"mercenary", 15, 15, 288},
		{"assist", 15, 15, 284},

		{"vagrant", 15, 50, 634},
		{"mercenary", 15, 50, 670},
		{"assist", 15, 50, 664},
	}

	for _, tc := range tests {
		got := CalcMaxHPForJob(tc.job, tc.level, tc.sta, 0, 0)
		if got != tc.want {
			t.Errorf("CalcMaxHPForJob(%s, lv %d, sta %d) mismatch: got %d, want %d", tc.job, tc.level, tc.sta, got, tc.want)
		}
	}
}
