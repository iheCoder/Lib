package llm

type GPTRole string
type GPTModel string

const (
	GPTFree GPTModel = "gpt-3.5-turbo"

	GPTRoleUser GPTRole = "user"
)

type GPTError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

type GPTRequest struct {
	Model       GPTModel  `json:"model"`
	Message     []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float32   `json:"temperature"`
	TopP        float32   `json:"top_p"`
}

type Message struct {
	Role    GPTRole `json:"role"`
	Content string  `json:"content"`
}

type GPTResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   GPTModel  `json:"model"`
	Choices []Choice  `json:"choices"`
	Err     *GPTError `json:"error"`
}

type Choice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Delta struct {
	Content string `json:"content"`
}
