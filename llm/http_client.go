package llm

import (
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/http"
	"time"
)

type HttpClient struct {
	client *resty.Client
}

func NewHttpClient() *HttpClient {
	return &HttpClient{
		client: resty.New().SetTimeout(time.Minute).SetRetryCount(3).SetRetryWaitTime(1 * time.Second).SetRetryMaxWaitTime(3 * time.Second),
	}
}

func (h *HttpClient) SetProxy(proxy string) {
	h.client.SetProxy(proxy)
}

func (h *HttpClient) Request(method, url string, headers map[string]string, params any) (*resty.Response, error) {
	req := h.client.R().SetHeaders(headers).SetBody(params)

	resp, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, resp.Error().(error)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.New(fmt.Sprintf("request llm error: %v %s", resp.StatusCode(), resp.Body()))
	}

	return resp, nil
}
