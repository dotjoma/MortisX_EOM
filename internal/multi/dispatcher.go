package multi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Dispatcher is the central orchestrator for multi-bot settings and commands.
type Dispatcher struct {
	deps           Deps
	registry       *Registry
	moduleRegistry *ModuleRegistry
	schemaBuilder  *SchemaBuilder
	activities     *ActivityManager
	serializer     *KeyedSerializer
	botMutexes     map[string]*sync.Mutex
	botMutexesMu   sync.Mutex
}

// NewDispatcher creates a new Dispatcher instance with the given dependencies.
func NewDispatcher(deps Deps) *Dispatcher {
	if deps.Workers <= 0 {
		deps.Workers = 12
	}
	if deps.Clock == nil {
		deps.Clock = RealClock{}
	}

	reg := NewRegistry()
	modReg := NewModuleRegistry(reg)
	sb := NewSchemaBuilder(reg, modReg)

	return &Dispatcher{
		deps:           deps,
		registry:       reg,
		moduleRegistry: modReg,
		schemaBuilder:  sb,
		activities:     NewActivityManager(),
		serializer:     NewKeyedSerializer(),
		botMutexes:     make(map[string]*sync.Mutex),
	}
}

// Registry returns the underlying capability Registry.
func (d *Dispatcher) Registry() *Registry {
	return d.registry
}

// ModuleRegistry returns the underlying ModuleRegistry.
func (d *Dispatcher) ModuleRegistry() *ModuleRegistry {
	return d.moduleRegistry
}

// RegisterModule registers a module into the module registry.
func (d *Dispatcher) RegisterModule(m Module) error {
	d.moduleRegistry.Register(m)
	return nil
}

// RegisterConfig registers a ConfigSpec directly into the registry.
func (d *Dispatcher) RegisterConfig(spec ConfigSpec) {
	RegisterConfigSpec(spec, d.registry)
}

// Activities returns the ActivityManager.
func (d *Dispatcher) Activities() *ActivityManager {
	return d.activities
}

// RemoveBot cleans up all internal mutexes, activities, and serializer keys for a removed bot.
func (d *Dispatcher) RemoveBot(botID string) {
	if botID == "" {
		return
	}
	d.botMutexesMu.Lock()
	delete(d.botMutexes, botID)
	d.botMutexesMu.Unlock()

	if d.activities != nil {
		d.activities.Clear(botID)
	}
	if d.serializer != nil {
		d.serializer.RemoveKey(botID)
	}
}

// GetSchema returns the full application Schema for UI auto-discovery.
func (d *Dispatcher) GetSchema() Schema {
	return d.schemaBuilder.Build()
}

// ListModules returns module metadata for UI sub-navigation.
func (d *Dispatcher) ListModules() []ModuleInfo {
	return d.schemaBuilder.ListModules()
}

func (d *Dispatcher) getBotMutex(botID string) *sync.Mutex {
	d.botMutexesMu.Lock()
	defer d.botMutexesMu.Unlock()
	m, ok := d.botMutexes[botID]
	if !ok {
		m = &sync.Mutex{}
		d.botMutexes[botID] = m
	}
	return m
}

