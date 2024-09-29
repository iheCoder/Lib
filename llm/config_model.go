package llm

type LLMConfig struct {
	ChatGPT LLMItem `json:"chatgpt" yaml:"chatgpt"`
}

type LLMItem struct {
	Url string `json:"url" yaml:"url"`
	Key string `json:"key" yaml:"key"`
}
