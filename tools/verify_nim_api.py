#!/usr/bin/env python3
"""Verify NVIDIA NIM API key and Nemotron 3 Ultra max-reasoning call."""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request

MODEL = "nvidia/nemotron-3-ultra-550b-a55b"
BASE = "https://integrate.api.nvidia.com/v1"


def request(method: str, path: str, body: dict | None = None) -> tuple[int, dict | str]:
    key = os.environ.get("NVIDIA_API_KEY", "")
    if not key:
        sys.exit("NVIDIA_API_KEY is not set. export NVIDIA_API_KEY=nvapi-...")

    data = None if body is None else json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        f"{BASE}{path}",
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            raw = resp.read().decode("utf-8")
            try:
                return resp.status, json.loads(raw)
            except json.JSONDecodeError:
                return resp.status, raw
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            return exc.code, json.loads(raw)
        except json.JSONDecodeError:
            return exc.code, raw


def main() -> None:
    print("Checking NVIDIA NIM API...")
    status, models = request("GET", "/models")
    if status != 200:
        print(f"FAIL /models HTTP {status}: {models}")
        sys.exit(1)

    ids = [m.get("id", "") for m in models.get("data", [])]
    if MODEL not in ids:
        print(f"WARN: {MODEL} not listed. Available Nemotron models:")
        for mid in sorted(i for i in ids if "nemotron" in i.lower()):
            print(f"  - {mid}")
    else:
        print(f"OK  model listed: {MODEL}")

    print("Testing max-reasoning chat completion...")
    payload = {
        "model": MODEL,
        "messages": [{"role": "user", "content": "Reply with exactly: OK"}],
        "max_tokens": 64,
        "temperature": 1.0,
        "top_p": 0.95,
        "chat_template_kwargs": {"enable_thinking": True},
        "reasoning_budget": -1,
    }
    status, result = request("POST", "/chat/completions", payload)
    if status != 200:
        print(f"FAIL /chat/completions HTTP {status}: {result}")
        sys.exit(1)

    choice = result.get("choices", [{}])[0]
    message = choice.get("message", {})
    content = message.get("content", "")
    reasoning = message.get("reasoning_content")
    usage = result.get("usage", {})

    print(f"OK  response content: {content!r}")
    if reasoning:
        preview = reasoning[:120].replace("\n", " ")
        print(f"OK  reasoning trace:  {preview}...")
    print(f"OK  usage: {usage}")
    print("\nNext: make nim-proxy  (then point Cursor to http://127.0.0.1:8765/v1)")


if __name__ == "__main__":
    main()