// resolveTarget validates Target according to Section 4.3:
// - If IDs is non-empty, FocusedID must be in IDs. Otherwise reject with error.
// - If IDs is empty, resolve strictly to [FocusedID].
// - If Scope is FocusedOnly, resolve strictly to [FocusedID].
func (d *Dispatcher) resolveTarget(target Target, scope Scope) ([]Bot, error) {
	if len(target.IDs) == 0 {
		if target.FocusedID == "" {
			return nil, fmt.Errorf("no bot selected")
		}
		target.IDs = []string{target.FocusedID}
	} else {
		// Non-empty IDs: FocusedID must be a member
		if target.FocusedID == "" {
			target.FocusedID = target.IDs[0]
		} else {
			found := false
			for _, id := range target.IDs {
				if id == target.FocusedID {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("ErrFocusedNotInSelection: focused bot %q not in selection %v", target.FocusedID, target.IDs)
			}
		}
	}

	if scope == FocusedOnly {
		target.IDs = []string{target.FocusedID}
	}

	bots := make([]Bot, 0, len(target.IDs))
	for _, id := range target.IDs {
		b, ok := d.deps.Bots.Get(id)
		if ok && b != nil {
			bots = append(bots, b)
		}
	}

	return bots, nil
}

func toBotRefs(bots []Bot) []BotRef {
	refs := make([]BotRef, len(bots))
	for i, b := range bots {
		refs[i] = BotRef{
			ID:   b.ID(),
			Name: b.Name(),
		}
	}
	return refs
}

// ApplySetting sets a single setting across target bots.
func (d *Dispatcher) ApplySetting(ctx context.Context, target Target, key string, value any, opts Opts) (Result, error) {
	def, ok := d.registry.GetSetting(key)
	if !ok {
		return Result{Error: fmt.Sprintf("setting not found: %s", key)}, nil
	}

	bots, err := d.resolveTarget(target, def.Scope)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}

	// 1. Validation & Risk evaluation phase
	maxRisk := def.Risk
	affectedBots := make([]Bot, 0, len(bots))
	agg := NewResultAggregator(len(bots))
	validBots := make([]Bot, 0, len(bots))

	for _, b := range bots {
		if def.RiskFor != nil {
			r := def.RiskFor(b, value)
			if r > maxRisk {
				maxRisk = r
			}
		}
		affectedBots = append(affectedBots, b)

		if def.Validate != nil {
			if valErr := def.Validate(b, value); valErr != nil {
				agg.AddSkipped(b.ID(), valErr.Error())
				continue
			}
		}
		validBots = append(validBots, b)
	}

	// 2. Central confirmation gate
	if maxRisk == High && len(bots) > 1 && !opts.Confirm {
		return Result{
			Summary: agg.Summary(),
			Confirm: &ConfirmRequest{
				Entry: key,
				Bots:  toBotRefs(affectedBots),
			},
		}, nil
	}

	// 3. Execution phase
	okBotIDs := make([]string, 0, len(validBots))
	var okMu sync.Mutex

	for _, b := range validBots {
		botMutex := d.getBotMutex(b.ID())
		botMutex.Lock()

		var unlockSerializer func() = func() {}
		if def.SerializeBy != nil {
			sKey := def.SerializeBy(b)
			if sKey != "" {
				unlockSerializer = d.serializer.Lock(sKey)
			}
		}

		setErr := def.Set(b, value)
		unlockSerializer()
		botMutex.Unlock()

		if setErr != nil {
			agg.AddFailed(b.ID(), setErr.Error())
		} else {
			agg.AddOK(b.ID(), nil)
			okMu.Lock()
			okBotIDs = append(okBotIDs, b.ID())
			okMu.Unlock()

			if def.Persist && d.deps.Persister != nil {
				_ = d.deps.Persister.Save(b)
			}
		}
	}

	summary := agg.Summary()

	// 4. Emit events
	if len(okBotIDs) > 0 && d.deps.Emitter != nil {
		d.deps.Emitter.Emit("multi:setting-changed", SettingChangedPayload{
			Keys:   []string{key},
			BotIDs: okBotIDs,
		})
	}
	if d.deps.Emitter != nil {
		d.deps.Emitter.Emit("multi:summary", summary)
	}

	return Result{Summary: summary}, nil
}

