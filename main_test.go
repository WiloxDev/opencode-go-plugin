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
	if !reg.Capabilities["management_api"] {
		t.Errorf("expected management_api capability to be true")
	}
}

func TestManagementRegisterAndAPI(t *testing.T) {
	// Test management.register
	raw, err := handlePluginMethod("management.register", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env envelope
	json.Unmarshal(raw, &env)
	var reg struct {
		Resources []struct {
			Path string `json:"Path"`
			Menu string `json:"Menu"`
		} `json:"resources"`
	}
	json.Unmarshal(env.Result, &reg)
	if len(reg.Resources) == 0 || reg.Resources[0].Path != "/accounts" {
		t.Errorf("expected /accounts resource in management.register")
	}

	// Test GET UI HTML
	reqUI, _ := json.Marshal(ManagementRequestPayload{
		Method: "GET",
		Path:   "/accounts",
	})
	rawUI, err := handlePluginMethod("management.handle", reqUI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envUI envelope
	json.Unmarshal(rawUI, &envUI)
	var respUI ManagementResponsePayload
	json.Unmarshal(envUI.Result, &respUI)
	if respUI.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", respUI.StatusCode)
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
}

func TestAuthFlowMultiAccount(t *testing.T) {
	sampleKey := "oc_sk_a664a43e0e73_Wyq2BJXXjusBKvLZeLL8e7YdKStNy0T7"

	// Parse auth with custom label
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

	// Parse auth without label
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
