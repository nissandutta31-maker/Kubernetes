---
title: OpenAI-Compatible API
tags: [llm, api, standard]
created: 2026-06-18
updated: 2026-06-18
status: seed
sources: none (seed)
---

# OpenAI-Compatible API

> An OpenAI-compatible API is an HTTP API that mirrors OpenAI's request/response schema, so
> any client built for OpenAI works against it with only a `base_url` change.

## Context
It has become the de-facto lingua franca of the LLM ecosystem. Because so many
[[ai-coding-tools]] are written against it, providers and proxies — including any
[[llm-api-proxy]] and the [[deepseek-cloud-proxy]] — expose this shape to maximize
compatibility.

## Key points
- **Core surface:** `POST /v1/chat/completions` (plus `/v1/models`, `/v1/embeddings`, etc.),
  with a `messages` array, a `model` field, and parameters like `temperature`.
- **Drop-in swapping:** point a client at a new `base_url` and API key; the `model` string
  selects the backend model. No client code changes.
- **Streaming:** `stream: true` returns incremental chunks over Server-Sent Events (SSE),
  terminated by a `[DONE]` sentinel.
- **Why it's a standard:** network effects — tooling, SDKs, and docs already assume it, so
  implementing it is the cheapest path to adoption for any new endpoint.

## Related
- [[llm-api-proxy]] — the component that usually exposes this surface
- [[deepseek-cloud-proxy]] — exposes DeepSeek behind this schema
- [[ai-coding-tools]] — the clients that depend on this format

## Sources
- OpenAI API conventions as a de-facto standard. Seed page; no `raw/` source captured yet.
