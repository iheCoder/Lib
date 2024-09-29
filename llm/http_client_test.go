package llm

import "testing"

func TestHttpClient_Request(t *testing.T) {
	client := NewHttpClient()
	proxy := ""
	client.SetProxy(proxy)

	// request google
	resp, err := client.Request("GET", "https://www.google.com", nil, nil)
	if err != nil {
		t.Errorf("request error: %v", err)
		return
	}

	if len(resp.Body()) == 0 {
		t.Errorf("empty response")
		return
	}
}
