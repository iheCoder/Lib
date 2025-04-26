package knowbase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type EmbedRequest struct {
	Input []string `json:"input"`
}

type EmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

func generateLocalEmbedding(texts []string) ([][]float64, error) {
	url := "http://localhost:8000/embed"

	reqBody := EmbedRequest{
		Input: texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bs, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP error: %d, body: %s", resp.StatusCode, string(bs))
	}

	var respData EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, err
	}

	return respData.Embeddings, nil
}
