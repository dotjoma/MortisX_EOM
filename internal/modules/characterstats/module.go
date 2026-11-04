package characterstats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/bot"
	"madrigal-multibot/pkg/models"
)

// StatAllocator abstracts stat allocation and summary retrieval on a bot instance.
type StatAllocator interface {
	Summary() models.BotSummary
	AllocateStats(addStr, addSta, addDex, addInt, autoStat int) (*models.BotSummary, error)
}

// Module implements the character stats multi-bot module for Main tab.
type Module struct {
	loadStatsJson func() (models.StatsJsonMap, error)
}

func New() *Module {
	return &Module{
		loadStatsJson: bot.LoadStatsJson,
	}
}

func (m *Module) Tab() string   { return "Main" }
func (m *Module) ID() string    { return "stats" }
func (m *Module) Title() string { return "Character Stats" }
func (m *Module) Order() int    { return 4 }
func (m *Module) Icon() string  { return "person" }

type AllocatePayload struct {
	AddStr   int `json:"add_str"`
	AddSta   int `json:"add_sta"`
	AddDex   int `json:"add_dex"`
	AddInt   int `json:"add_int"`
	AutoStat int `json:"auto_stat"` // -1 to keep current, 0=None, 1=STR, 2=STA, 3=DEX, 4=INT
}

type AutoStatPayload struct {
	AutoStat int `json:"auto_stat"`
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Allocate Stats command (manual allocation / apply)
	r.Command(multi.CommandDef{
		Key:          "main.stats.allocate",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p AllocatePayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if p.AddStr < 0 || p.AddSta < 0 || p.AddDex < 0 || p.AddInt < 0 {
				return errors.New("stat increments cannot be negative")
			}
			if p.AutoStat < -1 || p.AutoStat > 4 {
				return errors.New("invalid auto_stat: must be between -1 and 4")
			}
			totalAdd := p.AddStr + p.AddSta + p.AddDex + p.AddInt
			if totalAdd == 0 && p.AutoStat < 0 {
				return errors.New("no stat points or auto_stat specified")
			}

			inst, ok := b.Raw().(StatAllocator)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			summary := inst.Summary()
			if totalAdd > int(summary.Stats.StatPoints) {
				return fmt.Errorf("insufficient stat points (need %d, have %d)", totalAdd, summary.Stats.StatPoints)
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p AllocatePayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(StatAllocator)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.AllocateStats(p.AddStr, p.AddSta, p.AddDex, p.AddInt, p.AutoStat)
		},
	})

	// 2. Auto-Fill Stats command (allocates free points according to stats.json template)
	r.Command(multi.CommandDef{
		Key:          "main.stats.autofill",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		Stagger:      50 * time.Millisecond,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			inst, ok := b.Raw().(StatAllocator)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			summary := inst.Summary()
			if summary.Stats.StatPoints == 0 {
				return errors.New("no free stat points")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(StatAllocator)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			summary := inst.Summary()
			freePoints := int(summary.Stats.StatPoints)
			if freePoints <= 0 {
				return nil, errors.New("no free stat points")
			}

			botJob := strings.ToLower(summary.Stats.Job)
			if botJob == "" {
				botJob = "vagrant"
			}

			builds, err := m.loadStatsJson()
			if err != nil {
				return nil, fmt.Errorf("failed to load stats.json: %w", err)
			}
			classBuild, found := builds[botJob]
			if !found {
				return nil, fmt.Errorf("no stats.json template for job %s", botJob)
			}

			addStr, addSta, addDex, addInt, autoStat := CalculateBuildDistribution(
				classBuild,
				int(summary.Stats.STR),
				int(summary.Stats.STA),
				int(summary.Stats.DEX),
				int(summary.Stats.INT),
				freePoints,
			)

			return inst.AllocateStats(addStr, addSta, addDex, addInt, autoStat)
		},
	})

	// 3. Set Auto-Stat radio command
	r.Command(multi.CommandDef{
		Key:          "main.stats.set_autostat",
		Scope:        multi.AllSelected,
		Mode:         multi.Parallel,
		AllowOffline: true,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p AutoStatPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if p.AutoStat < 0 || p.AutoStat > 4 {
				return errors.New("invalid auto_stat: must be between 0 and 4")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p AutoStatPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(StatAllocator)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.AllocateStats(0, 0, 0, 0, p.AutoStat)
		},
	})
}

// CalculateBuildDistribution computes stat increments according to a ClassBuildConfig.
func CalculateBuildDistribution(
	build models.ClassBuildConfig,
	curStr, curSta, curDex, curInt, freePoints int,
) (addStr, addSta, addDex, addInt, autoStat int) {
	autoStat = build.AutoStat
	switch strings.ToLower(build.Mode) {
	case "cap":
		capVal := build.CapValue
		if capVal <= 0 {
			capVal = 43
		}
		capStat := strings.ToLower(build.CapStat)
		if capStat == "" {
			capStat = "dex"
		}
		thenStat := strings.ToLower(build.ThenStat)
		if thenStat == "" {
			thenStat = "str"
		}

		curVal := curDex
		switch capStat {
		case "str":
			curVal = curStr
		case "sta":
			curVal = curSta
		case "int":
			curVal = curInt
		}

		needed := capVal - curVal
		if needed < 0 {
			needed = 0
		}
		addCap := freePoints
		if addCap > needed {
			addCap = needed
		}
		rem := freePoints - addCap

		switch capStat {
		case "str":
			addStr += addCap
		case "sta":
			addSta += addCap
		case "dex":
			addDex += addCap
		case "int":
			addInt += addCap
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

	case "pure":
		stat := strings.ToLower(build.CapStat)
		switch stat {
		case "str":
			addStr = freePoints
		case "sta":
			addSta = freePoints
		case "dex":
			addDex = freePoints
		case "int":
			addInt = freePoints
		default:
			addInt = freePoints
		}

	default:
		addStr = freePoints
	}
	return
}
