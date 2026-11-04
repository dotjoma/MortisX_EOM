package multi

import "reflect"

// MaskSensitive returns "***" if sensitive is true, or the original value otherwise.
func MaskSensitive(val any, sensitive bool) any {
	if sensitive {
		return "***"
	}
	return val
}

// ComputeMixedValue evaluates raw values read across multiple bots.
// Raw values are compared to determine if a divergent "Mixed" state exists.
// When not mixed, the real value of the focused bot is returned in Value (even if sensitive).
// When mixed, Mixed is true and PerBot contains per-bot values, with Sensitive values masked as "***".
func ComputeMixedValue(rawValues map[string]any, focusedID string, sensitive bool) MixedValue {
	if len(rawValues) == 0 {
		return MixedValue{Mixed: false, Value: nil}
	}

	// Check if all values are identical
	var firstVal any
	hasFirst := false
	isMixed := false

	for _, val := range rawValues {
		if !hasFirst {
			firstVal = val
			hasFirst = true
		} else if !reflect.DeepEqual(firstVal, val) {
			isMixed = true
			break
		}
	}

	focusedVal, hasFocused := rawValues[focusedID]
	if !hasFocused {
		focusedVal = firstVal
	}

	if !isMixed {
		// Single uniform value across all bots: return real value for focused bot
		return MixedValue{
			Mixed:  false,
			Value:  focusedVal,
			PerBot: nil,
		}
	}

	// Divergent values across bots
	perBot := make(map[string]any, len(rawValues))
	for id, val := range rawValues {
		if sensitive {
			perBot[id] = "***"
		} else {
			perBot[id] = val
		}
	}

	return MixedValue{
		Mixed:  true,
		Value:  nil, // Mixed state has no single value
		PerBot: perBot,
	}
}
