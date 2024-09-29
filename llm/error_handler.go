package llm

import "errors"

func handleGPTErr(response any) error {
	resp := response.(*GPTResponse)

	if resp.Err != nil {
		return errors.New(resp.Err.Message)
	}

	return nil
}
