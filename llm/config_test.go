package llm

import "testing"

func TestLoadConfig(t *testing.T) {
	conf, err := LoadConfig()
	if err != nil {
		t.Errorf("load config error: %v", err)
		return
	}

	if conf.ChatGPT.Url != "https://api.openai.com/v1/chat/completions" || conf.ChatGPT.Key != "test-key" {
		t.Errorf("invalid config")
		return
	}
}
