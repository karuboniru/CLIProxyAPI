package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutorNativeResponsesNonStream(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","model":"upstream-model","output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatible-test", &config.Config{})
	auth := openAICompatResponsesTestAuth(server.URL)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-model",
		Payload: []byte(`{"model":"public-model","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat:   sdktranslator.FormatOpenAIResponse,
		ResponseFormat: sdktranslator.FormatOpenAIResponse,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if got := gjson.GetBytes(gotBody, "model").String(); got != "upstream-model" {
		t.Fatalf("model = %q, want upstream-model; body=%s", got, gotBody)
	}
	if !gjson.GetBytes(gotBody, "input").Exists() || gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("expected Responses request body, got %s", gotBody)
	}
	if got := gjson.GetBytes(resp.Payload, "object").String(); got != "response" {
		t.Fatalf("response object = %q; payload=%s", got, resp.Payload)
	}
}

func TestOpenAICompatExecutorResponsesDisabledUsesChatCompletions(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatible-test", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{"base_url": server.URL + "/v1"}}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-model",
		Payload: []byte(`{"model":"public-model","input":"hello"}`),
	}, cliproxyexecutor.Options{
		SourceFormat:   sdktranslator.FormatOpenAIResponse,
		ResponseFormat: sdktranslator.FormatOpenAIResponse,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
	if !gjson.GetBytes(gotBody, "messages").Exists() || gjson.GetBytes(gotBody, "input").Exists() {
		t.Fatalf("expected Chat Completions request body, got %s", gotBody)
	}
	if got := gjson.GetBytes(resp.Payload, "object").String(); got != "response" {
		t.Fatalf("response object = %q; payload=%s", got, resp.Payload)
	}
}

func TestOpenAICompatExecutorNativeResponsesStream(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatible-test", &config.Config{})
	auth := openAICompatResponsesTestAuth(server.URL)
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-model",
		Payload: []byte(`{"model":"public-model","input":"hello","stream":true}`),
	}, cliproxyexecutor.Options{
		Stream:         true,
		SourceFormat:   sdktranslator.FormatOpenAIResponse,
		ResponseFormat: sdktranslator.FormatOpenAIResponse,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var got strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		got.Write(chunk.Payload)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gjson.GetBytes(gotBody, "stream_options").Exists() {
		t.Fatalf("unexpected stream_options in body: %s", gotBody)
	}
	if !strings.Contains(got.String(), "event: response.output_text.delta") || !strings.Contains(got.String(), "event: response.completed") {
		t.Fatalf("native Responses events not preserved: %q", got.String())
	}
	if strings.Contains(got.String(), "[DONE]") {
		t.Fatalf("unexpected synthetic done marker: %q", got.String())
	}
}

func TestOpenAICompatTargetKeepsChatCompletionsWhenResponsesSupported(t *testing.T) {
	auth := &cliproxyauth.Auth{Attributes: map[string]string{cliproxyauth.AttributeSupportsResponsesAPI: "true"}}
	target := openAICompatTargetForRequest(auth, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAI})
	if target.endpoint != openAICompatChatCompletionsPath || target.format != sdktranslator.FormatOpenAI {
		t.Fatalf("target = %+v, want Chat Completions", target)
	}

	executor := NewOpenAICompatExecutor("openai-compatible-test", &config.Config{})
	if got := executor.RequestToFormatForAuth(auth, cliproxyexecutor.Request{}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse}); got != sdktranslator.FormatOpenAIResponse {
		t.Fatalf("RequestToFormatForAuth = %q, want %q", got, sdktranslator.FormatOpenAIResponse)
	}
}

func openAICompatResponsesTestAuth(serverURL string) *cliproxyauth.Auth {
	return &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": serverURL + "/v1",
		cliproxyauth.AttributeSupportsResponsesAPI: "true",
	}}
}