// ApplySettings applies a batch of settings.
// Batch atomicity rule: rejects batches spanning more than one ConfigSpec.
// If all keys belong to the same ConfigSpec, mutates the struct once under lock.
func (d *Dispatcher) ApplySettings(ctx context.Context, target Target, settings map[string]any, opts Opts) (Result, error) {
	if len(settings) == 0 {
		return Result{Summary: Summary{}}, nil
	}

	defs := make(map[string]SettingDef, len(settings))
	var sharedSpec *ConfigSpec

	for k := range settings {
		def, ok := d.registry.GetSetting(k)
		if !ok {
			return Result{Error: fmt.Sprintf("setting not found: %s", k)}, nil
		}
		defs[k] = def

		if def.ConfigSpec != nil {
			if sharedSpec == nil {
				sharedSpec = def.ConfigSpec
			} else if sharedSpec != def.ConfigSpec {
				return Result{Error: "batch spans more than one ConfigSpec"}, nil
			}
		}
	}

	bots, err := d.resolveTarget(target, AllSelected)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}

	// Validation and Risk check
	maxRisk := Low
	for k, v := range settings {
		def := defs[k]
		if def.Risk > maxRisk {
			maxRisk = def.Risk
		}
		for _, b := range bots {
			if def.RiskFor != nil {
				r := def.RiskFor(b, v)
				if r > maxRisk {
					maxRisk = r
				}
			}
			if def.Validate != nil {
				if vErr := def.Validate(b, v); vErr != nil {
					return Result{Error: fmt.Sprintf("validation failed on %s for bot %s: %v", k, b.ID(), vErr)}, nil
				}
			}
		}
	}

	if maxRisk == High && len(bots) > 1 && !opts.Confirm {
		return Result{
			Confirm: &ConfirmRequest{
				Entry: "batch",
				Bots:  toBotRefs(bots),
			},
		}, nil
	}

	agg := NewResultAggregator(len(bots))
	okBotIDs := make([]string, 0, len(bots))
	var keysList []string
	for k := range settings {
		keysList = append(keysList, k)
	}

	for _, b := range bots {
		botMutex := d.getBotMutex(b.ID())
		botMutex.Lock()

		var opErr error
		if sharedSpec != nil {
			// Single locked read-modify-write on struct
			opErr = sharedSpec.Update(b, func(cfg any) {
				for k, v := range settings {
					def := defs[k]
					if def.MutateInPlace != nil {
						if err := def.MutateInPlace(cfg, v); err != nil {
							panic(err)
						}
					} else {
						_ = def.Set(b, v)
					}
				}
			})
		} else {
			// Sequential application
			for k, v := range settings {
				def := defs[k]
				if err := def.Set(b, v); err != nil {
					opErr = err
					break
				}
			}
		}

		botMutex.Unlock()

		if opErr != nil {
			agg.AddFailed(b.ID(), opErr.Error())
		} else {
			agg.AddOK(b.ID(), nil)
			okBotIDs = append(okBotIDs, b.ID())
			if d.deps.Persister != nil {
				_ = d.deps.Persister.Save(b)
			}
		}
	}

	summary := agg.Summary()
	if len(okBotIDs) > 0 && d.deps.Emitter != nil {
		d.deps.Emitter.Emit("multi:setting-changed", SettingChangedPayload{
			Keys:   keysList,
			BotIDs: okBotIDs,
		})
	}
	if d.deps.Emitter != nil {
		d.deps.Emitter.Emit("multi:summary", summary)
	}

	return Result{Summary: summary}, nil
}

