package multi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type dummyFarmModule struct{}

func (dummyFarmModule) Tab() string   { return "automation" }
func (dummyFarmModule) ID() string    { return "farm" }
func (dummyFarmModule) Title() string { return "Farm" }
func (dummyFarmModule) Order() int    { return 1 }
func (dummyFarmModule) Icon() string  { return "cpu" }
func (dummyFarmModule) Register(r Registrar) {
	r.Setting(SettingDef{
		Key:       "automation.farm.mode",
		Scope:     AllSelected,
		Risk:      Low,
		Sensitive: false,
	})
	r.Command(CommandDef{
		Key:   "automation.farm.start",
		Scope: AllSelected,
		Risk:  Low,
	})
	r.Collection(CollectionDef{
		Key:  "automation.farm.buyList",
		Risk: Low,
	})
	r.Reader(ReaderDef{
		Key:       "automation.farm.stats",
		Scope:     FocusedOnly,
		Sensitive: false,
	})
}

func TestSchema_Golden(t *testing.T) {
	reg := NewRegistry()
	modReg := NewModuleRegistry(reg)
	modReg.Register(dummyFarmModule{})

	sb := NewSchemaBuilder(reg, modReg)
	schema := sb.Build()

	schemaBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal schema: %v", err)
	}

	goldenPath := filepath.Join("testdata", "schema_golden.json")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		// First run: generate golden file
		_ = os.MkdirAll("testdata", 0755)
		if err := os.WriteFile(goldenPath, schemaBytes, 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
	}

	expectedBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if string(schemaBytes) != string(expectedBytes) {
		t.Fatalf("schema does not match golden file!\nGot:\n%s\nExpected:\n%s", string(schemaBytes), string(expectedBytes))
	}
}
