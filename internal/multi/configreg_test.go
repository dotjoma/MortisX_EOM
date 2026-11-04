package multi

import (
	"errors"
	"reflect"
	"testing"
)

type TestExploits struct {
	GodMode bool `multi:"godMode" risk:"high"`
}

type TestFarmConfig struct {
	Mode        string       `multi:"mode" enum:"hunt,gather,idle"`
	LeashRadius float64      `multi:"leashRadius" min:"10" max:"500"`
	Step        float32      `multi:"step" min:"0.2" max:"20"`
	Workers     int          `multi:"workers" min:"1" max:"16"`
	SmartMode   bool         `multi:"smartMode"`
	Webhook     string       `multi:"webhookUrl" sensitive:"true"`
	Exploits    TestExploits `multi:"exploits"`
	BuyList     []string     `collection:"buyList"`
	CachedData  string       `multi:"-"`
}

type BadSliceConfig struct {
	Items []string // Missing collection or multi:"-"
}

type MissingTagConfig struct {
	UnTaggedField string // Exported but missing multi tag
}

func TestConfigReg_Coercion(t *testing.T) {
	// 1. Valid int coercion from float64
	val, err := CoerceValue(float64(5.0), reflect.TypeOf(int(0)))
	if err != nil || val.(int) != 5 {
		t.Fatalf("expected int 5, got %v, err %v", val, err)
	}

	// 2. Lossy float64 to int rejected
	_, err = CoerceValue(float64(5.5), reflect.TypeOf(int(0)))
	if err == nil {
		t.Fatalf("expected error on lossy float 5.5 to int")
	}

	// 3. Valid float32 coercion
	val, err = CoerceValue(float64(1.2), reflect.TypeOf(float32(0)))
	if err != nil || val.(float32) != float32(1.2) {
		t.Fatalf("expected float32 1.2, got %v, err %v", val, err)
	}

	// 4. Float32 overflow rejected
	_, err = CoerceValue(float64(1e39), reflect.TypeOf(float32(0)))
	if err == nil {
		t.Fatalf("expected error on float32 overflow")
	}

	// 5. Valid bool
	val, err = CoerceValue(true, reflect.TypeOf(true))
	if err != nil || val.(bool) != true {
		t.Fatalf("expected bool true")
	}

	// 6. Valid string
	val, err = CoerceValue("hunt", reflect.TypeOf(""))
	if err != nil || val.(string) != "hunt" {
		t.Fatalf("expected string 'hunt'")
	}
}

func TestConfigReg_TagsAndRegistration(t *testing.T) {
	reg := NewRegistry()
	cfg := &TestFarmConfig{
		Mode:        "hunt",
		LeashRadius: 100,
		Step:        1.2,
		Workers:     4,
		SmartMode:   true,
		Exploits:    TestExploits{GodMode: false},
	}

	updateCount := 0
	spec := ConfigSpec{
		Prefix: "automation.farm.",
		Target: cfg,
		Get: func(b Bot) any {
			return cfg
		},
		Update: func(b Bot, mutate func(cfg any)) error {
			mutate(cfg)
			updateCount++
			return nil
		},
		Validate: func(old, new any) error {
			c := new.(*TestFarmConfig)
			if c.LeashRadius > 500 {
				return errors.New("leash radius exceeds 500")
			}
			return nil
		},
		Persist: true,
	}

	RegisterConfigSpec(spec, reg)

	// Verify registered keys
	modeDef, ok := reg.GetSetting("automation.farm.mode")
	if !ok {
		t.Fatalf("expected automation.farm.mode to be registered")
	}

	mockB := &testBot{id: "bot-1", online: true}

	// Test valid enum
	if err := modeDef.Validate(mockB, "gather"); err != nil {
		t.Fatalf("expected gather to be valid enum, got %v", err)
	}

	// Test invalid enum
	if err := modeDef.Validate(mockB, "invalid_mode"); err == nil {
		t.Fatalf("expected error on invalid enum")
	}

	// Test min/max validation
	radiusDef, ok := reg.GetSetting("automation.farm.leashRadius")
	if !ok {
		t.Fatalf("expected leashRadius to be registered")
	}
	if err := radiusDef.Validate(mockB, float64(5)); err == nil {
		t.Fatalf("expected error on radius < 10")
	}
	if err := radiusDef.Validate(mockB, float64(600)); err == nil {
		t.Fatalf("expected error on radius > 500")
	}
	if err := radiusDef.Validate(mockB, float64(250)); err != nil {
		t.Fatalf("expected radius 250 to be valid: %v", err)
	}

	// Test Set and Update hook
	if err := radiusDef.Set(mockB, float64(250)); err != nil {
		t.Fatalf("failed to set leashRadius: %v", err)
	}
	if cfg.LeashRadius != 250 {
		t.Fatalf("expected cfg.LeashRadius to be 250, got %v", cfg.LeashRadius)
	}
	if updateCount != 1 {
		t.Fatalf("expected updateCount 1, got %d", updateCount)
	}

	// Test nested struct key
	godModeDef, ok := reg.GetSetting("automation.farm.exploits.godMode")
	if !ok {
		t.Fatalf("expected exploits.godMode to be registered")
	}
	if godModeDef.Risk != High {
		t.Fatalf("expected godMode to have High risk, got %v", godModeDef.Risk)
	}

	// Test sensitive key
	webhookDef, ok := reg.GetSetting("automation.farm.webhookUrl")
	if !ok {
		t.Fatalf("expected webhookUrl to be registered")
	}
	if !webhookDef.Sensitive {
		t.Fatalf("expected webhookUrl to be sensitive")
	}
}

