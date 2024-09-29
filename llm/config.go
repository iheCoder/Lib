package llm

import (
	"gopkg.in/yaml.v3"
	"os"
)

var (
	localConfigPath = "config.yaml"
)

func LoadConfig() (LLMConfig, error) {
	// read local config file
	data, err := os.ReadFile(localConfigPath)
	if err != nil {
		return LLMConfig{}, err
	}

	// unmarshal config
	var config LLMConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return LLMConfig{}, err
	}

	return config, nil
}
