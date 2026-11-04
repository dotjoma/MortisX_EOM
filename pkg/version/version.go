package version

import "fmt"

// Rule of thumb:
// Leading number (v1, v2, v3) -> Malalaking update (major)
// Decimal (.0, .10, .25)       -> Mga bagong feature (minor)
// "p" suffix (p1, p2, p3...)  -> Maliliit na patch o bug fix (patch)
const (
	AppName = "Mortis"
	Major   = 1
	Minor   = 7
	Patch   = 2
)

// String returns formatted version string, e.g. "v1.0 p1"
func String() string {
	return fmt.Sprintf("v%d.%d p%d", Major, Minor, Patch)
}

// WindowTitle returns standard window title, e.g. "Mortis v1.0 p1"
func WindowTitle() string {
	return fmt.Sprintf("%s %s", AppName, String())
}

// SemverString returns semver formatted string, e.g. "1.0.1"
func SemverString() string {
	return fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)
}

// Details returns full version details map
func Details() map[string]interface{} {
	return map[string]interface{}{
		"app_name": AppName,
		"major":    Major,
		"minor":    Minor,
		"patch":    Patch,
		"version":  String(),
		"title":    WindowTitle(),
	}
}
