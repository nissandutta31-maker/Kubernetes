# Cursor + Nemotron 3 Ultra (NVIDIA NIM API)

Configure Cursor to use **Nemotron 3 Ultra** through the NVIDIA NIM OpenAI-compatible API with **maximum reasoning** and the **largest context window your endpoint actually supports**.

> **Important:** Cursor model settings live on **your machine** (`Cursor Settings` or User `settings.json`). This cloud agent cannot change your local Cursor app. Follow the steps below on the computer where Cursor Desktop runs.

---

## 1. NVIDIA NIM API (base setup)

| Setting | Value |
|---|---|
| **Base URL** | `https://integrate.api.nvidia.com/v1` |
| **API key** | Your `nvapi-...` key from [build.nvidia.com](https://build.nvidia.com) |
| **Model ID** | `nvidia/nemotron-3-ultra-550b-a55b` |

### Cursor UI steps

1. Open **Cursor Settings** (`Ctrl/Cmd + ,`) → **Models**
2. Under **OpenAI API Key**: paste your `nvapi-...` key and toggle **ON**
3. Toggle **Override OpenAI Base URL** → **ON**
4. Set base URL to: `https://integrate.api.nvidia.com/v1`
5. Click **Add model** (or **+ Add Custom Model**)
6. Enter model name exactly: `nvidia/nemotron-3-ultra-550b-a55b`
7. Enable the model and select it in Chat / Agent

### Verify the API key works

```bash
export NVIDIA_API_KEY="nvapi-YOUR_KEY"
curl -s https://integrate.api.nvidia.com/v1/models \
  -H "Authorization: Bearer $NVIDIA_API_KEY" | jq '.data[] | select(.id | contains("nemotron-3-ultra"))'
```

---

## 2. Maximum reasoning / thinking (Nemotron-specific)

Nemotron 3 Ultra does **not** use Cursor's built-in "thinking" slider. Reasoning is controlled by NIM request fields:

| Mode | API fields |
|---|---|
| **Max reasoning (recommended for hard problems)** | `"chat_template_kwargs": {"enable_thinking": true}` + `"reasoning_budget": -1` |
| Medium reasoning (faster, fewer thinking tokens) | `"enable_thinking": true, "medium_effort": true` |
| Reasoning off | `"enable_thinking": false` |

`reasoning_budget: -1` means **no cap** on internal thinking tokens (true max reasoning).

### Problem: Cursor may not send these fields

When you use **Override OpenAI Base URL**, Cursor often sends plain chat completions **without** `chat_template_kwargs` or `reasoning_budget`. Nemotron may still think (default ON on NIM), but you cannot guarantee max reasoning from Cursor alone.

### Recommended workaround: local proxy (max reasoning)

```bash
export NVIDIA_API_KEY="nvapi-YOUR_KEY"
make nim-verify   # optional: test key + max-reasoning call
make nim-proxy    # start proxy (keep running)
```

Or run the setup helper:

```bash
make cursor-setup
```

Then in Cursor **Models**:

| Setting | Value |
|---|---|
| Override OpenAI Base URL | `http://127.0.0.1:8765/v1` |
| OpenAI API Key | `unused` (proxy reads `NVIDIA_API_KEY`) |
| Model | `nvidia/nemotron-3-ultra-550b-a55b` |

The proxy forwards to `https://integrate.api.nvidia.com/v1` and injects:

```json
{
  "chat_template_kwargs": {
    "enable_thinking": true,
    "force_nonempty_content": true
  },
  "reasoning_budget": -1
}
```

For **Agent mode** (tools), `force_nonempty_content: true` is required by NVIDIA for parsing reasoning + tool calls together.

---

## 3. Context window: 200K vs 1M

### What Nemotron supports

| Deployment | Native context | Extended context |
|---|---|---|
| **NIM hosted API** (`integrate.api.nvidia.com`) | **256K** (262,144 tokens) — default | Check `/v1/models` for your account; 1M may not be exposed on hosted trial |
| **Self-hosted NIM** (vLLM) | 256K default | Up to **1M** with server env vars |

Self-hosted 1M setup (server side, not Cursor):

```bash
export VLLM_ALLOW_LONG_MAX_MODEL_LEN=1
export NIM_PASSTHROUGH_ARGS="--max-model-len 1048576"
```

### Why Cursor shows ~200K

Cursor's status bar context denominator is often **wrong for custom BYOK models** — known limitation. Custom OpenAI-compatible models may show 200K, 256K, or 1M depending on Cursor version and catalog heuristics, and it may **not** match the real NIM limit.

### What you can do in Cursor today

1. **Model options** (chat panel): set **Context** to the largest available (e.g. **Max**, **200K**, or **1M** if shown).
2. **User settings.json** (merge carefully — see `docs/cursor-settings-nemotron.json` in this repo):

```json
{
  "cursor.chat.contextLength": "long",
  "cursor.aiModels": [
    {
      "name": "nemotron-3-ultra-nim",
      "provider": "openai-compatible",
      "apiKey": "nvapi-YOUR_KEY",
      "baseUrl": "https://integrate.api.nvidia.com/v1",
      "model": "nvidia/nemotron-3-ultra-550b-a55b",
      "contextLength": 1048576,
      "maxTokens": 32768,
      "temperature": 1.0
    }
  ],
  "cursor.defaultModel": "nemotron-3-ultra-nim"
}
```

3. **Reality check on hosted NIM:** even if Cursor assumes 1M, the **API will reject** prompts over the real limit (often 256K on hosted). Check usage/errors in the proxy logs or NIM response.

Open User Settings JSON: `Ctrl/Cmd + Shift + P` → **Preferences: Open User Settings (JSON)**

---

## 4. Suggested parameters for coding / agents

| Parameter | Value | Why |
|---|---|---|
| `temperature` | `1.0` | NVIDIA Nemotron examples for reasoning |
| `top_p` | `0.95` | Default in NIM docs |
| `max_tokens` | `16384`–`32768` | Room for long reasoning traces + code |
| `enable_thinking` | `true` | Full reasoning |
| `reasoning_budget` | `-1` | No thinking cap |
| `force_nonempty_content` | `true` | Required for Agent/tool use |

---

## 5. Troubleshooting

| Symptom | Fix |
|---|---|
| Network / TLS errors | Cursor Settings → Network → **HTTP Compatibility Mode** → **HTTP/1.1** |
| Empty error `{}` in Agent mode | Known Cursor + custom endpoint issue; try **Ask mode** or use the proxy |
| `prompt token count exceeds the limit` | Real API limit hit (often 256K hosted); start new chat or reduce context |
| Status bar shows wrong context size | Known Cursor bug for custom models; trust API errors over UI denominator |
| Slow / timeout on NIM free tier | Hosted NIM queues under load; retry or self-host for production |
| No reasoning visible | Nemotron reasoning may be in a separate field; proxy logs show full response |

---

## 6. Quick test (outside Cursor)

```bash
export NVIDIA_API_KEY="nvapi-YOUR_KEY"
python3 - <<'PY'
from openai import OpenAI
client = OpenAI(
    base_url="https://integrate.api.nvidia.com/v1",
    api_key=__import__("os").environ["NVIDIA_API_KEY"],
)
r = client.chat.completions.create(
    model="nvidia/nemotron-3-ultra-550b-a55b",
    messages=[{"role": "user", "content": "Which is larger: 9.11 or 9.8? Think step by step."}],
    max_tokens=8192,
    temperature=1.0,
    top_p=0.95,
    extra_body={
        "chat_template_kwargs": {"enable_thinking": True},
        "reasoning_budget": -1,
    },
)
print(r.choices[0].message.content)
PY
```

If this works but Cursor does not, use the **local proxy** (Section 2).
