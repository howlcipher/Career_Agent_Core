package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func AcknowledgeActiveSettings(eff *EffectiveSettings, path string) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0700)

	data, err := json.Marshal(eff)
	if err != nil {
		return err
	}
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile)
		return err
	}
	return nil
}

func LoadActiveSettings(path string) (*EffectiveSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var eff EffectiveSettings
	if err := json.Unmarshal(data, &eff); err != nil {
		return nil, err
	}
	return &eff, nil
}
