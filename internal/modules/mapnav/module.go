package mapnav

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"madrigal-multibot/internal/multi"
)

// NavController abstracts waypoint navigation and warping on a bot instance.
type NavController interface {
	SetWaypoint(x, z float32)
	ClearWaypoint()
	Warp(target string) error
}

type Module struct{}

func New() *Module {
	return &Module{}
}

func (m *Module) Tab() string   { return "Map" }
func (m *Module) ID() string    { return "nav" }
func (m *Module) Title() string { return "Map & Navigation" }
func (m *Module) Order() int    { return 5 }
func (m *Module) Icon() string  { return "map" }

type CoordsPayload struct {
	X float32 `json:"x"`
	Z float32 `json:"z"`
}

func (m *Module) Register(r multi.Registrar) {
	// 1. Set Waypoint
	r.Command(multi.CommandDef{
		Key:     "map.nav.waypoint_set",
		Scope:   multi.AllSelected,
		Mode:    multi.Parallel,
		Stagger: 100 * time.Millisecond,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p CoordsPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p CoordsPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(NavController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			inst.SetWaypoint(p.X, p.Z)
			return nil, nil
		},
	})

	// 2. Clear Waypoint
	r.Command(multi.CommandDef{
		Key:   "map.nav.waypoint_clear",
		Scope: multi.AllSelected,
		Mode:  multi.Parallel,
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			inst, ok := b.Raw().(NavController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			inst.ClearWaypoint()
			return nil, nil
		},
	})

	// 3. Warp to Coords
	r.Command(multi.CommandDef{
		Key:     "map.nav.warp",
		Scope:   multi.AllSelected,
		Mode:    multi.Parallel,
		Stagger: 100 * time.Millisecond,
		Validate: func(b multi.Bot, payload json.RawMessage) error {
			var p CoordsPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
			return nil
		},
		Execute: func(ctx context.Context, b multi.Bot, payload json.RawMessage) (any, error) {
			var p CoordsPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			inst, ok := b.Raw().(NavController)
			if !ok || inst == nil {
				return nil, errors.New("instance not found")
			}
			targetStr := fmt.Sprintf("%.1f, %.1f", p.X, p.Z)
			return nil, inst.Warp(targetStr)
		},
	})
}
