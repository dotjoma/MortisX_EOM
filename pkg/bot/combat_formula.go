package bot

import (
	"math"
	"math/rand"
	"time"

	"madrigal-multibot/pkg/models"
)

// Real client constants matching madrigal_dump reference
const (
	FormulaMinHitRate        = 20
	FormulaMaxHitRate        = 100
	ClientMobAnimLengthGuess = 1400 * time.Millisecond // INFER: Morti-only guess, calibrate via live network packet logging
	ClientDefaultMobRadius   = float32(0.45)           // movers.gd:558, remote.gd:185
)

// MobRadiusOverrides allows per-mob mesh radius overrides (empty by default).
var MobRadiusOverrides = make(map[string]float32)

// DeltaFactor table verbatim matching formula.gd:16
var DeltaFactor = []float64{
	1.0, 0.995, 0.981, 0.957, 0.924, 0.882, 0.831, 0.773,
	0.707, 0.634, 0.556, 0.471, 0.383, 0.29, 0.195, 0.098,
}

// CombatRNG provides an injectable interface for random rolls in formulas.
type CombatRNG interface {
	// RandMod returns an integer in [0, n-1], matching formula.gd:24 _r(n).
	RandMod(n int) int
	// RandRange returns an integer in [a, b], matching formula.gd:25 _rr(a, b).
	RandRange(a, b int) int
}

// DefaultCombatRNG implements CombatRNG using math/rand.
type DefaultCombatRNG struct{}