// RunCommand dispatches an executable command across target bots.
func (d *Dispatcher) RunCommand(ctx context.Context, target Target, key string, payload json.RawMessage, opts Opts) (Result, error) {
	def, ok := d.registry.GetCommand(key)
	if !ok {
		return Result{Error: fmt.Sprintf("command not found: %s", key)}, nil
	}

	bots, err := d.resolveTarget(target, def.Scope)
	if err != nil {
		return Result{Error: err.Error()}, nil
	}

	agg := NewResultAggregator(len(bots))
	validBots := make([]Bot, 0, len(bots))
	maxRisk := def.Risk
	affectedBots := make([]Bot, 0, len(bots))

	// 1. Validation & Pre-execution checks
	for _, b := range bots {
		affectedBots = append(affectedBots, b)

		if def.RiskFor != nil {
			r := def.RiskFor(b, payload)
			if r > maxRisk {
				maxRisk = r
			}
		}

		if !b.Online() && !def.AllowOffline {
			agg.AddSkipped(b.ID(), "offline")
			continue
		}

		if def.CanRun != nil {
			if err := def.CanRun(b); err != nil {
				agg.AddSkipped(b.ID(), err.Error())
				continue
			}
		}

		if def.Validate != nil {
			if err := def.Validate(b, payload); err != nil {
				agg.AddSkipped(b.ID(), err.Error())
				continue
			}
		}

		// Bot-to-bot reference checks
		if def.RefsBots != nil {
			refs := def.RefsBots(payload)
			refFail := false
			for _, ref := range refs {
				// Check self-reference
				if ref.ID == b.ID() || (ref.Name != "" && ref.Name == b.Name()) {
					agg.AddSkipped(b.ID(), "self-reference")
					refFail = true
					break
				}

				// Resolve referenced bot via BotProvider
				var refBot Bot
				if ref.ID != "" {
					refBot, _ = d.deps.Bots.Get(ref.ID)
				} else if ref.Name != "" {
					refBot, _ = d.deps.Bots.ByName(ref.Name)
				}

				if refBot == nil || !refBot.Online() {
					agg.AddSkipped(b.ID(), "referenced bot offline")
					refFail = true
					break
				}

				if def.ValidateRef != nil {
					if err := def.ValidateRef(b, refBot, payload); err != nil {
						agg.AddSkipped(b.ID(), err.Error())
						refFail = true
						break
					}
				}
			}
			if refFail {
				continue
			}
		}

		validBots = append(validBots, b)
	}

	// 2. Central confirmation gate
	if maxRisk == High && len(bots) > 1 && !opts.Confirm {
		return Result{
			Summary: agg.Summary(),
			Confirm: &ConfirmRequest{
				Entry: key,
				Bots:  toBotRefs(affectedBots),
			},
		}, nil
	}

	// 3. Execution phase
	executeSingleBot := func(b Bot) {
		botMutex := d.getBotMutex(b.ID())
		botMutex.Lock()
		defer botMutex.Unlock()

		// Activity lock
		var unlockActivity func() = func() {}
		if def.Exclusive != "" {
			if def.LockRefs && def.RefsBots != nil {
				refs := def.RefsBots(payload)
				lockIDs := []string{b.ID()}
				for _, r := range refs {
					if r.ID != "" {
						lockIDs = append(lockIDs, r.ID)
					} else if r.Name != "" {
						if target, ok := d.deps.Bots.ByName(r.Name); ok {
							lockIDs = append(lockIDs, target.ID())
						}
					}
				}
				var ok bool
				var busyID, curAct string
				unlockActivity, ok, busyID, curAct = d.activities.TryLockMultiple(lockIDs, def.Exclusive)
				if !ok {
					agg.AddSkipped(b.ID(), fmt.Sprintf("busy: %s (bot %s)", curAct, busyID))
					return
				}
			} else {
				var ok bool
				var curAct string
				unlockActivity, ok, curAct = d.activities.TryLock(b.ID(), def.Exclusive)
				if !ok {
					agg.AddSkipped(b.ID(), fmt.Sprintf("busy: %s", curAct))
					return
				}
			}
		}
		defer unlockActivity()

		// Keyed serializer
		var unlockSerializer func() = func() {}
		if def.SerializeBy != nil {
			sKey := def.SerializeBy(b)
			if sKey != "" {
				unlockSerializer = d.serializer.Lock(sKey)
			}
		}
		defer unlockSerializer()

		// Timeout context
		botCtx := ctx
		var cancel context.CancelFunc = func() {}
		if def.Timeout > 0 {
			botCtx, cancel = context.WithTimeout(ctx, def.Timeout)
		}
		defer cancel()

		// Panic recovery per bot
		defer func() {
			if r := recover(); r != nil {
				agg.AddFailed(b.ID(), fmt.Sprintf("panic: %v", r))
			}
		}()

		res, execErr := def.Execute(botCtx, b, payload)
		if execErr != nil {
			if botCtx.Err() == context.DeadlineExceeded {
				agg.AddFailed(b.ID(), "timeout")
			} else {
				agg.AddFailed(b.ID(), execErr.Error())
			}
		} else {
			agg.AddOK(b.ID(), res)
		}
	}

	// Deduplication tracking across batch
	seenDedupe := make(map[string]struct{})
	var dedupeMu sync.Mutex

	checkDedupe := func(b Bot) bool {
		if def.Dedupe == nil {
			return false
		}
		dKey := def.Dedupe(b, payload)
		if dKey == "" {
			return false
		}
		dedupeMu.Lock()
		defer dedupeMu.Unlock()
		if _, exists := seenDedupe[dKey]; exists {
			return true
		}
		seenDedupe[dKey] = struct{}{}
		return false
	}

	if def.Mode == Sequential {
		for i, b := range validBots {
			if checkDedupe(b) {
				agg.AddSkipped(b.ID(), fmt.Sprintf("duplicate %s", def.Dedupe(b, payload)))
				continue
			}
			if i > 0 && def.Stagger > 0 {
				<-d.deps.Clock.After(def.Stagger)
			}
			executeSingleBot(b)
		}
	} else {
		// Parallel worker pool
		sem := make(chan struct{}, d.deps.Workers)
		var wg sync.WaitGroup

		for i, b := range validBots {
			if checkDedupe(b) {
				agg.AddSkipped(b.ID(), fmt.Sprintf("duplicate %s", def.Dedupe(b, payload)))
				continue
			}

			if i > 0 && def.Stagger > 0 {
				<-d.deps.Clock.After(def.Stagger)
			}

			wg.Add(1)
			go func(bot Bot) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				executeSingleBot(bot)
			}(b)
		}
		wg.Wait()
	}

	summary := agg.Summary()
	if d.deps.Emitter != nil {
		d.deps.Emitter.Emit("multi:summary", summary)
	}

	return Result{Summary: summary}, nil
}

