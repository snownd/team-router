package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestFetchUpstreamModelsOpenAI：Bearer 鉴权头 + data[].id 解析 + 字典序排序。
func TestFetchUpstreamModelsOpenAI(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"},{"id":""}]}`))
	}))
	defer srv.Close()

	ch := &Channel{Type: "openai", BaseURL: srv.URL, APIKey: "sk-test"}
	models, err := FetchUpstreamModels(context.Background(), NewOpenAIAdapter(), ch, srv.Client())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotPath != "/v1/models" {
		t.Errorf("path = %q, want /v1/models", gotPath)
	}
	if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "gpt-4o-mini" {
		t.Errorf("models = %v", models)
	}
}

// TestFetchUpstreamModelsAnthropic：x-api-key + anthropic-version 鉴权头。
func TestFetchUpstreamModelsAnthropic(t *testing.T) {
	var gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-5"},{"id":"claude-3-5-haiku-20241022"}]}`))
	}))
	defer srv.Close()

	ch := &Channel{Type: "anthropic", BaseURL: srv.URL, APIKey: "ak-test"}
	models, err := FetchUpstreamModels(context.Background(), NewAnthropicAdapter(), ch, srv.Client())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotKey != "ak-test" || gotVersion != "2023-06-01" {
		t.Errorf("x-api-key = %q, anthropic-version = %q", gotKey, gotVersion)
	}
	if len(models) != 2 || models[0] != "claude-3-5-haiku-20241022" || models[1] != "claude-sonnet-4-5" {
		t.Errorf("models = %v", models)
	}
}

// TestFetchUpstreamModelsError：上游 401 时错误信息透出上游 message。
func TestFetchUpstreamModelsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key provided","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	ch := &Channel{Type: "openai", BaseURL: srv.URL, APIKey: "bad"}
	_, err := FetchUpstreamModels(context.Background(), NewOpenAIAdapter(), ch, srv.Client())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Invalid API key provided") {
		t.Errorf("err = %v, want upstream message", err)
	}
}

// TestFetchUpstreamModelsBaseURLWithV1：base 已含 /v1 时不重复拼接。
func TestFetchUpstreamModelsBaseURLWithV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	ch := &Channel{Type: "openai", BaseURL: srv.URL + "/v1", APIKey: "sk"}
	if _, err := FetchUpstreamModels(context.Background(), NewOpenAIAdapter(), ch, srv.Client()); err != nil {
		t.Fatalf("err = %v", err)
	}
	if gotPath != "/v1/models" {
		t.Errorf("path = %q, want /v1/models", gotPath)
	}
}

// TestRouterFetchModelsUnsupportedType：未注册类型报错。
func TestRouterFetchModelsUnsupportedType(t *testing.T) {
	r := NewRouter(nil, nil, NewRegistry([]Adapter{NewOpenAIAdapter()}), zap.NewNop())
	_, err := r.FetchModels(context.Background(), &Channel{Type: "gemini", BaseURL: "https://x"}, http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "不支持的渠道类型") {
		t.Errorf("err = %v, want unsupported type error", err)
	}
}
