---
title: AI Coding Tools
tags: [llm, tooling, agents]
created: 2026-06-18
updated: 2026-06-18
status: seed
sources: none (seed)
---

# AI Coding Tools

> AI coding tools are editors, assistants, and autonomous agents that use large language
> models to read, write, and reason about code.

## Context
They are the consumers at the top of the LLM stack in this knowledge base: they issue
requests that ultimately hit an [[llm-api-proxy]] or a provider directly. They also operate
the `raw/ → wiki/` pipeline for this [[personal-knowledge-base]], which is why they matter to
[[how-i-work]].

## Key points
- **Examples:** in-editor assistants and autocomplete (e.g. Cursor, Copilot), terminal/CLI
  agents (e.g. Claude Code), and **cloud agents** that run autonomously in a sandbox, commit
  to branches, and open PRs.
- **They speak the OpenAI dialect.** Most accept an [[openai-compatible-api]] endpoint, so
  pointing a tool at a different model is often just a `base_url` + API-key swap.
- **Proxies unlock flexibility.** Routing a tool through an [[llm-api-proxy]] lets you switch
  providers, add fallbacks, and centralize keys without changing the tool.
- **Agents are pipeline workers.** For a knowledge base, an agent can read a new `raw/` file,
  extract atomic notes, and wire `[[links]]` per [[CLAUDE]] §5.

## Related
- [[llm-api-proxy]] — what these tools talk through to reach models
- [[openai-compatible-api]] — the request format they expect
- [[how-i-work]] — how I delegate the KB pipeline to these tools

## Sources
- General tooling landscape. Seed page; no `raw/` source captured yet.
