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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

type ManagementRequestPayload struct {
	Method  string              `json:"Method"`
	Path    string              `json:"Path"`
	Headers map[string][]string `json:"Headers"`
	Query   map[string][]string `json:"Query"`
	Body    []byte              `json:"Body"`
}

type ManagementResponsePayload struct {
	StatusCode int                 `json:"StatusCode"`
	Headers    map[string][]string `json:"Headers"`
	Body       string              `json:"Body"` // Base64 encoded payload
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
			OwnedBy:             "opencode-go",
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

func getAuthDir() string {
	candidates := []string{
		"/root/.cli-proxy-api",
		filepath.Join(os.Getenv("HOME"), ".cli-proxy-api"),
		".cli-proxy-api",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return candidates[0]
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

	logToFile(fmt.Sprintf("CALL method=%s reqLen=%d", methodStr, len(reqBytes)))

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
				"Author":           "Wilox <wilsonlavio9@gmail.com>",
				"Description":      "OpenCode Go provider, multi-account pool and model catalog integration",
				"GitHubRepository": "https://github.com/WiloxDev/opencode-go-plugin",
				"Logo":             "",
				"ConfigFields":     []any{},
			},
			"capabilities": map[string]any{
				"model_registrar": true,
				"model_provider":  true,
				"auth_provider":   true,
				"management_api":  true,
			},
		}
		raw, err := json.Marshal(registration)
		if err != nil {
			return nil, err
		}
		return okEnvelope(raw), nil

	case "management.register":
		regResponse := map[string]any{
			"resources": []map[string]any{
				{
					"Path":        "/accounts",
					"Menu":        "🔑 OpenCode Accounts",
					"Description": "Gestor y creador de múltiples cuentas de OpenCode Go para CPAMC",
				},
			},
		}
		raw, err := json.Marshal(regResponse)
		if err != nil {
			return nil, err
		}
		return okEnvelope(raw), nil

	case "management.handle":
		return handleManagementHTTP(request)

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
			OAuthCallbackLabel: "Enter OpenCode API Key or Account Key (oc_sk_...):",
			Instructions:       "1. Obtain your API key from OpenCode (https://opencode.ai) or your WiloxDev License Portal.\n2. Paste your key below to add and bind this account session to CPAMC.\n3. You can add multiple accounts by repeating this process in CPAMC.",
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

		if len(apiKey) < 10 {
			raw, _ := json.Marshal(pollLoginResponse{
				Status: "error",
				Error:  "Invalid key format: key must be a valid OpenCode or subscription key",
			})
			return okEnvelope(raw), nil
		}

		raw, _ := json.Marshal(pollLoginResponse{Status: "complete"})
		return okEnvelope(raw), nil

	case "auth.parse", "parse_auth":
		var parsed map[string]any
		if len(request) > 0 {
			_ = json.Unmarshal(request, &parsed)
		}

		label := "OpenCode Go"
		if rawLabel, ok := parsed["label"].(string); ok && strings.TrimSpace(rawLabel) != "" {
			label = strings.TrimSpace(rawLabel)
		} else if rawKey, ok := parsed["api_key"].(string); ok && len(rawKey) > 16 {
			label = fmt.Sprintf("OpenCode Go (%s...%s)", rawKey[:8], rawKey[len(rawKey)-4:])
		}

		res := map[string]any{
			"provider": "opencode-go",
			"status":   "active",
			"label":    label,
			"prefix":   "ocgo",
		}
		raw, _ := json.Marshal(res)
		return okEnvelope(raw), nil

	default:
		logToFile(fmt.Sprintf("unhandled method: %s", method))
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handleManagementHTTP(request []byte) ([]byte, error) {
	var req ManagementRequestPayload
	if len(request) > 0 {
		_ = json.Unmarshal(request, &req)
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	path := strings.TrimSpace(req.Path)
	if path == "" || path == "/" {
		path = "/accounts"
	}

	// API Endpoints
	if strings.HasPrefix(path, "/api/accounts") || strings.HasPrefix(path, "/accounts/api") {
		return handleAccountsAPI(method, req)
	}

	// Render Management UI
	html := getAccountsHTML()
	resp := ManagementResponsePayload{
		StatusCode: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type": {"text/html; charset=utf-8"},
		},
		Body: base64.StdEncoding.EncodeToString([]byte(html)),
	}
	raw, _ := json.Marshal(resp)
	return okEnvelope(raw), nil
}

type accountItem struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	KeyMask   string `json:"key_mask"`
	Prefix    string `json:"prefix"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"created_at"`
}

