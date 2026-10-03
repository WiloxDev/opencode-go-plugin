package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPluginRegistration(t *testing.T) {
	raw, err := handlePluginMethod("plugin.register", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected env.OK to be true")
	}

	var reg struct {
		SchemaVersion int             `json:"schema_version"`
		Metadata      map[string]any  `json:"metadata"`
		Capabilities  map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("failed to unmarshal registration: %v", err)
	}

	if reg.Metadata["Name"] != "opencode-go" {
		t.Errorf("expected Name 'opencode-go', got '%v'", reg.Metadata["Name"])
	}
	if reg.Metadata["GitHubRepository"] != "https://github.com/WiloxDev/opencode-go-plugin" {
		t.Errorf("expected GitHubRepository to match WiloxDev repo, got '%v'", reg.Metadata["GitHubRepository"])
	}
	if !reg.Capabilities["model_provider"] || !reg.Capabilities["auth_provider"] {
		t.Errorf("expected model_provider and auth_provider capabilities to be true")
	}
}

func TestModelCatalog(t *testing.T) {
	raw, err := handlePluginMethod("model.register", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected env.OK to be true")
	}

	var res modelRegistrationResult
	if err := json.Unmarshal(env.Result, &res); err != nil {
		t.Fatalf("failed to unmarshal modelRegistrationResult: %v", err)
	}

	if res.Provider != "opencode-go" {
		t.Errorf("expected Provider 'opencode-go', got '%s'", res.Provider)
	}
	if len(res.Models) != len(baseOpenCodeModelIDs) {
		t.Errorf("expected %d models, got %d", len(baseOpenCodeModelIDs), len(res.Models))
	}

	foundDeepSeek := false
	for _, m := range res.Models {
		if m.ID == "deepseek-v4-pro" {
			foundDeepSeek = true
			if m.ContextLength != 1048576 {
				t.Errorf("expected context length 1048576, got %d", m.ContextLength)
			}
			break
		}
	}
	if !foundDeepSeek {
		t.Errorf("deepseek-v4-pro not found in catalog")
	}
}

func TestAuthFlowMultiAccount(t *testing.T) {
	// 1. Start login
	raw, err := handlePluginMethod("auth.start_login", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("failed unmarshal: %v", err)
	}
	var start startLoginResponse
	if err := json.Unmarshal(env.Result, &start); err != nil {
		t.Fatalf("failed unmarshal startLoginResponse: %v", err)
	}
	if start.State != "waiting_for_key" {
		t.Errorf("expected State 'waiting_for_key', got '%s'", start.State)
	}

	// 2. Poll login pending
	rawPollPending, err := handlePluginMethod("auth.poll_login", []byte(`{"state":"waiting_for_key","code":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envPending envelope
	json.Unmarshal(rawPollPending, &envPending)
	var pollPending pollLoginResponse
	json.Unmarshal(envPending.Result, &pollPending)
	if pollPending.Status != "pending" {
		t.Errorf("expected status 'pending', got '%s'", pollPending.Status)
	}

	// 3. Poll login validation error (too short)
	rawPollShort, err := handlePluginMethod("auth.poll_login", []byte(`{"state":"waiting_for_key","code":"short"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envShort envelope
	json.Unmarshal(rawPollShort, &envShort)
	var pollShort pollLoginResponse
	json.Unmarshal(envShort.Result, &pollShort)
	if pollShort.Status != "error" {
		t.Errorf("expected status 'error' for invalid key, got '%s'", pollShort.Status)
	}

	// 4. Poll login complete with valid key
	sampleKey := "oc_sk_a664a43e0e73_Wyq2BJXXjusBKvLZeLL8e7YdKStNy0T7"
	rawPollComplete, err := handlePluginMethod("auth.poll_login", []byte(`{"state":"waiting_for_key","code":"`+sampleKey+`"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envComplete envelope
	json.Unmarshal(rawPollComplete, &envComplete)
	var pollComplete pollLoginResponse
	json.Unmarshal(envComplete.Result, &pollComplete)
	if pollComplete.Status != "complete" {
		t.Errorf("expected status 'complete', got '%s'", pollComplete.Status)
	}

	// 5. Parse auth with custom label (multi-account identification)
	reqParseWithLabel := []byte(`{"api_key":"` + sampleKey + `","label":"OpenCode Cuenta Wilox 1"}`)
	rawParse1, err := handlePluginMethod("auth.parse", reqParseWithLabel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envParse1 envelope
	json.Unmarshal(rawParse1, &envParse1)
	var parsed1 map[string]any
	json.Unmarshal(envParse1.Result, &parsed1)
	if parsed1["label"] != "OpenCode Cuenta Wilox 1" {
		t.Errorf("expected custom label to be preserved, got '%v'", parsed1["label"])
	}

	// 6. Parse auth without label (auto-derived truncated label)
	reqParseNoLabel := []byte(`{"api_key":"` + sampleKey + `"}`)
	rawParse2, err := handlePluginMethod("auth.parse", reqParseNoLabel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envParse2 envelope
	json.Unmarshal(rawParse2, &envParse2)
	var parsed2 map[string]any
	json.Unmarshal(envParse2.Result, &parsed2)
	if !strings.HasPrefix(parsed2["label"].(string), "OpenCode Go (oc_sk_a6...") {
		t.Errorf("expected auto truncated label, got '%v'", parsed2["label"])
	}
}
