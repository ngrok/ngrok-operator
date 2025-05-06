package cmd

import (
	"os"

	"gopkg.in/yaml.v2"
)

func loadConfig(configPath string, destCfg any) error {
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	err = yaml.Unmarshal(configBytes, destCfg)
	if err != nil {
		return err
	}
	return nil
}
