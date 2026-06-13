#!/usr/bin/env python3
"""
Local OpenAI-compatible proxy: Cursor -> this server -> NVIDIA NIM API.

Injects Nemotron 3 Ultra max-reasoning fields on every chat completion:
  - chat_template_kwargs.enable_thinking = true
  - chat_template_kwargs.force_nonempty_content = true  (Agent/tools)
  - reasoning_budget = -1  (no thinking token cap)

Usage:
  export NVIDIA_API_KEY=nvapi-...
  python3 tools/nim_cursor_proxy.py

Cursor Models settings:
  Override OpenAI Base URL: http://127.0.0.1:8765/v1
  OpenAI API Key: unused
  Model: nvidia/nemotron-3-ultra-550b-a55b
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LISTEN_HOST = os.environ.get("NIM_PROXY_HOST", "127.0.0.1")
LISTEN_PORT = int(os.environ.get("NIM_PROXY_PORT", "8765"))
UPSTREAM_BASE = os.environ.get(
    "NIM_UPSTREAM_BASE", "https://integrate.api.nvidia.com/v1"
).rstrip("/")
UPSTREAM_KEY = os.environ.get("NVIDIA_API_KEY", "")

MAX_REASONING_BODY = {
    "chat_template_kwargs": {
        "enable_thinking": True,
        "force_nonempty_content": True,
    },
    "reasoning_budget": -1,
}


def merge_reasoning(payload: dict) -> dict:
    merged = dict(payload)
    extra = dict(merged.get("extra_body") or {})
    chat_kwargs = dict(extra.get("chat_template_kwargs") or {})
    chat_kwargs["enable_thinking"] = True
    chat_kwargs["force_nonempty_content"] = True
    extra["chat_template_kwargs"] = chat_kwargs
    extra["reasoning_budget"] = -1
    merged["extra_body"] = extra
    # Also set top-level fields NIM accepts directly.
    merged["chat_template_kwargs"] = chat_kwargs
    merged["reasoning_budget"] = -1
    return merged


def forward(method: str, path: str, body: bytes | None, headers: dict[str, str]) -> tuple[int, bytes, dict[str, str]]:
    if not UPSTREAM_KEY:
        return 500, b'{"error":{"message":"NVIDIA_API_KEY not set"}}', {"Content-Type": "application/json"}

    url = f"{UPSTREAM_BASE}{path}"
    req_headers = {
        "Authorization": f"Bearer {UPSTREAM_KEY}",
        "Content-Type": headers.get("Content-Type", "application/json"),
    }

    payload = None
    if body:
        try:
            payload = json.loads(body.decode("utf-8"))
        except json.JSONDecodeError:
            payload = None

    if payload is not None and path.endswith("/chat/completions"):
        payload = merge_reasoning(payload)
        body = json.dumps(payload).encode("utf-8")

    request = urllib.request.Request(url, data=body, method=method, headers=req_headers)
    try:
        with urllib.request.urlopen(request, timeout=300) as resp:
            return resp.status, resp.read(), dict(resp.headers)
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read(), dict(exc.headers)


class ProxyHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _handle(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length) if length else None
        status, resp_body, resp_headers = forward(self.command, self.path, body, dict(self.headers))

        self.send_response(status)
        content_type = resp_headers.get("Content-Type", "application/json")
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(resp_body)))
        self.end_headers()
        self.wfile.write(resp_body)

    def do_GET(self) -> None:
        self._handle()

    def do_POST(self) -> None:
        self._handle()

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self.end_headers()


def main() -> None:
    if not UPSTREAM_KEY:
        sys.exit("Set NVIDIA_API_KEY before starting the proxy.")

    server = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), ProxyHandler)
    print(f"NIM Cursor proxy listening on http://{LISTEN_HOST}:{LISTEN_PORT}/v1")
    print(f"Upstream: {UPSTREAM_BASE}")
    print("Injecting enable_thinking=true, force_nonempty_content=true, reasoning_budget=-1")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")
        server.server_close()


if __name__ == "__main__":
    main()
