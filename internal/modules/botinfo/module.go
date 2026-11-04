package botinfo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/bot"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Main" }
func (m *Module) ID() string    { return "bot" }
func (m *Module) Title() string { return "Bot Information" }
func (m *Module) Order() int    { return 2 }
func (m *Module) Icon() string  { return "person" }

func (m *Module) Register(r multi.Registrar) {
	// 1. Connect command
	r.Command(multi.CommandDef{
		Key:          "main.bot.connect",
		Scope:        multi.AllSelected,
		AllowOffline: true,
		Mode:         multi.Parallel,
		Stagger:      450 * time.Millisecond,
		CanRun: func(b multi.Bot) error {
			if b == nil {
				return errors.New("bot is nil")
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return errors.New("instance not found")
			}
			if b.Online() {
				return errors.New("already connected")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return nil, inst.Start()
		},
	})

	// 2. Disconnect command
	r.Command(multi.CommandDef{
		Key:   "main.bot.disconnect",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		CanRun: func(b multi.Bot) error {
			if b == nil {
				return errors.New("bot is nil")
			}
			if !b.Online() {
				return errors.New("already disconnected")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return nil, inst.Stop()
		},
	})

	// 3. Follow target command
	r.Command(multi.CommandDef{
		Key:   "main.bot.follow",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct {
				Target string `json:"target"`
			}
			if len(payload) > 0 && string(payload) != "null" {
				if err := json.Unmarshal(payload, &p); err != nil {
					return err
				}
			}
			if p.Target != "" && strings.EqualFold(b.Name(), p.Target) {
				return errors.New("cannot follow self")
			}
			return nil
		},
		RefsBots: func(payload json.RawMessage) []multi.BotRef {
			var p struct {
				Target string `json:"target"`
			}
			if len(payload) > 0 && string(payload) != "null" {
				if err := json.Unmarshal(payload, &p); err == nil && p.Target != "" {
					return []multi.BotRef{{Name: p.Target}}
				}
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				Target string `json:"target"`
			}
			if len(payload) > 0 && string(payload) != "null" {
				_ = json.Unmarshal(payload, &p)
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			inst.SetFollowTarget(p.Target)
			return nil, nil
		},
	})

	// 4. Warp command
	r.Command(multi.CommandDef{
		Key:     "main.bot.warp",
		Scope:   multi.AllSelected,
		Mode:    multi.Parallel,
		Stagger: 100 * time.Millisecond,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct {
				Target string `json:"target"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			trimmed := strings.TrimSpace(p.Target)
			if trimmed == "" {
				return errors.New("warp target cannot be empty")
			}
			if strings.EqualFold(b.Name(), trimmed) {
				return errors.New("cannot warp to self")
			}
			return nil
		},
		RefsBots: func(payload json.RawMessage) []multi.BotRef {
			var p struct {
				Target string `json:"target"`
			}
			if err := json.Unmarshal(payload, &p); err == nil {
				trimmed := strings.TrimSpace(p.Target)
				// If not coordinates (no comma), treat as potential player name
				if trimmed != "" && !strings.Contains(trimmed, ",") {
					return []multi.BotRef{{Name: trimmed}}
				}
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				Target string `json:"target"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return nil, inst.Warp(p.Target)
		},
	})

	// 5. Advance job command
	r.Command(multi.CommandDef{
		Key:   "main.bot.advance_job",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct {
				TargetJob string `json:"targetJob"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if strings.TrimSpace(p.TargetJob) == "" {
				return errors.New("target job required")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				TargetJob string `json:"targetJob"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return inst.AdvanceJob(p.TargetJob)
		},
	})

	// 6. Update server/channel command
	r.Command(multi.CommandDef{
		Key:     "main.bot.channel",
		Scope:   multi.AllSelected,
		Mode:    multi.Parallel,
		Stagger: 400 * time.Millisecond,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct {
				Channel string `json:"channel"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			if strings.TrimSpace(p.Channel) == "" {
				return errors.New("channel required")
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				Channel string `json:"channel"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			return nil, inst.UpdateServer(p.Channel)
		},
	})
}
