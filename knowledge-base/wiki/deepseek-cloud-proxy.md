---
title: DeepSeek Cloud Proxy
tags: [llm, deepseek, proxy, infrastructure]
created: 2026-06-18
updated: 2026-06-18
status: seed
sources: none (seed)
---

# DeepSeek Cloud Proxy

> A DeepSeek cloud proxy exposes DeepSeek models behind an OpenAI-compatible endpoint so that
> tools expecting OpenAI can use DeepSeek without modification.

## Context
This is a concrete instance of the [[llm-api-proxy]] pattern, specialized for DeepSeek. It
matters for [[ai-coding-tools]] (e.g. cloud coding agents) that are wired to the
[[openai-compatible-api]] but should be served by DeepSeek models behind the scenes.

## Key points
- **Endpoint shape:** presents `/v1/chat/completions` and friends per the
  [[openai-compatible-api]]; clients just override `base_url`.
- **Key mapping:** translates the caller's API key/auth to the upstream DeepSeek credential,
  keeping the real provider key server-side.
- **Model mapping:** maps the requested `model` name to the appropriate DeepSeek model.
- **Streaming passthrough:** forwards SSE token streams so interactive tools stay responsive.
- **Use case:** lets an OpenAI-oriented tool transparently run on DeepSeek — swap the backend
  without touching the tool.

## Related
- [[llm-api-proxy]] — the general pattern this specializes
- [[openai-compatible-api]] — the surface it presents to clients

## Sources
- Specialization of the LLM proxy pattern for DeepSeek. Seed page; no `raw/` source captured
  yet — promote to `growing` after processing real DeepSeek API docs through the pipeline.