func handleAccountsAPI(method string, req ManagementRequestPayload) ([]byte, error) {
	authDir := getAuthDir()

	switch method {
	case http.MethodGet:
		// List OpenCode accounts
		files, _ := os.ReadDir(authDir)
		accounts := []accountItem{}
		for _, f := range files {
			if !f.IsDir() && strings.HasPrefix(f.Name(), "opencode-go") && strings.HasSuffix(f.Name(), ".json") {
				content, err := os.ReadFile(filepath.Join(authDir, f.Name()))
				if err != nil {
					continue
				}
				var item map[string]any
				if err := json.Unmarshal(content, &item); err == nil {
					label, _ := item["label"].(string)
					apiKey, _ := item["api_key"].(string)
					disabled, _ := item["disabled"].(bool)
					prefix, _ := item["prefix"].(string)
					createdAt, _ := item["created_at"].(string)

					keyMask := "oc_sk_***"
					if len(apiKey) > 16 {
						keyMask = fmt.Sprintf("%s...%s", apiKey[:8], apiKey[len(apiKey)-4:])
					}

					accounts = append(accounts, accountItem{
						ID:        f.Name(),
						Label:     label,
						KeyMask:   keyMask,
						Prefix:    prefix,
						Disabled:  disabled,
						CreatedAt: createdAt,
					})
				}
			}
		}

		resBytes, _ := json.Marshal(map[string]any{
			"ok":       true,
			"accounts": accounts,
		})
		resp := ManagementResponsePayload{
			StatusCode: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type": {"application/json; charset=utf-8"},
			},
			Body: base64.StdEncoding.EncodeToString(resBytes),
		}
		raw, _ := json.Marshal(resp)
		return okEnvelope(raw), nil

	case http.MethodPost:
		// Add new account
		var input struct {
			Label  string `json:"label"`
			APIKey string `json:"api_key"`
		}
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &input)
		}

		key := strings.TrimSpace(input.APIKey)
		if len(key) < 10 {
			resBytes, _ := json.Marshal(map[string]any{"ok": false, "error": "API Key inválida o muy corta"})
			resp := ManagementResponsePayload{
				StatusCode: http.StatusBadRequest,
				Headers:    map[string][]string{"Content-Type": {"application/json"}},
				Body:       base64.StdEncoding.EncodeToString(resBytes),
			}
			raw, _ := json.Marshal(resp)
			return okEnvelope(raw), nil
		}

		label := strings.TrimSpace(input.Label)
		if label == "" {
			label = "OpenCode Go Account"
		}

		// Calculate deterministic file ID
		hash := sha256.Sum256([]byte(key))
		accountID := fmt.Sprintf("opencode-go-%s.json", hex.EncodeToString(hash[:6]))

		accountData := map[string]any{
			"type":         "opencode-go",
			"provider":     "opencode-go",
			"label":        label,
			"api_key":      key,
			"access_token": key,
			"base_url":     "https://opencode.ai/zen/go/v1",
			"prefix":       "ocgo",
			"disabled":     false,
			"created_at":   time.Now().UTC().Format(time.RFC3339),
			"fields": map[string]any{
				"provider_url": "https://opencode.ai/zen/go/v1",
			},
		}

		encoded, _ := json.MarshalIndent(accountData, "", "  ")
		targetPath := filepath.Join(authDir, accountID)
		if err := os.WriteFile(targetPath, encoded, 0644); err != nil {
			resBytes, _ := json.Marshal(map[string]any{"ok": false, "error": "No se pudo guardar la cuenta: " + err.Error()})
			resp := ManagementResponsePayload{
				StatusCode: http.StatusInternalServerError,
				Headers:    map[string][]string{"Content-Type": {"application/json"}},
				Body:       base64.StdEncoding.EncodeToString(resBytes),
			}
			raw, _ := json.Marshal(resp)
			return okEnvelope(raw), nil
		}

		resBytes, _ := json.Marshal(map[string]any{"ok": true, "id": accountID, "message": "Cuenta creada con éxito"})
		resp := ManagementResponsePayload{
			StatusCode: http.StatusOK,
			Headers:    map[string][]string{"Content-Type": {"application/json"}},
			Body:       base64.StdEncoding.EncodeToString(resBytes),
		}
		raw, _ := json.Marshal(resp)
		return okEnvelope(raw), nil

	case http.MethodDelete:
		// Delete account
		var input struct {
			ID string `json:"id"`
		}
		if len(req.Body) > 0 {
			_ = json.Unmarshal(req.Body, &input)
		}
		id := filepath.Clean(strings.TrimSpace(input.ID))
		if id == "" || !strings.HasPrefix(id, "opencode-go") || !strings.HasSuffix(id, ".json") {
			resBytes, _ := json.Marshal(map[string]any{"ok": false, "error": "ID de cuenta inválido"})
			resp := ManagementResponsePayload{
				StatusCode: http.StatusBadRequest,
				Headers:    map[string][]string{"Content-Type": {"application/json"}},
				Body:       base64.StdEncoding.EncodeToString(resBytes),
			}
			raw, _ := json.Marshal(resp)
			return okEnvelope(raw), nil
		}

		targetPath := filepath.Join(authDir, id)
		_ = os.Remove(targetPath)

		resBytes, _ := json.Marshal(map[string]any{"ok": true, "message": "Cuenta eliminada con éxito"})
		resp := ManagementResponsePayload{
			StatusCode: http.StatusOK,
			Headers:    map[string][]string{"Content-Type": {"application/json"}},
			Body:       base64.StdEncoding.EncodeToString(resBytes),
		}
		raw, _ := json.Marshal(resp)
		return okEnvelope(raw), nil
	}

	resp := ManagementResponsePayload{
		StatusCode: http.StatusMethodNotAllowed,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		Body:       base64.StdEncoding.EncodeToString([]byte(`{"error":"method_not_allowed"}`)),
	}
	raw, _ := json.Marshal(resp)
	return okEnvelope(raw), nil
}

