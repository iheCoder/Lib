package knowbase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

const embeddingModel = "text-embedding-3-small"

var (
	apiKey = os.Getenv("OPENAI_API_KEY")
)

type openAIEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// 调用 OpenAI Embedding API
func generateEmbedding(text string) ([]float64, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing OPENAI_API_KEY")
	}

	requestBody := openAIEmbeddingRequest{
		Input: text,
		Model: embeddingModel,
	}
	bodyBytes, _ := json.Marshal(requestBody)

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI error: %s", respBody)
	}

	var response openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	return response.Data[0].Embedding, nil
}
