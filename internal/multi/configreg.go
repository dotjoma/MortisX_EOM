package multi

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// ConfigSpec configures tag-driven reflection over a configuration struct.
type ConfigSpec struct {
	Prefix   string
	Target   any // Pointer to struct or struct instance
	Get      func(b Bot) any
	Update   func(b Bot, mutate func(cfg any)) error
	Validate func(old, new any) error // Cross-field validation hook
	Persist  bool                     // Default persistence flag for fields
}

type fieldAccessor struct {
	index     []int
	fieldType reflect.Type
}

func (fa *fieldAccessor) get(val reflect.Value) any {
	v := reflect.Indirect(val)
	fieldVal := v.FieldByIndex(fa.index)
	return fieldVal.Interface()
}

func (fa *fieldAccessor) set(val reflect.Value, newVal any) {
	v := reflect.Indirect(val)
	fieldVal := v.FieldByIndex(fa.index)
	newRVal := reflect.ValueOf(newVal)
	if newRVal.Type() != fieldVal.Type() && newRVal.Type().ConvertibleTo(fieldVal.Type()) {
		newRVal = newRVal.Convert(fieldVal.Type())
	}
	fieldVal.Set(newRVal)
}

// CoerceValue converts an input value (often float64 from JSON) into the target reflect.Type,
// strictly rejecting lossy conversions (e.g. 1.5 to int or out-of-range bounds).
func CoerceValue(input any, targetType reflect.Type) (any, error) {
	if input == nil {
		return nil, fmt.Errorf("cannot coerce nil to %v", targetType)
	}

	inVal := reflect.ValueOf(input)
	if inVal.Type().AssignableTo(targetType) {
		return input, nil
	}

	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var int64Val int64
		switch v := input.(type) {
		case float64:
			if v != math.Trunc(v) {
				return nil, fmt.Errorf("lossy conversion: fractional float %v cannot be converted to %v", v, targetType)
			}
			if v < math.MinInt64 || v > math.MaxInt64 {
				return nil, fmt.Errorf("integer overflow: %v", v)
			}
			int64Val = int64(v)
		case int:
			int64Val = int64(v)
		case int64:
			int64Val = v
		case int32:
			int64Val = int64(v)
		default:
			return nil, fmt.Errorf("cannot convert %T to %v", input, targetType)
		}

		// Check bounds for specific int types
		switch targetType.Kind() {
		case reflect.Int8:
			if int64Val < math.MinInt8 || int64Val > math.MaxInt8 {
				return nil, fmt.Errorf("int8 out of bounds: %d", int64Val)
			}
			return int8(int64Val), nil
		case reflect.Int16:
			if int64Val < math.MinInt16 || int64Val > math.MaxInt16 {
				return nil, fmt.Errorf("int16 out of bounds: %d", int64Val)
			}
			return int16(int64Val), nil
		case reflect.Int32:
			if int64Val < math.MinInt32 || int64Val > math.MaxInt32 {
				return nil, fmt.Errorf("int32 out of bounds: %d", int64Val)
			}
			return int32(int64Val), nil
		case reflect.Int:
			return int(int64Val), nil
		case reflect.Int64:
			return int64Val, nil
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var uint64Val uint64
		switch v := input.(type) {
		case float64:
			if v < 0 || v != math.Trunc(v) {
				return nil, fmt.Errorf("cannot convert negative or fractional float %v to %v", v, targetType)
			}
			uint64Val = uint64(v)
		case uint:
			uint64Val = uint64(v)
		case uint64:
			uint64Val = v
		case int:
			if v < 0 {
				return nil, fmt.Errorf("cannot convert negative int %d to %v", v, targetType)
			}
			uint64Val = uint64(v)
		default:
			return nil, fmt.Errorf("cannot convert %T to %v", input, targetType)
		}

		switch targetType.Kind() {
		case reflect.Uint8:
			if uint64Val > math.MaxUint8 {
				return nil, fmt.Errorf("uint8 out of bounds: %d", uint64Val)
			}
			return uint8(uint64Val), nil
		case reflect.Uint16:
			if uint64Val > math.MaxUint16 {
				return nil, fmt.Errorf("uint16 out of bounds: %d", uint64Val)
			}
			return uint16(uint64Val), nil
		case reflect.Uint32:
			if uint64Val > math.MaxUint32 {
				return nil, fmt.Errorf("uint32 out of bounds: %d", uint64Val)
			}
			return uint32(uint64Val), nil
		case reflect.Uint:
			return uint(uint64Val), nil
		case reflect.Uint64:
			return uint64Val, nil
		}

	case reflect.Float32:
		switch v := input.(type) {
		case float64:
			if math.Abs(v) > math.MaxFloat32 {
				return nil, fmt.Errorf("float32 overflow: %v", v)
			}
			return float32(v), nil
		case float32:
			return v, nil
		case int:
			return float32(v), nil
		default:
			return nil, fmt.Errorf("cannot convert %T to float32", input)
		}

	case reflect.Float64:
		switch v := input.(type) {
		case float64:
			return v, nil
		case float32:
			return float64(v), nil
		case int:
			return float64(v), nil
		default:
			return nil, fmt.Errorf("cannot convert %T to float64", input)
		}

	case reflect.Bool:
		if b, ok := input.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("cannot convert %T to bool", input)

	case reflect.String:
		if s, ok := input.(string); ok {
			return s, nil
		}
		return nil, fmt.Errorf("cannot convert %T to string", input)
	}

	return nil, fmt.Errorf("unsupported target coercion type: %v", targetType)
}

func toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// RegisterConfigSpec inspects spec.Target and registers each tagged field as a SettingDef.
func RegisterConfigSpec(spec ConfigSpec, reg *Registry) {
	t := reflect.TypeOf(spec.Target)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic("ConfigSpec.Target must be a pointer to a struct or a struct")
	}

	specPtr := &spec
	scanStructFields(specPtr, t, nil, spec.Prefix, reg)
}

func scanStructFields(spec *ConfigSpec, t reflect.Type, baseIndex []int, prefix string, reg *Registry) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // Unexported
		}

		multiTag := field.Tag.Get("multi")
		colTag := field.Tag.Get("collection")

		if multiTag == "-" {
			continue // Explicit opt-out
		}

		fieldIndex := append(append([]int(nil), baseIndex...), i)

		// Slice/map check
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map {
			if colTag == "" {
				panic(fmt.Sprintf("slice/map field %s must declare collection or multi:\"-\"", field.Name))
			}
			// Registered via module.Collection(...)
			continue
		}

		// Struct recursion (e.g. Exploits struct)
		if field.Type.Kind() == reflect.Struct {
			subPrefix := prefix
			if multiTag != "" {
				subPrefix = prefix + multiTag + "."
			}
			scanStructFields(spec, field.Type, fieldIndex, subPrefix, reg)
			continue
		}

		if multiTag == "" && colTag == "" {
			panic(fmt.Sprintf("exported field %s missing multi or collection tag", field.Name))
		}

		fullKey := prefix + multiTag
		fa := &fieldAccessor{
			index:     fieldIndex,
			fieldType: field.Type,
		}

		// Parse tags
		enumTag := field.Tag.Get("enum")
		var enumOptions []string
		if enumTag != "" {
			parts := strings.Split(enumTag, ",")
			for _, p := range parts {
				enumOptions = append(enumOptions, strings.TrimSpace(p))
			}
		}

		var minVal *float64
		if minStr := field.Tag.Get("min"); minStr != "" {
			if f, err := strconv.ParseFloat(minStr, 64); err == nil {
				minVal = &f
			}
		}

		var maxVal *float64
		if maxStr := field.Tag.Get("max"); maxStr != "" {
			if f, err := strconv.ParseFloat(maxStr, 64); err == nil {
				maxVal = &f
			}
		}

		risk := ParseRisk(field.Tag.Get("risk"))
		sensitive := field.Tag.Get("sensitive") == "true"
		scope := AllSelected
		if field.Tag.Get("scope") == "focused" {
			scope = FocusedOnly
		}

		persist := spec.Persist
		if field.Tag.Get("persist") == "false" {
			persist = false
		}

		// Build per-field validator
		validateFn := func(b Bot, v any) error {
			coerced, err := CoerceValue(v, fa.fieldType)
			if err != nil {
				return err
			}

			// Validate enum
			if len(enumOptions) > 0 {
				strVal, ok := coerced.(string)
				if !ok {
					return fmt.Errorf("enum field %s requires string value, got %T", fullKey, coerced)
				}
				matched := false
				for _, opt := range enumOptions {
					if opt == strVal {
						matched = true
						break
					}
				}
				if !matched {
					return fmt.Errorf("invalid enum value %q for %s; allowed: %v", strVal, fullKey, enumOptions)
				}
			}

			// Validate min/max
			if num, ok := toFloat64(coerced); ok {
				if minVal != nil && num < *minVal {
					return fmt.Errorf("value %v is less than min %v for %s", num, *minVal, fullKey)
				}
				if maxVal != nil && num > *maxVal {
					return fmt.Errorf("value %v is greater than max %v for %s", num, *maxVal, fullKey)
				}
			}

			return nil
		}

		settingDef := SettingDef{
			Key:        fullKey,
			Scope:      scope,
			Validate:   validateFn,
			Persist:    persist,
			Risk:       risk,
			Sensitive:  sensitive,
			Kind:       "setting",
			ConfigSpec: spec,
			Get: func(b Bot) (any, error) {
				cfg := spec.Get(b)
				if cfg == nil {
					return nil, fmt.Errorf("bot %s config is nil", b.ID())
				}
				val := reflect.ValueOf(cfg)
				return fa.get(val), nil
			},
			MutateInPlace: func(target any, v any) error {
				coerced, err := CoerceValue(v, fa.fieldType)
				if err != nil {
					return err
				}
				val := reflect.ValueOf(target)
				fa.set(val, coerced)
				return nil
			},
			Set: func(b Bot, v any) error {
				coerced, err := CoerceValue(v, fa.fieldType)
				if err != nil {
					return err
				}

				return spec.Update(b, func(cfg any) {
					oldCfg := cfg
					val := reflect.ValueOf(cfg)
					fa.set(val, coerced)
					if spec.Validate != nil {
						if err := spec.Validate(oldCfg, cfg); err != nil {
							panic(err)
						}
					}
				})
			},
		}

		reg.RegisterSetting(spec.Prefix, settingDef)
	}
}
