package llm

import "testing"

func TestTrySendRequest(t *testing.T) {
	client := NewGPTClient()
	msg := "please return the factorial of 3"

	resp, err := client.SendRequest(msg)
	if err != nil {
		t.Errorf("error: %v", err)
		return
	}

	t.Logf("response: %v", resp)
}
