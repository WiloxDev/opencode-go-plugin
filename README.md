# OpenCode Go Plugin for CPAMC

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.20%2B-00ADD8?logo=go)](https://go.dev/)
[![CPAMC Compatibility](https://img.shields.io/badge/CPAMC-ABI%20v1%20Compliant-success)](#compatibility)
[![Platform](https://img.shields.io/badge/Platform-Linux%20x86__64-orange)](#system-requirements)

High-performance native Go integration plugin for **CLI Proxy API Console (CPAMC)**. Connects OpenCode Go providers, delivers automated model catalog synchronization, and handles API authentication directly through CGo ABI v1 shared library bindings.

---

## Highlights & Features

- **Blazing Fast Native Execution**: Implemented in Go and compiled as a C-shared library (`.so`) with zero Node/Python runtime overhead.
- **Dynamic Model Catalog**: Automatically registers modern AI models (DeepSeek, GLM, Grok, Qwen, Kimi, MiniMax, GPT Luna, and more) into CPAMC with up to 1M context windows.
- **Streamlined API Key Auth**: Native `auth.start_login` and `auth.poll_login` flow for OpenCode API keys (`oc_sk_...`).
- **Standard ABI v1 Support**: Full compliance with `cliproxy_plugin.h` ABI specifications (schema v6, thread-safe buffers, error envelopes, and panic recovery).

---

## Quick Installation

### Option 1: One-Line Auto Installer (Recommended)
Download and install the latest release directly into your CPAMC plugins directory:

```bash
curl -fsSL https://raw.githubusercontent.com/WiloxDev/opencode-go-plugin/main/install.sh | bash
```

### Option 2: Build From Source

#### Requirements:
- **Go** >= 1.20
- **GCC / Build Essentials** (`sudo apt install build-essential` or distro equivalent)

```bash
# Clone the repository
git clone https://github.com/WiloxDev/opencode-go-plugin.git
cd opencode-go-plugin

# Compile the shared library (.so)
make build

# Install to ~/.config/cpamc/plugins/
make install
```

---

## Configuration & Usage in CPAMC

1. Ensure the plugin binary is placed in your CPAMC plugin directory:
   - Default: `~/.config/cpamc/plugins/opencode-go-linux-amd64.so`
2. Start or restart your CPAMC instance:
   ```bash
   cpamc restart
   ```
3. Authenticate with your OpenCode credentials:
   - Go to CPAMC web console or TUI.
   - Select **OpenCode Go** provider.
   - Enter your OpenCode API Key (`oc_sk_...`).

---

## Model Catalog Overview

The plugin automatically registers and manages capabilities for the following models:

| Family | Models | Context Window | Max Output Tokens |
| :--- | :--- | :--- | :--- |
| **DeepSeek** | `deepseek-v4-pro`, `deepseek-v4.1-flash`, `deepseek-v4-flash-vision-exp` | 1,048,576 | 64,000 |
| **GLM** | `glm-5.3`, `glm-5.3-flash`, `glm-5.2` | 1,048,576 | 64,000 |
| **Grok** | `grok-4.7`, `grok-4.6` | 1,048,576 | 64,000 |
| **Qwen** | `qwen3.8-max`, `qwen3.8-flash`, `qwen3.7-plus` | 1,048,576 | 64,000 |
| **Kimi** | `kimi-k3`, `kimi-k2.7-code` | 1,048,576 | 64,000 |
| **MiniMax & MiMo** | `minimax-m3`, `mimo-v2.6-pro`, `mimo-v2.6-flash` | 1,048,576 | 64,000 |
| **Luna Series** | `gpt-6-luna`, `gpt-5.6-luna` | 1,048,576 | 64,000 |

---

## Diagnostics & Troubleshooting

The plugin writes detailed call tracing and crash protection logs to `/tmp`:

```bash
tail -f /tmp/opencode-go-plugin.log
```

If you encounter unexpected errors or need debug information:
1. Verify ABI version matching: `ABI Version 1`.
2. Inspect log timestamps and payload sizes in `/tmp/opencode-go-plugin.log`.

---

## Commercial Licensing & Enterprise Support

This project is licensed under the **MIT License**.

For custom integrations, white-label distributions, enterprise SLAs, or dedicated plugin development:
- **Author**: Wilox ([WiloxDev](https://github.com/WiloxDev))
- **Email**: [wilsonlavio9@gmail.com](mailto:wilsonlavio9@gmail.com)
- **Repository**: [https://github.com/WiloxDev/opencode-go-plugin](https://github.com/WiloxDev/opencode-go-plugin)
