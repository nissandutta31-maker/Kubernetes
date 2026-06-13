#!/usr/bin/env bash
# Print Cursor + Nemotron setup steps and optionally verify NIM API key.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Nemotron 3 Ultra + Cursor setup ==="
echo

if [[ -z "${NVIDIA_API_KEY:-}" ]]; then
  echo "NVIDIA_API_KEY is not set."
  echo "  export NVIDIA_API_KEY=nvapi-YOUR_KEY"
  echo
else
  echo "NVIDIA_API_KEY is set (length ${#NVIDIA_API_KEY})."
  echo "Running API verification..."
  python3 "$ROOT/tools/verify_nim_api.py" || true
  echo
fi

cat <<'EOF'
Cursor Settings -> Models
-------------------------
1. Toggle ON: Override OpenAI Base URL
2. Base URL (with proxy running): http://127.0.0.1:8765/v1
3. OpenAI API Key: unused
4. Add model: nvidia/nemotron-3-ultra-550b-a55b
5. Chat model options -> Context -> largest available (Max / 200K / 1M)

Start proxy (in a separate terminal):
  export NVIDIA_API_KEY=nvapi-...
  make nim-proxy

Optional: merge docs/cursor-settings-nemotron.json into User settings.json
  Ctrl/Cmd+Shift+P -> Preferences: Open User Settings (JSON)

Full guide: docs/cursor-nemotron-nim-setup.md
EOF
