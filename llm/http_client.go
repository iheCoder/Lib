package llm

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/go-resty/resty/v2"
)

var (
	defaultProxy = "socks5://127.0.0.1:7890"

	ErrEmptyResponse = errors.New("empty response")
	ErrStatusCode    = errors.New("request llm status code error")
)

type ErrHandler func(a any) error

type HttpClient struct {
	client      *resty.Client
	errHandlers map[reflect.Type]ErrHandler
}

func NewHttpClient() *HttpClient {
	return &HttpClient{
		client: resty.New().
			SetTimeout(time.Minute).
			SetRetryCount(3).
			SetRetryWaitTime(1 * time.Second).
			SetRetryMaxWaitTime(3 * time.Second),
		errHandlers: make(map[reflect.Type]ErrHandler),
	}
}

func (h *HttpClient) EnableDefaultProxy() *HttpClient {
	h.client.SetProxy(defaultProxy)
	return h
}

func (h *HttpClient) SetProxy(proxy string) {
	h.client.SetProxy(proxy)
}

func (h *HttpClient) RegisterErrHandler(resp any, handler ErrHandler) {
	h.errHandlers[reflect.TypeOf(resp)] = handler
}

func (h *HttpClient) request(method, url string, headers map[string]string, params any) (*resty.Response, error) {
	req := h.client.R().SetHeaders(headers).SetBody(params)

	resp, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *HttpClient) RequestWithResp(method, url string, headers map[string]string, params, response any) error {
	resp, err := h.request(method, url, headers, params)
	if err != nil {
		return err
	}

	err = json.Unmarshal(resp.Body(), response)
	if err != nil {
		return err
	}

	return h.handleError(response)
}

func (h *HttpClient) handleError(resp any) error {
	if len(h.errHandlers) == 0 {
		return nil
	}

	errType := reflect.TypeOf(resp)
	handler, ok := h.errHandlers[errType]
	if !ok {
		return nil
	}

	return handler(resp)
}
