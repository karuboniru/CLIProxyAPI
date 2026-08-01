package config

import "testing"

func TestParseConfigBytesOpenAICompatSupportsResponsesAPI(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte(`
openai-compatibility:
  - name: compat
    base-url: https://example.com/v1
    supports-responses-api: true
    models:
      - name: upstream-model
        alias: public-model
`))
	if err != nil {
		t.Fatalf("ParseConfigBytes error: %v", err)
	}
	if len(cfg.OpenAICompatibility) != 1 {
		t.Fatalf("openai compatibility entries = %d, want 1", len(cfg.OpenAICompatibility))
	}
	if !cfg.OpenAICompatibility[0].SupportsResponsesAPI {
		t.Fatal("supports-responses-api = false, want true")
	}
}