func getAccountsHTML() string {
	return `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8">
  <title>OpenCode Multi-Account Manager - CPAMC</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    :root {
      --bg: #0f172a;
      --card-bg: #1e293b;
      --text: #f8fafc;
      --muted: #94a3b8;
      --primary: #3b82f6;
      --primary-hover: #2563eb;
      --danger: #ef4444;
      --danger-hover: #dc2626;
      --success: #10b981;
      --border: #334155;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; }
    body { background-color: var(--bg); color: var(--text); padding: 24px; }
    .header { margin-bottom: 24px; border-bottom: 1px solid var(--border); padding-bottom: 16px; }
    .header h1 { font-size: 1.5rem; font-weight: 700; display: flex; align-items: center; gap: 8px; }
    .header p { color: var(--muted); font-size: 0.875rem; margin-top: 4px; }
    .grid { display: grid; grid-template-columns: 1fr 2fr; gap: 24px; }
    @media (max-width: 800px) { .grid { grid-template-columns: 1fr; } }
    .card { background: var(--card-bg); border: 1px solid var(--border); border-radius: 8px; padding: 20px; }
    .card h2 { font-size: 1.1rem; margin-bottom: 16px; font-weight: 600; }
    .form-group { margin-bottom: 14px; }
    .form-group label { display: block; font-size: 0.85rem; color: var(--muted); margin-bottom: 6px; }
    .form-group input { width: 100%; background: #0b1120; border: 1px solid var(--border); border-radius: 6px; padding: 10px 12px; color: #fff; font-size: 0.9rem; }
    .form-group input:focus { outline: none; border-color: var(--primary); }
    .btn { background: var(--primary); color: #fff; border: none; border-radius: 6px; padding: 10px 16px; font-weight: 500; font-size: 0.9rem; cursor: pointer; transition: 0.2s; width: 100%; }
    .btn:hover { background: var(--primary-hover); }
    .btn-danger { background: var(--danger); width: auto; padding: 6px 12px; font-size: 0.8rem; }
    .btn-danger:hover { background: var(--danger-hover); }
    .account-card { background: #0b1120; border: 1px solid var(--border); border-radius: 6px; padding: 14px; margin-bottom: 12px; display: flex; justify-content: space-between; align-items: center; }
    .account-info h3 { font-size: 0.95rem; font-weight: 600; display: flex; align-items: center; gap: 6px; }
    .account-info p { font-size: 0.8rem; color: var(--muted); font-family: monospace; margin-top: 4px; }
    .badge { background: rgba(16, 185, 129, 0.2); color: var(--success); font-size: 0.75rem; padding: 2px 6px; border-radius: 4px; font-weight: 600; }
    .models-hint { font-size: 0.75rem; color: var(--muted); margin-top: 6px; }
    .alert { padding: 10px; border-radius: 6px; font-size: 0.85rem; margin-bottom: 14px; display: none; }
    .alert-success { background: rgba(16, 185, 129, 0.2); color: var(--success); border: 1px solid var(--success); }
    .alert-error { background: rgba(239, 68, 68, 0.2); color: var(--danger); border: 1px solid var(--danger); }
  </style>
</head>
<body>
  <div class="header">
    <h1>🔑 Gestor de Múltiples Cuentas OpenCode Go</h1>
    <p>Agrega o elimina cuentas de OpenCode en CPAMC. Cada cuenta se refleja automáticamente como una tarjeta en Auth Files con su propio pool de modelos.</p>
  </div>

  <div class="grid">
    <div class="card">
      <h2>➕ Conectar Nueva Cuenta</h2>
      <div id="alert" class="alert"></div>
      <form id="addForm">
        <div class="form-group">
          <label>Nombre / Etiqueta de la Cuenta</label>
          <input type="text" id="label" placeholder="Ej: Cuenta 1 - Wilox" required />
        </div>
        <div class="form-group">
          <label>API Key de OpenCode (oc_sk_...)</label>
          <input type="password" id="apiKey" placeholder="oc_sk_..." required />
        </div>
        <button type="submit" class="btn">Conectar y Crear Tarjeta en CPAMC</button>
      </form>
    </div>

    <div class="card">
      <h2>📋 Cuentas Conectadas en CPAMC</h2>
      <div id="accountsList">Cargando cuentas...</div>
    </div>
  </div>

  <script>
    const baseUrl = window.location.pathname.replace(/\/accounts\/?$/, "");
    const apiUrl = baseUrl + "/accounts/api";

    async function loadAccounts() {
      const listDiv = document.getElementById("accountsList");
      try {
        const res = await fetch(apiUrl, { method: "GET" });
        const data = await res.json();
        if (!data.accounts || data.accounts.length === 0) {
          listDiv.innerHTML = '<p style="color:var(--muted); font-size:0.9rem;">No hay cuentas conectadas aún. Agrega una desde el formulario.</p>';
          return;
        }
        listDiv.innerHTML = data.accounts.map(acc => ` + "`" + `
          <div class="account-card">
            <div class="account-info">
              <h3>${acc.label || 'OpenCode Go'} <span class="badge">ACTIVA</span></h3>
              <p>Key: ${acc.key_mask} | Prefijo: ${acc.prefix || 'ocgo'} | Archivo: ${acc.id}</p>
              <div class="models-hint">✨ Modelos disponibles: DeepSeek V4 Pro/Flash, GLM 5.3, Grok 4.7, Qwen, Kimi, etc.</div>
            </div>
            <div>
              <button class="btn btn-danger" onclick="deleteAccount('${acc.id}')">🗑️ Quitar</button>
            </div>
          </div>
        ` + "`" + `).join("");
      } catch (err) {
        listDiv.innerHTML = '<p style="color:var(--danger)">Error al cargar cuentas: ' + err.message + '</p>';
      }
    }

    async function deleteAccount(id) {
      if (!confirm("¿Deseas quitar esta cuenta de CPAMC?")) return;
      try {
        const res = await fetch(apiUrl, {
          method: "DELETE",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ id })
        });
        const data = await res.json();
        if (data.ok) {
          showAlert("Cuenta eliminada correctamente", "success");
          loadAccounts();
        } else {
          showAlert(data.error || "Error al eliminar", "error");
        }
      } catch (e) {
        showAlert(e.message, "error");
      }
    }

    document.getElementById("addForm").addEventListener("submit", async (e) => {
      e.preventDefault();
      const label = document.getElementById("label").value;
      const apiKey = document.getElementById("apiKey").value;

      try {
        const res = await fetch(apiUrl, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ label, api_key: apiKey })
        });
        const data = await res.json();
        if (data.ok) {
          showAlert("¡Cuenta conectada! Ya puedes ver su tarjeta en Auth Files.", "success");
          document.getElementById("addForm").reset();
          loadAccounts();
        } else {
          showAlert(data.error || "No se pudo conectar la cuenta", "error");
        }
      } catch (e) {
        showAlert(e.message, "error");
      }
    });

    function showAlert(msg, type) {
      const el = document.getElementById("alert");
      el.textContent = msg;
      el.className = "alert alert-" + type;
      el.style.display = "block";
      setTimeout(() => { el.style.display = "none"; }, 4000);
    }

    loadAccounts();
  </script>
</body>
</html>`
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
