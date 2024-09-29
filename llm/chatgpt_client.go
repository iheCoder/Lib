package llm

import (
	"errors"
)

const (
	apiURL = "https://api.openai.com/v1/chat/completions"
	apiKey = ""
)

type GPTClient struct {
	client *HttpClient
}

func NewGPTClient() *GPTClient {
	client := NewHttpClient().EnableDefaultProxy()
	client.RegisterErrHandler(&GPTResponse{}, handleGPTErr)

	return &GPTClient{
		client: client,
	}
}

func (g *GPTClient) SendRequest(msg string) (string, error) {
	req := makeGPTRequest(msg)
	header := makeGPTHeader()

	var resp GPTResponse
	err := g.client.RequestWithResp("POST", apiURL, header, req, &resp)
	if err != nil {
		return "", err
	}

	if resp.Err != nil {

	}

	return g.HandleNonStreamResponse(&resp)
}

func makeGPTRequest(msg string) GPTRequest {
	return GPTRequest{
		Model:  GPTFree,
		Stream: false,
		Message: []Message{
			{
				Role:    GPTRoleUser,
				Content: msg,
			},
		},
	}
}

func makeGPTHeader() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Content-Type":  "application/json",
	}
}

func (g *GPTClient) HandleNonStreamResponse(response *GPTResponse) (string, error) {
	if len(response.Choices) == 0 {
		return "", errors.New("empty choices")
	}

	return response.Choices[0].Message.Content, nil
}
