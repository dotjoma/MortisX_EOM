package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"madrigal-multibot/pkg/models"
)

func DefaultClassBuilds() models.StatsJsonMap {
	return models.StatsJsonMap{
		"mercenary": {
			Job:         "mercenary",
			Mode:        "cap",
			CapStat:     "dex",
			CapValue:    43,
			ThenStat:    "str",
			AutoStat:    1,
			Description: "43 DEX Cap (Hit Rate/Aspd threshold), then all STR",
		},
		"blade": {
			Job:         "blade",
			Mode:        "cap",
			CapStat:     "dex",
			CapValue:    43,
			ThenStat:    "str",
			AutoStat:    1,
			Description: "Classic 1v1 Meta: 43 DEX, Rest STR",
		},
		"knight": {
			Job:         "knight",
			Mode:        "pure",
			CapStat:     "sta",
			CapValue:    0,
			ThenStat:    "sta",
			AutoStat:    2,
			Description: "Pure STA (Mega HP Tank & AoE)",
		},
		"acrobat": {
			Job:         "acrobat",
			Mode:        "pure",
			CapStat:     "dex",
			CapValue:    0,
			ThenStat:    "dex",
			AutoStat:    3,
			Description: "Pure DEX (Max Bow & Yoyo Damage, Evasion)",
		},
		"jester": {
			Job:         "jester",
			Mode:        "pure",
			CapStat:     "dex",
			CapValue:    0,
			ThenStat:    "dex",
			AutoStat:    3,
			Description: "Pure DEX (100% Critical Hit Yoyo)",
		},
		"ranger": {
			Job:         "ranger",
			Mode:        "pure",
			CapStat:     "dex",
			CapValue:    0,
			ThenStat:    "dex",
			AutoStat:    3,
			Description: "Pure DEX (Bow AoE Spam)",
		},
		"assist": {
			Job:         "assist",
			Mode:        "pure",
			CapStat:     "int",
			CapValue:    0,
			ThenStat:    "int",
			AutoStat:    4,
			Description: "Pure INT (Max Buff Durations & Heals)",
		},
		"ringmaster": {
			Job:         "ringmaster",
			Mode:        "pure",
			CapStat:     "int",
			CapValue:    0,
			ThenStat:    "int",
			AutoStat:    4,
			Description: "Pure INT (Buffs, Holycross, Heal Potency)",
		},
		"billposter": {
			Job:         "billposter",
			Mode:        "pure",
			CapStat:     "str",
			CapValue:    0,
			ThenStat:    "str",
			AutoStat:    1,
			Description: "Pure STR (Fast 1v1 Solo Leveling)",
		},
		"magician": {
			Job:         "magician",
			Mode:        "pure",
			CapStat:     "int",
			CapValue:    0,
			ThenStat:    "int",
			AutoStat:    4,
			Description: "Pure INT (Spell Burst & Mana Pool)",
		},
		"psykeeper": {
			Job:         "psykeeper",
			Mode:        "pure",
			CapStat:     "int",
			CapValue:    0,
			ThenStat:    "int",
			AutoStat:    4,
			Description: "Pure INT (Spirit Bomb 1-Shot Burst)",
		},
		"elementor": {
			Job:         "elementor",
			Mode:        "pure",
			CapStat:     "int",
			CapValue:    0,
			ThenStat:    "int",
			AutoStat:    4,
			Description: "Pure INT (Elemental AoE Farming)",
		},
	}
}

func GetStatsJsonPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "stats.json", err
	}
	return filepath.Join(filepath.Dir(exePath), "stats.json"), nil
}

func EnsureStatsJsonExists() (models.StatsJsonMap, error) {
	targetPath, err := GetStatsJsonPath()
	if err != nil {
		targetPath = "stats.json"
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		defaults := DefaultClassBuilds()
		data, mErr := json.MarshalIndent(defaults, "", "  ")
		if mErr != nil {
			return defaults, mErr
		}
		_ = os.WriteFile(targetPath, data, 0644)
		return defaults, nil
	}

	return LoadStatsJson()
}

func LoadStatsJson() (models.StatsJsonMap, error) {
	targetPath, err := GetStatsJsonPath()
	if err != nil {
		targetPath = "stats.json"
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return DefaultClassBuilds(), err
	}

	var builds models.StatsJsonMap
	if err := json.Unmarshal(data, &builds); err != nil {
		return DefaultClassBuilds(), err
	}

	return builds, nil
}

func SaveStatsJson(cfg models.StatsJsonMap) error {
	targetPath, err := GetStatsJsonPath()
	if err != nil {
		targetPath = "stats.json"
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode stats.json: %w", err)
	}

	return os.WriteFile(targetPath, data, 0644)
}
