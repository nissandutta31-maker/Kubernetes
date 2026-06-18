---
title: LLM API Proxy
tags: [llm, infrastructure, networking]
created: 2026-06-18
updated: 2026-06-18
status: seed
sources: none (seed)
---

# LLM API Proxy

> An LLM API proxy is a service that sits between a client and one or more model providers to
> route, transform, authenticate, and observe requests.

## Context
It is the middle layer of the LLM stack: [[ai-coding-tools]] call the proxy, and the proxy
calls the real provider. A common job is to present an [[openai-compatible-api]] surface so
existing clients work unchanged regardless of which backend serves the request — exactly what
a [[deepseek-cloud-proxy]] does for DeepSeek models.

## Key points
- **Why proxy:** unify API-key management, swap or load-balance providers, add an
  OpenAI-compatible surface, enforce rate limits, log/audit, cache, and fail over.
- **Request/response translation:** map a client's schema to each provider's native schema
  and back, including model-name remapping.
- **Streaming:** must faithfully forward token streams, typically Server-Sent Events (SSE),
  so interactive tools stay responsive.
- **Trust boundary:** the proxy holds upstream credentials, so it is a security-sensitive
  component (key storage, auth, egress control).

## Related
- [[openai-compatible-api]] — the surface a proxy most often exposes
- [[deepseek-cloud-proxy]] — a concrete proxy instance
- [[ai-coding-tools]] — the typical clients of a proxy

## Sources
- General LLM infrastructure patterns. Seed page; no `raw/` source captured yet.
