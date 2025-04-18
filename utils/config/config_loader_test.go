package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"testing"
)

func TestHandleEvents(t *testing.T) {
	// new config loader, and start to watch the config file
	path := "test_config.yaml"
	loader, err := NewConfigLoader(path)
	if err != nil {
		t.Error(err)
		return
	}

	// write the config file
	if err = changeConfigVersion(path, "1.0.1"); err != nil {
		t.Error(err)
		return
	}

	// check the version
	// the version should be 1
	if loader.version != 1 {
		t.Errorf("version should be 1, but got %d", loader.version)
		return
	}
}

type TestConfig struct {
	Version string `yaml:"version"`
}

func changeConfigVersion(path string, version string) error {
	// read the config file
	d, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// parse yaml file
	var config TestConfig
	if err := yaml.Unmarshal(d, &config); err != nil {
		return err
	}

	// change the version
	config.Version = version

	// write the config file
	d, err = yaml.Marshal(&config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, d, 0644)
}
