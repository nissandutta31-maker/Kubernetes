# raw/ — the capture inbox

This folder is a **write-once inbox** for source material: articles, docs, chat logs,
snippets, screenshots — anything worth distilling later. It is the "capture" half of the
system described in [[../wiki/personal-knowledge-base]].

## Rules

- **Immutable.** Once a file lands here, do **not** edit, rename, or delete it. It is the
  permanent record of *what the source actually said*. All interpretation happens in `wiki/`.
- **Capture fast, reason later.** Don't clean up or summarize on the way in. Drop it raw.
- **Corrections never rewrite history.** If a source was wrong or superseded, capture the
  correction as a *new* raw file and fix the conclusion in the relevant `wiki/` page.

## Naming

```
YYYY-MM-DD-short-slug.ext
```

e.g. `2026-06-18-deepseek-api-reference.md`, `2026-06-18-cursor-cloud-agent-thread.txt`.
Date = when captured. The date prefix keeps the inbox sorted and gives every source a stable
`[[raw/...]]` link target.

## The "hook"

This directory is treated as a watched inbox. When a new file appears, an agent runs the
`raw/ → wiki/` pipeline defined in [[../CLAUDE]] §5: read → extract atoms → create/update
atomic pages → cross-link → update the [[../wiki/index]] Map of Content → log the session in
`learnings.md`. The "hook" is that documented procedure, not a background daemon — running it
is what turns raw capture into linked knowledge.