func TestConfigReg_BadSliceFails(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on slice without collection tag")
		}
	}()

	reg := NewRegistry()
	RegisterConfigSpec(ConfigSpec{
		Prefix: "test.",
		Target: &BadSliceConfig{},
	}, reg)
}

func TestConfigReg_MissingTagFails(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on exported field missing tags")
		}
	}()

	reg := NewRegistry()
	RegisterConfigSpec(ConfigSpec{
		Prefix: "test.",
		Target: &MissingTagConfig{},
	}, reg)
}

// Dedicated standalone test: enum rejects invalid values
func TestConfigReg_EnumRejectsInvalidValues(t *testing.T) {
	reg := NewRegistry()
	cfg := &TestFarmConfig{Mode: "hunt"}
	RegisterConfigSpec(ConfigSpec{
		Prefix: "farm.",
		Target: cfg,
	}, reg)

	modeDef, ok := reg.GetSetting("farm.mode")
	if !ok {
		t.Fatalf("mode setting not found")
	}

	bot := &testBot{id: "bot-1", online: true}
	// Valid values
	for _, valid := range []string{"hunt", "gather", "idle"} {
		if err := modeDef.Validate(bot, valid); err != nil {
			t.Fatalf("expected %q to be valid enum option, got err: %v", valid, err)
		}
	}

	// Invalid values
	for _, invalid := range []string{"invalid", "unknown", "HUNT", "hunt,gather"} {
		if err := modeDef.Validate(bot, invalid); err == nil {
			t.Fatalf("expected %q to be rejected as invalid enum option", invalid)
		}
	}
}

// Dedicated standalone test: Update hook fires OnChange exactly once per update
func TestConfigReg_OnChangeFiresExactlyOncePerUpdate(t *testing.T) {
	reg := NewRegistry()
	cfg := &TestFarmConfig{LeashRadius: 50}
	onChangeCalls := 0

	RegisterConfigSpec(ConfigSpec{
		Prefix: "farm.",
		Target: cfg,
		Get:    func(b Bot) any { return cfg },
		Update: func(b Bot, mutate func(cfg any)) error {
			mutate(cfg)
			onChangeCalls++ // Simulates engine OnChange(old, new)
			return nil
		},
	}, reg)

	radiusDef, ok := reg.GetSetting("farm.leashRadius")
	if !ok {
		t.Fatalf("leashRadius setting not found")
	}

	bot := &testBot{id: "bot-1", online: true}
	if err := radiusDef.Set(bot, float64(150)); err != nil {
		t.Fatalf("failed to set leashRadius: %v", err)
	}

	if onChangeCalls != 1 {
		t.Fatalf("expected OnChange to fire exactly 1 time, got %d", onChangeCalls)
	}
	if cfg.LeashRadius != 150 {
		t.Fatalf("expected LeashRadius to be 150, got %v", cfg.LeashRadius)
	}
}
