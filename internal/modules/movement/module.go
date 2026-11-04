package movement

import (
	"context"
	"encoding/json"
	"errors"

	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/bot"
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Main" }
func (m *Module) ID() string    { return "movement" }
func (m *Module) Title() string { return "Movement" }
func (m *Module) Order() int    { return 3 }
func (m *Module) Icon() string  { return "directions_run" }

func (m *Module) Register(r multi.Registrar) {
	// 1. Move Vector command
	r.Command(multi.CommandDef{
		Key:   "main.movement.move",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p struct {
				Dx float32 `json:"dx"`
				Dz float32 `json:"dz"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p struct {
				Dx float32 `json:"dx"`
				Dz float32 `json:"dz"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			inst.Move(p.Dx, p.Dz)
			return nil, nil
		},
	})

	// 2. Stop Move command
	r.Command(multi.CommandDef{
		Key:   "main.movement.stop",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(*bot.BotInstance)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			inst.StopMove()
			return nil, nil
		},
	})
}
