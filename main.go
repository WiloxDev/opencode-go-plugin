package main

/*
#include "cliproxy_plugin.h"

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const abiVersion uint32 = 1

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ModelInfo struct {
	ID                         string   `json:"id"`
	Object                     string   `json:"object,omitempty"`
	Created                    int64    `json:"created,omitempty"`
	OwnedBy                    string   `json:"owned_by,omitempty"`
	Type                       string   `json:"type,omitempty"`
	DisplayName                string   `json:"display_name,omitempty"`
	Name                       string   `json:"name,omitempty"`
	Version                    string   `json:"version,omitempty"`
	Description                string   `json:"description,omitempty"`
	ContextLength              int64    `json:"context_length,omitempty"`
	MaxCompletionTokens        int64    `json:"max_completion_tokens,omitempty"`
	SupportedGenerationMethods []string `json:"supported_generation_methods,omitempty"`
}

type modelRegistrationResult struct {
	Provider string      `json:"provider"`
	Models   []ModelInfo `json:"models"`
}

type startLoginResponse struct {
	LoginURL           string `json:"login_url"`
	State              string `json:"state"`
	OAuthCallbackLabel string `json:"oauth_callback_label,omitempty"`
	Instructions       string `json:"instructions,omitempty"`
}

type pollLoginRequest struct {
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
}

type pollLoginResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type authFileContent struct {
	Type           string    `json:"type"`
	APIKey         string    `json:"api_key"`
	Prefix         string    `json:"prefix"`
	Disabled       bool      `json:"disabled"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
	ExcludedModels []string  `json:"excluded_models,omitempty"`
}

var (
	logMu sync.Mutex

	baseOpenCodeModelIDs = []string{
		"deepseek-v4-flash",
		"deepseek-v4-flash-vision-exp",
		"deepseek-flash",
		"deepseek-v4.1-flash",
		"deepseek-v4-pro",
		"glm-5.2",
		"glm-5.3",
		"grok-4.6",
		"grok-4.7",
		"muse-spark-1.2-contributor",
		"muse-spark-1.3-contributor",
		"glm-5.3-flash",
		"gpt-5.6-luna",
		"gpt-6-luna",
		"hy3",
		"hy4-preview",
		"kimi-k2.7-code",
		"kimi-k3",
		"mimo-v2.5",
		"mimo-v2.6-flash",
		"mimo-v2.5-pro",
		"mimo-v2.6-pro",
		"minimax-m2.7",
		"minimax-m3",
		"space-bunny-free",
		"longcat-2.0",
		"longcat-2.5-preview-free",
		"qwen3.8-max",
		"qwen3.8-flash",
		"qwen3.7-plus",
	}
)

func logToFile(msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile("/tmp/opencode-go-plugin.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	f.WriteString(fmt.Sprintf("[%s] %s\n", ts, msg))
}

func getOpenCodeModelCatalog() []ModelInfo {
	models := make([]ModelInfo, 0, len(baseOpenCodeModelIDs))
	for _, id := range baseOpenCodeModelIDs {
		models = append(models, ModelInfo{
			ID:                  id,
			Name:                id,
			DisplayName:         id,
			Type:                "opencode-go",
			OwnedBy:             "opencode",
			Object:              "model",
			Created:             1790587280,
			ContextLength:       1048576,
			MaxCompletionTokens: 64000,
			SupportedGenerationMethods: []string{
				"generateContent",
				"chat",
			},
		})
	}
	return models
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	logToFile("cliproxy_plugin_init called")
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	logToFile("cliproxy_plugin_init finished successfully")
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) (rc C.int) {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	defer func() {
		if r := recover(); r != nil {
			logToFile(fmt.Sprintf("PANIC recovered in cliproxyPluginCall: %v", r))
			if response != nil && response.ptr != nil {
				C.free(response.ptr)
				response.ptr = nil
				response.len = 0
			}
			writeResponse(response, errorEnvelope("plugin_panic", fmt.Sprintf("recovered: %v", r)))
			rc = 1
		}
	}()

	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method required"))
		return 1
	}

	methodStr := C.GoString(method)
	var reqBytes []byte
	if request != nil && requestLen > 0 {
		reqBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}

	logToFile(fmt.Sprintf("CALL method=%s reqLen=%d req=%s", methodStr, len(reqBytes), string(reqBytes)))

	raw, err := handlePluginMethod(methodStr, reqBytes)
	if err != nil {
		logToFile(fmt.Sprintf("ERROR method=%s: %v", methodStr, err))
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}

	logToFile(fmt.Sprintf("REPLY method=%s bytes=%d", methodStr, len(raw)))
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = len
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	logToFile("cliproxyPluginShutdown called")
}

func handlePluginMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		registration := map[string]any{
			"schema_version": 6,
			"metadata": map[string]any{
				"Name":             "opencode-go",
				"Version":          "1.0.0",
				"Author":           "Wilox",
				"Description":      "OpenCode Go provider and model catalog integration",
				"GitHubRepository": "https://github.com/opencode",
				"Logo":             "",
				"ConfigFields":     []any{},
			},
			"capabilities": map[string]any{
				"model_registrar": true,
				"model_provider":  true,
				"auth_provider":   true,
			},
		}
		raw, err := json.Marshal(registration)
		if err != nil {
			return nil, err
		}
		return okEnvelope(raw), nil

	case "model.register", "model.static", "model.for_auth":
		result := modelRegistrationResult{
			Provider: "opencode-go",
			Models:   getOpenCodeModelCatalog(),
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		return okEnvelope(raw), nil

	case "auth.start_login", "start_login":
		resp := startLoginResponse{
			LoginURL:           "",
			State:              "waiting_for_key",
			OAuthCallbackLabel: "Paste your OpenCode Go API key (oc_sk_...)",
			Instructions:       "1. Go to https://opencode.ai -> Settings -> API Keys\n2. Create or copy your key (starts with oc_sk_)\n3. Paste it in the field below and click Confirm",
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		return okEnvelope(raw), nil

	case "auth.poll_login", "poll_login":
		var req pollLoginRequest
		if len(request) > 0 {
			_ = json.Unmarshal(request, &req)
		}
		apiKey := strings.TrimSpace(req.Code)
		if apiKey == "" {
			raw, _ := json.Marshal(pollLoginResponse{Status: "pending"})
			return okEnvelope(raw), nil
		}
		raw, _ := json.Marshal(pollLoginResponse{Status: "complete"})
		return okEnvelope(raw), nil

	case "auth.parse", "parse_auth":
		res := map[string]any{
			"provider": "opencode-go",
			"status":   "active",
		}
		raw, _ := json.Marshal(res)
		return okEnvelope(raw), nil

	default:
		logToFile(fmt.Sprintf("unhandled method: %s", method))
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func okEnvelope(result []byte) []byte {
	raw, _ := json.Marshal(envelope{OK: true, Result: json.RawMessage(result)})
	return raw
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func main() {
	// Empty main required for c-shared library
}
