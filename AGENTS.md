# Agent Instructions

## Model selection: DeepSeek V4 Pro and Cloud Agents

DeepSeek V4 Pro cannot run on **Cursor Cloud Agents**. Cloud Agents only support Cursor's curated Max Mode model list (Claude, GPT, Gemini, Composer, Grok, Kimi, etc.). DeepSeek is not on that list.

### Option A — Use DeepSeek V4 Pro locally

1. In Cursor desktop, switch the agent dropdown from **Cloud** to **Local**.
2. Open **Settings → Models → Add Custom Model**.
3. Enter the model ID exactly: `deepseek-v4-pro` (or `deepseek-v4-flash` for a cheaper/faster tier).
4. Set **Base URL** to `https://api.deepseek.com` (do not add `/v1`; Cursor appends it).
5. Paste your DeepSeek API key and click **Verify**.
6. Update Cursor to the latest stable version to avoid known "model not found" resume bugs with BYOK models.

If multi-turn agent sessions fail with `reasoning_content` errors on `deepseek-v4-pro`, try `deepseek-v4-flash` for simpler tasks, or route through a local Ollama proxy.

### Option B — Use Cloud Agents with a supported model

1. Keep **Cloud** selected in the agent dropdown.
2. Choose a supported Max Mode model (e.g. Claude Opus/Sonnet, GPT-5.x, Gemini 3.x, Composer 2.5).
3. Ensure API usage and a Cloud Agent spend limit are configured in your Cursor dashboard.

## Cursor Cloud specific instructions

This repo is a Kubernetes runtime demo. Cloud agents should use a supported model (Option B above), not DeepSeek.

### Prerequisites installed in the cloud environment

The `.cursor/Dockerfile` provides Go 1.22, Docker, kubectl, kind, and Python 3.

### Common commands

```bash
# Build the app image
make build

# Create a local Kind cluster, deploy, and verify
make all

# Step by step
make kind-up
make deploy
make verify

# Expose the service locally inside the VM
make port-forward
```

### Verification

After `make deploy`, run `make verify`. All pods in the `nvidia-runtime-demo` namespace should report `Running`.

### Notes

- Kind requires Docker. The cloud environment starts the Docker daemon on boot.
- `make kind-up` is idempotent; it tolerates an existing cluster.
- The health check script (`automation/health_check.py`) needs a reachable Kubernetes API — run `make kind-up` and `make deploy` first.