// ReadSettings reads values for multiple keys across target bots, evaluating mixed state.
func (d *Dispatcher) ReadSettings(ctx context.Context, target Target, keys []string) (map[string]MixedValue, error) {
	if len(target.IDs) == 0 {
		if target.FocusedID == "" {
			return nil, fmt.Errorf("no bot selected")
		}
		target.IDs = []string{target.FocusedID}
	} else {
		if target.FocusedID == "" {
			target.FocusedID = target.IDs[0]
		} else {
			found := false
			for _, id := range target.IDs {
				if id == target.FocusedID {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("ErrFocusedNotInSelection: focused bot %q not in selection %v", target.FocusedID, target.IDs)
			}
		}
	}

	bots := make([]Bot, 0, len(target.IDs))
	for _, id := range target.IDs {
		b, ok := d.deps.Bots.Get(id)
		if ok && b != nil {
			bots = append(bots, b)
		}
	}

	resultMap := make(map[string]MixedValue, len(keys))

	for _, key := range keys {
		var rawValues = make(map[string]any, len(bots))
		var sensitive bool

		if sDef, ok := d.registry.GetSetting(key); ok {
			sensitive = sDef.Sensitive
			for _, b := range bots {
				val, err := sDef.Get(b)
				if err == nil {
					rawValues[b.ID()] = val
				}
			}
		} else if rDef, ok := d.registry.GetReader(key); ok {
			sensitive = rDef.Sensitive
			for _, b := range bots {
				val, err := rDef.Read(b)
				if err == nil {
					rawValues[b.ID()] = val
				}
			}
		} else {
			return nil, fmt.Errorf("key not registered as setting or reader: %s", key)
		}

		resultMap[key] = ComputeMixedValue(rawValues, target.FocusedID, sensitive)
	}

	return resultMap, nil
}