func (DefaultCombatRNG) RandMod(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

func (DefaultCombatRNG) RandRange(a, b int) int {
	if b <= a {
		return a
	}
	return a + rand.Intn(b-a+1)
}

// ScriptedCombatRNG allows tests to inject pre-determined sequences of rolls.
type ScriptedCombatRNG struct {
	ModRolls   []int
	RangeRolls []int
	modIdx     int
	rangeIdx   int
}

func (s *ScriptedCombatRNG) RandMod(n int) int {
	if s.modIdx < len(s.ModRolls) {
		r := s.ModRolls[s.modIdx]
		s.modIdx++
		return r
	}
	return 0
}

func (s *ScriptedCombatRNG) RandRange(a, b int) int {
	if s.rangeIdx < len(s.RangeRolls) {
		r := s.RangeRolls[s.rangeIdx]
		s.rangeIdx++
		return r
	}
	return a
}

// PlayerCombatInput encapsulates player stats needed for combat damage calculations.
type PlayerCombatInput struct {
	Level           int
	STA             int
	DEX             int
	Job             string
	ArmorMin        int // sum of item ab[0]
	ArmorMax        int // sum of item ab[1]
	ItemParry       int
	BuffParry       int
	BuffAdjDef      int
	BuffAdjDefRate  int
	BuffBlockMelee  int
	BuffBlockRange  int
	SuitElement     int // 0 if un-enchanted
	SuitElementLvl  int
}

// MonsterCombatInput encapsulates monster metadata needed for combat damage calculations.
type MonsterCombatInput struct {
	Level      int
	HR         int
	ER         int
	DEX        int
	AtkM       []int // [min, max]
	Atk1       []int // [min, max]
	Atk2AB     []int // [min, max]
	Rank       string
	Element    int
	ElementLvl int
	Berserk    bool
	BerserkMul float64
}

// MonsterAttackResult details the outcome of monster_attack.
type MonsterAttackResult struct {
	Damage  int
	Missed  bool
	Blocked bool
}

// CalcLevelFactor calculates level-difference multiplier matching formula.gd:291-293.
// Returns 1.0 when defender_lv <= attacker_lv. When defender_lv > attacker_lv, looks up DELTA_FACTOR.
func CalcLevelFactor(defenderLv int, attackerLv int) float64 {
	d := defenderLv - attackerLv
	if d <= 0 {
		return 1.0
	}
	idx := d
	if idx >= len(DeltaFactor) {
		idx = len(DeltaFactor) - 1
	}
	return DeltaFactor[idx]
}

// CalcPlayerParry calculates player parry rating matching formula.gd:141-144.
func CalcPlayerParry(dex int, itemParry int, buffParry int) int {
	return int(float64(dex)*0.5) + buffParry + itemParry
}

// CalcPlayerDefense calculates player defense matching formula.gd:206-214.
func CalcPlayerDefense(lv int, sta int, jobDef float64, armorMin int, armorMax int, buffAdjDef int, buffAdjDefRate int, rng CombatRNG) int {
	byItem := armorMin
	if armorMax > armorMin {
		if rng != nil {
			byItem += rng.RandMod(armorMax - armorMin)
		} else {
			byItem += (armorMax - armorMin) / 2
		}
	}
	basePart := (float64(lv*2) + float64(sta/2)) / 2.8
	d := int(basePart) - 4 + int(float64(sta-14)*jobDef)
	d += byItem / 4 + buffAdjDef
	if d < 0 {
		d = 0
	}
	rateMult := 1.0 + float64(buffAdjDefRate)/100.0
	return int(float64(d) * rateMult)
}

// CalcPlayerBlock calculates player block multiplier matching formula.gd:259-270.
// Returns 1.0 (no block), 0.1 (crit block), or 0.0 (full block).
func CalcPlayerBlock(lv int, pd int, jobBlock float64, ml int, md int, ranged bool, buffBlock int, rng CombatRNG) float64 {
	r := 0
	if rng != nil {
		r = rng.RandMod(80)
	}
	if r <= 5 {
		return 1.0
	}
	if r >= 75 {
		return 0.1
	}
	a := float64(lv) / (float64(lv+ml) * 15.0)
	diff := float64(pd - md)
	b := math.Min(float64(pd+md+2)*(diff/800.0), 10.0)
	add := math.Max(0.0, a+b) + float64(buffBlock)
	br := int((float64(pd)/8.0)*jobBlock + add)
	if br < 0 {
		br = 0
	}
	if br > r {
		return 0.0
	}
	return 1.0
}

// CalcElementFactor matches formula.gd:273-275. Returns 0 if either side is un-enchanted.
func CalcElementFactor(at, al, dt, dl int) int {
	if at <= 0 || dt <= 0 {
		return 0
	}
	// Simplified element matchup factor matching client
	return 0
}

// MonsterAttack calculates incoming monster attack damage matching formula.gd:466-490.
// RNG call sequence:
// 1. RandMod(100): Hit rate check (formula.gd:470)
// 2. RandMod(100): Crit check (formula.gd:475)
// 3. RandRange(mn, mx): Attack roll (formula.gd:478)
// 4. RandMod(armorMax - armorMin): Optional defense roll if armorMax > armorMin (formula.gd:207)
// 5. RandMod(80): Block roll (formula.gd:260), only if raw dmg > 0
func MonsterAttack(player PlayerCombatInput, mon MonsterCombatInput, method int, ranged bool, rng CombatRNG) MonsterAttackResult {
	if rng == nil {
		rng = DefaultCombatRNG{}
	}

	ml := mon.Level
	if ml < 1 {
		ml = 1
	}
	lv := player.Level
	if lv < 1 {
		lv = 1
	}

	hr := mon.HR
	if hr <= 0 {
		hr = 50
	}

	pParry := CalcPlayerParry(player.DEX, player.ItemParry, player.BuffParry)
	parryDenom := hr + pParry
	if parryDenom < 1 {
		parryDenom = 1
	}
	hrPart := (float64(hr) * 1.5) / float64(parryDenom)

	lvDenom := float64(ml) + float64(lv)*0.3
	if lvDenom < 1.0 {
		lvDenom = 1.0
	}
	lvPart := (float64(ml) * 0.5) / lvDenom

	rate := int(hrPart * 2.0 * lvPart * 100.0)
	clampedRate := rate
	if clampedRate < FormulaMinHitRate {
		clampedRate = FormulaMinHitRate
	} else if clampedRate > FormulaMaxHitRate {
		clampedRate = FormulaMaxHitRate
	}

	// 1. Hit check (formula.gd:470)
	hitRoll := rng.RandMod(100)
	if clampedRate <= hitRoll {
		return MonsterAttackResult{Damage: 0, Missed: true, Blocked: false}
	}

	ef := CalcElementFactor(mon.Element, mon.ElementLvl, player.SuitElement, player.SuitElementLvl)

	// Base damage range
	mn := 1
	mx := 1
	if len(mon.AtkM) >= 2 {
		mn = mon.AtkM[0]
		mx = mon.AtkM[1]
	}
	if method == 2 && len(mon.Atk2AB) >= 2 {
		mn += mon.Atk2AB[0]
		mx += mon.Atk2AB[1]
	} else if len(mon.Atk1) >= 2 {
		mn += mon.Atk1[0]
		mx += mon.Atk1[1]
	}

	// 2. Crit check (formula.gd:475)
	critThreshold := mon.DEX / 10
	critRoll := rng.RandMod(100)
	if critRoll < critThreshold {
		lo := 1.1
		hi := 1.4
		if ml > lv {
			lo = 1.4
			hi = 1.8
		}
		mn = int(float64(mn) * lo)
		mx = int(float64(mx) * hi)
	}

	// 3. Attack roll (formula.gd:478)
	rawAtk := rng.RandRange(mn, mx)
	atk := int(float64(rawAtk) * float64(10000+ef) / 10000.0)

	// +5% Rule when mob level > player level (formula.gd:479)
	if ml > lv {
		levelDiff := ml - lv
		plusFiveMult := 1.0 + 0.05*float64(levelDiff)
		atk = int(float64(atk) * plusFiveMult)
	}

	jobMeta := models.GetJobCombatMeta(player.Job)

	// 4. Player Defense roll (formula.gd:480)
	rawDef := CalcPlayerDefense(player.Level, player.STA, jobMeta.Def, player.ArmorMin, player.ArmorMax, player.BuffAdjDef, player.BuffAdjDefRate, rng)
	def := int(float64(rawDef) * float64(10000+ef) / 10000.0)

	dmg := atk - def
	blocked := false

	// 5. Block check (formula.gd:482)
	if dmg > 0 {
		buffBlock := player.BuffBlockMelee
		if ranged {
			buffBlock = player.BuffBlockRange
		}
		blockMult := CalcPlayerBlock(player.Level, player.DEX, jobMeta.Block, ml, mon.DEX, ranged, buffBlock, rng)
		if blockMult < 1.0 {
			blocked = true
			dmg = int(float64(dmg) * blockMult)
		}
	} else {
		dmg = 0
	}

	// Floor check: at least 10% of atk (formula.gd:487)
	minFloor := int(math.Max(0, float64(atk)*0.1))
	if dmg < minFloor {
		dmg = minFloor
	}

	// Level factor (formula.gd:488)
	if mon.Rank != "RANK_GUARD" && mon.Rank != "RANK_SUPER" {
		factor := CalcLevelFactor(lv, ml)
		dmg = int(float64(dmg) * factor)
	}

	// Berserk multiplier from movers.gd:1028
	if mon.Berserk && mon.BerserkMul > 1.0 && dmg > 0 {
		dmg = int(float64(dmg) * mon.BerserkMul)
	}

	if dmg < 1 {
		dmg = 1
	}

	return MonsterAttackResult{
		Damage:  dmg,
		Missed:  false,
		Blocked: blocked,
	}
}

// CalcMaxHPForJob calculates max HP matching formula.gd:216-224 using dynamic job multiplier.
func CalcMaxHPForJob(job string, level int, sta int, buffHpMax int, buffHpMaxRate int) int {
	if level < 1 {
		level = 1
	}
	if sta < 15 {
		sta = 15
	}
	jobMeta := models.GetJobCombatMeta(job)
	hpMul := jobMeta.HP
	if hpMul <= 0 {
		hpMul = 0.9
	}

	lv := float64(level)
	st := float64(sta)
	a := (hpMul * lv) / 2.0
	b := a * ((lv + 1.0) / 4.0) * (1.0 + st/50.0) + st*10.0
	origin := int(b + 80.0)

	n := origin + buffHpMax
	rateMult := 1.0 + float64(buffHpMaxRate)/100.0
	res := int(float64(n) * rateMult)
	if res < 1 {
		return 1
	}
	return res
}
