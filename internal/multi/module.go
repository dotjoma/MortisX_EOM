package multi

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Module represents a self-describing functional unit within a tab.
type Module interface {
	Tab() string          // e.g. "automation", "main"
	ID() string           // e.g. "farm", "transfer". Key prefix becomes "<tab>.<id>."
	Title() string        // Dynamic title for navigation
	Order() int           // Sub-navigation ordering
	Icon() string         // Icon name; empty = default icon
	Register(r Registrar) // Declares settings, commands, collections, readers
}

// Registrar is passed to Module.Register to register capabilities.
type Registrar interface {
	Config(spec ConfigSpec)
	Setting(def SettingDef)
	Command(def CommandDef)
	Collection(def CollectionDef)
	Reader(def ReaderDef)
}

// ScopedRegistrar enforces prefix matching when a module registers its capabilities.
type ScopedRegistrar struct {
	prefix   string
	registry *Registry
}

// NewScopedRegistrar creates a registrar tied to a specific key prefix.
func NewScopedRegistrar(prefix string, registry *Registry) *ScopedRegistrar {
	return &ScopedRegistrar{
		prefix:   prefix,
		registry: registry,
	}
}

func (sr *ScopedRegistrar) Setting(def SettingDef) {
	sr.registry.RegisterSetting(sr.prefix, def)
}

func (sr *ScopedRegistrar) Command(def CommandDef) {
	sr.registry.RegisterCommand(sr.prefix, def)
}

func (sr *ScopedRegistrar) Collection(def CollectionDef) {
	sr.registry.RegisterCollection(sr.prefix, def)
}

func (sr *ScopedRegistrar) Reader(def ReaderDef) {
	sr.registry.RegisterReader(sr.prefix, def)
}

func (sr *ScopedRegistrar) Config(spec ConfigSpec) {
	if spec.Prefix == "" {
		spec.Prefix = sr.prefix
	}
	RegisterConfigSpec(spec, sr.registry)
}

// ModuleRegistry manages registered modules.
type ModuleRegistry struct {
	mu       sync.RWMutex
	modules  map[string]Module
	registry *Registry
}

// NewModuleRegistry creates a ModuleRegistry backed by a central Registry.
func NewModuleRegistry(registry *Registry) *ModuleRegistry {
	return &ModuleRegistry{
		modules:  make(map[string]Module),
		registry: registry,
	}
}

// Register adds a module, invoking its Register method under its prefix.
func (mr *ModuleRegistry) Register(m Module) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	tab := strings.ToLower(m.Tab())
	id := strings.ToLower(m.ID())
	key := fmt.Sprintf("%s.%s", tab, id)
	if _, exists := mr.modules[key]; exists {
		panic(fmt.Sprintf("module %s already registered", key))
	}

	mr.modules[key] = m
	prefix := fmt.Sprintf("%s.%s.", tab, id)
	scoped := NewScopedRegistrar(prefix, mr.registry)
	m.Register(scoped)
}

// Modules returns all registered modules sorted by Tab then Order.
func (mr *ModuleRegistry) Modules() []Module {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	list := make([]Module, 0, len(mr.modules))
	for _, m := range mr.modules {
		list = append(list, m)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Tab() != list[j].Tab() {
			return list[i].Tab() < list[j].Tab()
		}
		return list[i].Order() < list[j].Order()
	})

	return list
}
