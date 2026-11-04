package multi

import (
	"strings"
)

// SchemaBuilder generates Schema and ModuleInfo structures for UI reflection.
type SchemaBuilder struct {
	registry       *Registry
	moduleRegistry *ModuleRegistry
}

// NewSchemaBuilder creates a SchemaBuilder.
func NewSchemaBuilder(reg *Registry, modReg *ModuleRegistry) *SchemaBuilder {
	return &SchemaBuilder{
		registry:       reg,
		moduleRegistry: modReg,
	}
}

// Build generates the complete application Schema.
func (sb *SchemaBuilder) Build() Schema {
	modules := sb.ListModules()
	return Schema{
		Modules: modules,
	}
}

// ListModules returns ModuleInfo for each registered module, populated with its fields.
func (sb *SchemaBuilder) ListModules() []ModuleInfo {
	if sb.moduleRegistry == nil {
		return nil
	}

	rawModules := sb.moduleRegistry.Modules()
	res := make([]ModuleInfo, 0, len(rawModules))

	for _, m := range rawModules {
		tab := m.Tab()
		id := m.ID()
		prefix := strings.ToLower(tab) + "." + strings.ToLower(id) + "."

		info := ModuleInfo{
			Tab:    tab,
			ID:     id,
			Title:  m.Title(),
			Order:  m.Order(),
			Icon:   m.Icon(),
			Fields: make([]FieldSchema, 0),
		}

		if sb.registry != nil {
			// Find all settings with this prefix
			for _, s := range sb.registry.AllSettings() {
				if strings.HasPrefix(s.Key, prefix) {
					info.Fields = append(info.Fields, FieldSchema{
						Key:       s.Key,
						Kind:      "setting",
						Risk:      s.Risk.String(),
						Scope:     s.Scope.String(),
						Sensitive: s.Sensitive,
						Label:     deriveLabel(s.Key),
					})
				}
			}

			// Find all commands with this prefix
			for _, c := range sb.registry.AllCommands() {
				if strings.HasPrefix(c.Key, prefix) {
					info.Fields = append(info.Fields, FieldSchema{
						Key:   c.Key,
						Kind:  "command",
						Risk:  c.Risk.String(),
						Scope: c.Scope.String(),
						Label: deriveLabel(c.Key),
					})
				}
			}

			// Find all collections with this prefix
			for _, col := range sb.registry.AllCollections() {
				if strings.HasPrefix(col.Key, prefix) {
					info.Fields = append(info.Fields, FieldSchema{
						Key:   col.Key,
						Kind:  "collection",
						Risk:  col.Risk.String(),
						Scope: AllSelected.String(),
						Label: deriveLabel(col.Key),
					})
				}
			}

			// Find all readers with this prefix
			for _, r := range sb.registry.AllReaders() {
				if strings.HasPrefix(r.Key, prefix) {
					info.Fields = append(info.Fields, FieldSchema{
						Key:       r.Key,
						Kind:      "reader",
						Scope:     r.Scope.String(),
						Sensitive: r.Sensitive,
						Label:     deriveLabel(r.Key),
					})
				}
			}
		}

		res = append(res, info)
	}

	return res
}

func deriveLabel(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return key
	}
	last := parts[len(parts)-1]
	return strings.Title(strings.ReplaceAll(last, "_", " "))
}
