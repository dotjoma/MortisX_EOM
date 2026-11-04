package multi

import (
	"fmt"
	"strings"
	"sync"
)

// Registry stores all registered settings, commands, collections, and readers.
type Registry struct {
	mu          sync.RWMutex
	settings    map[string]SettingDef
	commands    map[string]CommandDef
	collections map[string]CollectionDef
	readers     map[string]ReaderDef
}

// NewRegistry creates a new Registry.
func NewRegistry() *Registry {
	return &Registry{
		settings:    make(map[string]SettingDef),
		commands:    make(map[string]CommandDef),
		collections: make(map[string]CollectionDef),
		readers:     make(map[string]ReaderDef),
	}
}

func (r *Registry) validateKey(prefix, key string) {
	if prefix != "" && !strings.HasPrefix(key, prefix) {
		panic(fmt.Sprintf("key %q does not match module prefix %q", key, prefix))
	}
	if _, exists := r.settings[key]; exists {
		panic(fmt.Sprintf("duplicate registration for key: %s", key))
	}
	if _, exists := r.commands[key]; exists {
		panic(fmt.Sprintf("duplicate registration for key: %s", key))
	}
	if _, exists := r.collections[key]; exists {
		panic(fmt.Sprintf("duplicate registration for key: %s", key))
	}
	if _, exists := r.readers[key]; exists {
		panic(fmt.Sprintf("duplicate registration for key: %s", key))
	}
}

// RegisterSetting registers a SettingDef, panicking if duplicate or outside prefix.
func (r *Registry) RegisterSetting(prefix string, def SettingDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validateKey(prefix, def.Key)
	if def.Kind == "" {
		def.Kind = "setting"
	}
	r.settings[def.Key] = def
}

// RegisterCommand registers a CommandDef, panicking if duplicate or outside prefix.
func (r *Registry) RegisterCommand(prefix string, def CommandDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validateKey(prefix, def.Key)
	if def.Kind == "" {
		def.Kind = "command"
	}
	r.commands[def.Key] = def
}

// RegisterCollection registers a CollectionDef, panicking if duplicate or outside prefix.
func (r *Registry) RegisterCollection(prefix string, def CollectionDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validateKey(prefix, def.Key)
	if def.Kind == "" {
		def.Kind = "collection"
	}
	r.collections[def.Key] = def
}

// RegisterReader registers a ReaderDef, panicking if duplicate or outside prefix.
func (r *Registry) RegisterReader(prefix string, def ReaderDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validateKey(prefix, def.Key)
	if def.Kind == "" {
		def.Kind = "reader"
	}
	r.readers[def.Key] = def
}

// GetSetting retrieves a SettingDef by key.
func (r *Registry) GetSetting(key string) (SettingDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.settings[key]
	return s, ok
}

// GetCommand retrieves a CommandDef by key.
func (r *Registry) GetCommand(key string) (CommandDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.commands[key]
	return c, ok
}

// GetCollection retrieves a CollectionDef by key.
func (r *Registry) GetCollection(key string) (CollectionDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collections[key]
	return c, ok
}

// GetReader retrieves a ReaderDef by key.
func (r *Registry) GetReader(key string) (ReaderDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rd, ok := r.readers[key]
	return rd, ok
}

// AllSettings returns a slice of all registered SettingDefs.
func (r *Registry) AllSettings() []SettingDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]SettingDef, 0, len(r.settings))
	for _, s := range r.settings {
		res = append(res, s)
	}
	return res
}

// AllCommands returns a slice of all registered CommandDefs.
func (r *Registry) AllCommands() []CommandDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]CommandDef, 0, len(r.commands))
	for _, c := range r.commands {
		res = append(res, c)
	}
	return res
}

// AllCollections returns a slice of all registered CollectionDefs.
func (r *Registry) AllCollections() []CollectionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]CollectionDef, 0, len(r.collections))
	for _, c := range r.collections {
		res = append(res, c)
	}
	return res
}

// AllReaders returns a slice of all registered ReaderDefs.
func (r *Registry) AllReaders() []ReaderDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]ReaderDef, 0, len(r.readers))
	for _, rd := range r.readers {
		res = append(res, rd)
	}
	return res
}
