---
title: Index — Map of Content
tags: [moc, index]
created: 2026-06-18
updated: 2026-06-18
status: evergreen
sources: none (structural)
---

# Index — Map of Content

The front door to the wiki. Every page in `wiki/` is listed here, grouped into clusters.
New pages must be added to a cluster (see [[CLAUDE]] §5 and §7). Schema lives in [[CLAUDE]];
the capture inbox is described in [[raw/README]].

## Meta — the system itself
- [[personal-knowledge-base]] — what this whole thing is and the method behind it
- [[how-i-work]] — the working principles that operate the PKB

## LLM infrastructure
From client down to backend:

- [[ai-coding-tools]] — editors/agents that consume LLMs (the clients)
- [[llm-api-proxy]] — the routing/translation layer between client and provider
- [[openai-compatible-api]] — the de-facto request/response standard the layer speaks
- [[deepseek-cloud-proxy]] — a concrete proxy serving DeepSeek behind that standard

## How it connects
- The meta cluster explains *why* the KB exists; the LLM-infra cluster is its first body of
  content. [[ai-coding-tools]] is the bridge: agents both *use* the LLM stack and *operate*
  the [[personal-knowledge-base]] pipeline described in [[how-i-work]].
