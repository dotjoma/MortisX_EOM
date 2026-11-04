package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"madrigal-multibot/internal/modules/botinfo"
	"madrigal-multibot/internal/modules/characterstats"
	"madrigal-multibot/internal/modules/console"
	"madrigal-multibot/internal/modules/farm"
	"madrigal-multibot/internal/modules/inventory"
	"madrigal-multibot/internal/modules/mapnav"
	"madrigal-multibot/internal/modules/movement"
	"madrigal-multibot/internal/modules/preferences"
	"madrigal-multibot/internal/modules/store"
	"madrigal-multibot/internal/modules/transfer"
	"madrigal-multibot/internal/multi"
	"madrigal-multibot/pkg/bot"
)

// botAdapter wraps a bot ID and BotManager reference.
// Direct accessors avoid heavy Summary() copying and slice allocations.
type botAdapter struct {
	bm   *bot.BotManager
	id   string
	inst *bot.BotInstance
}

func (b *botAdapter) ID() string {
	return b.id
}

func (b *botAdapter) Name() string {
	if b.inst != nil {
		return b.inst.GetName()
	}
	if b.bm == nil {
		return ""
	}
	inst, err := b.bm.GetInstance(b.id)
	if err != nil || inst == nil {
		return ""
	}
	return inst.GetName()
}

func (b *botAdapter) Online() bool {
	if b.inst != nil {
		return b.inst.IsOnline()
	}
	if b.bm == nil {
		return false
	}
	inst, err := b.bm.GetInstance(b.id)
	if err != nil || inst == nil {
		return false
	}
	return inst.IsOnline()
}

func (b *botAdapter) Raw() any {
	if b.inst != nil {
		return b.inst
	}
	if b.bm == nil {
		return nil
	}
	inst, err := b.bm.GetInstance(b.id)
	if err != nil {
		return nil
	}
	return inst
}

type botManagerAdapter struct {
	bm *bot.BotManager
}

func (bma *botManagerAdapter) Get(id string) (multi.Bot, bool) {
	if bma.bm == nil {
		return nil, false
	}
	inst, err := bma.bm.GetInstance(id)
	if err != nil || inst == nil {
		return nil, false
	}
	return &botAdapter{bm: bma.bm, id: id, inst: inst}, true
}

func (bma *botManagerAdapter) ByName(name string) (multi.Bot, bool) {
	if bma.bm == nil {
		return nil, false
	}
	inst := bma.bm.FindInstanceByName(name)
	if inst == nil {
		return nil, false
	}
	return &botAdapter{bm: bma.bm, id: inst.ID, inst: inst}, true
}

func (bma *botManagerAdapter) All() []multi.Bot {
	if bma.bm == nil {
		return nil
	}
	instances := bma.bm.GetAllInstances()
	res := make([]multi.Bot, len(instances))
	for i, inst := range instances {
		res[i] = &botAdapter{bm: bma.bm, id: inst.ID, inst: inst}
	}
	return res
}

type wailsEmitter struct {
	app *App
}

func (we *wailsEmitter) Emit(eventName string, data any) {
	if we.app != nil && we.app.ctx != nil {
		runtime.EventsEmit(we.app.ctx, eventName, data)
	}
}

// newMultiDispatcher creates an instance of the Multi Dispatcher bound to this App instance.
func (a *App) newMultiDispatcher() *multi.Dispatcher {
	d := multi.NewDispatcher(multi.Deps{
		Bots:    &botManagerAdapter{bm: a.botManager},
		Emitter: &wailsEmitter{app: a},
		Persister: multi.NewDebouncedPersister(500*time.Millisecond, func(b multi.Bot) error {
			_, err := a.SaveBotsFile()
			return err
		}, multi.RealClock{}),
		Workers: 64,
		Clock:   multi.RealClock{},
	})

	if err := d.RegisterModule(preferences.New()); err != nil {
		panic(fmt.Sprintf("failed to register preferences module: %v", err))
	}
	if err := d.RegisterModule(botinfo.New()); err != nil {
		panic(fmt.Sprintf("failed to register botinfo module: %v", err))
	}
	if err := d.RegisterModule(movement.New()); err != nil {
		panic(fmt.Sprintf("failed to register movement module: %v", err))
	}
	if err := d.RegisterModule(characterstats.New()); err != nil {
		panic(fmt.Sprintf("failed to register characterstats module: %v", err))
	}
	if err := d.RegisterModule(mapnav.New()); err != nil {
		panic(fmt.Sprintf("failed to register mapnav module: %v", err))
	}
	if err := d.RegisterModule(console.New()); err != nil {
		panic(fmt.Sprintf("failed to register console module: %v", err))
	}
	if err := d.RegisterModule(inventory.New()); err != nil {
		panic(fmt.Sprintf("failed to register inventory module: %v", err))
	}
	if err := d.RegisterModule(store.New()); err != nil {
		panic(fmt.Sprintf("failed to register store module: %v", err))
	}
	if err := d.RegisterModule(farm.New()); err != nil {
		panic(fmt.Sprintf("failed to register farm module: %v", err))
	}
	if err := d.RegisterModule(transfer.New()); err != nil {
		panic(fmt.Sprintf("failed to register transfer module: %v", err))
	}

	return d
}

func (a *App) getMultiDispatcher() *multi.Dispatcher {
	if a.multiDispatcher == nil {
		a.multiDispatcher = a.newMultiDispatcher()
	}
	return a.multiDispatcher
}

// MultiApplySetting applies a single setting value across target bots.
func (a *App) MultiApplySetting(target multi.Target, key string, value any, opts multi.Opts) (multi.Result, error) {
	return a.getMultiDispatcher().ApplySetting(a.ctx, target, key, value, opts)
}

// MultiApplySettings applies a batch of settings atomically across target bots.
func (a *App) MultiApplySettings(target multi.Target, settings map[string]any, opts multi.Opts) (multi.Result, error) {
	return a.getMultiDispatcher().ApplySettings(a.ctx, target, settings, opts)
}

// MultiRunCommand dispatches an executable command across target bots.
func (a *App) MultiRunCommand(target multi.Target, key string, payload any, opts multi.Opts) (multi.Result, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return multi.Result{Error: err.Error()}, nil
	}
	return a.getMultiDispatcher().RunCommand(a.ctx, target, key, raw, opts)
}

// MultiReadSettings reads multiple settings across target bots with mixed-state detection.
func (a *App) MultiReadSettings(target multi.Target, keys []string) (map[string]multi.MixedValue, error) {
	return a.getMultiDispatcher().ReadSettings(a.ctx, target, keys)
}

// MultiGetSchema returns the complete application schema for dynamic UI discovery.
func (a *App) MultiGetSchema() multi.Schema {
	return a.getMultiDispatcher().GetSchema()
}

// MultiListModules returns metadata for registered modules.
func (a *App) MultiListModules() []multi.ModuleInfo {
	return a.getMultiDispatcher().ListModules()
}
