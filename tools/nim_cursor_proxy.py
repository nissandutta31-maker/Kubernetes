#!/usr/bin/env python3
"""
Local OpenAI-compatible proxy: Cursor -> this server -> NVIDIA NIM API.

Injects Nemotron 3 Ultra max-reasoning fields on every chat completion:
  - chat_template_kwargs.enable_thinking = true
  - chat_template_kwargs.force_nonempty_content = true  (Agent/tools)
  - reasoning_budget = -1  (no thinking token cap)

Supports streaming (SSE) for Cursor chat.

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


def merge_reasoning(payload: dict) -> dict:
    merged = dict(payload)
    extra = dict(merged.get("extra_body") or {})
    chat_kwargs = dict(extra.get("chat_template_kwargs") or {})
    chat_kwargs["enable_thinking"] = True
    chat_kwargs["force_nonempty_content"] = True
    extra["chat_template_kwargs"] = chat_kwargs
    extra["reasoning_budget"] = -1
    merged["extra_body"] = extra
    merged["chat_template_kwargs"] = chat_kwargs
    merged["reasoning_budget"] = -1
    return merged


def upstream_url(path: str) -> str:
    """Join upstream base with request path, avoiding /v1/v1 duplication."""
    if not path.startswith("/"):
        path = "/" + path
    if path.startswith("/v1/"):
        path = path[3:]
    elif path == "/v1":
        path = "/"
    return f"{UPSTREAM_BASE}{path}"


def build_upstream_request(
    method: str, path: str, body: bytes | None, headers: dict[str, str]
) -> tuple[urllib.request.Request, bool]:
    if not UPSTREAM_KEY:
        raise RuntimeError("NVIDIA_API_KEY not set")

    payload = None
    streaming = False
    if body:
        try:
            payload = json.loads(body.decode("utf-8"))
        except json.JSONDecodeError:
            payload = None

    if payload is not None and "/chat/completions" in path:
        payload = merge_reasoning(payload)
        streaming = bool(payload.get("stream"))
        body = json.dumps(payload).encode("utf-8")

    req_headers = {
        "Authorization": f"Bearer {UPSTREAM_KEY}",
        "Content-Type": headers.get("Content-Type", "application/json"),
        "Accept": headers.get("Accept", "application/json"),
    }
    if streaming:
        req_headers["Accept"] = "text/event-stream"

    request = urllib.request.Request(
        upstream_url(path), data=body, method=method, headers=req_headers
    )
    return request, streaming


class ProxyHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _read_body(self) -> bytes | None:
        length = int(self.headers.get("Content-Length", "0"))
        return self.rfile.read(length) if length else None

    def _send_error_json(self, status: int, message: str) -> None:
        body = json.dumps({"error": {"message": message}}).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _proxy(self) -> None:
        body = self._read_body()
        try:
            request, streaming = build_upstream_request(
                self.command, self.path, body, dict(self.headers)
            )
        except RuntimeError as exc:
            self._send_error_json(500, str(exc))
            return

        try:
            upstream = urllib.request.urlopen(request, timeout=600)
        except urllib.error.HTTPError as exc:
            err_body = exc.read()
            self.send_response(exc.code)
            content_type = exc.headers.get("Content-Type", "application/json")
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(err_body)))
            self.end_headers()
            self.wfile.write(err_body)
            return

        if streaming:
            self.send_response(upstream.status)
            content_type = upstream.headers.get("Content-Type", "text/event-stream")
            self.send_header("Content-Type", content_type)
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.end_headers()
            while True:
                chunk = upstream.read(4096)
                if not chunk:
                    break
                self.wfile.write(chunk)
                self.wfile.flush()
            upstream.close()
            return

        resp_body = upstream.read()
        upstream.close()
        self.send_response(upstream.status)
        content_type = upstream.headers.get("Content-Type", "application/json")
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(resp_body)))
        self.end_headers()
        self.wfile.write(resp_body)

    def do_GET(self) -> None:
        self._proxy()

    def do_POST(self) -> None:
        self._proxy()

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self.end_headers()


def main() -> None:
    if not UPSTREAM_KEY:
        sys.exit(
            "Set NVIDIA_API_KEY before starting the proxy.\n"
            "  export NVIDIA_API_KEY=nvapi-...\n"
            "  make nim-proxy"
        )

    server = ThreadingHTTPServer((LISTEN_HOST, LISTEN_PORT), ProxyHandler)
    print(f"NIM Cursor proxy: http://{LISTEN_HOST}:{LISTEN_PORT}/v1")
    print(f"Upstream:         {UPSTREAM_BASE}")
    print("Reasoning:        enable_thinking=true, reasoning_budget=-1, force_nonempty_content=true")
    print("Cursor base URL:  http://127.0.0.1:8765/v1")
    print("Cursor model:     nvidia/nemotron-3-ultra-550b-a55b")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")
        server.server_close()


if __name__ == "__main__":
    main()
