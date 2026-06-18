# CLAUDE.md — Knowledge Base Schema & Operating Manual

This file is the **single source of truth** for how this knowledge base is built and
maintained. Any agent (or human) working anywhere under `knowledge-base/` MUST read this
first and follow it exactly. If a rule here conflicts with your instinct, the rule wins.

---

## 1. Purpose

A personal, atomic, cross-linked wiki — a "second brain". Sources are captured verbatim in
`raw/`, then distilled into small, single-idea pages in `wiki/` that reference each other
with `[[wiki-links]]`. The philosophy is documented in [[personal-knowledge-base]].

Two modes, kept strictly separate:

- **Capture** (cheap, lossy-free): drop a source into `raw/`. Never reasoned about at capture time.
- **Distill** (expensive, lossy): turn raw material into atomic `wiki/` pages. This is where thinking happens.

---

## 2. Layout

| Path | Role | Mutable? |
|---|---|---|
| `CLAUDE.md` | This schema. The rules. | Yes (deliberate edits only) |
| `learnings.md` | Meta-log: what worked / what didn't, per session. | Append-only |
| `raw/` | Immutable inbox of captured sources. | **No — never edit/delete** |
| `wiki/` | Atomic, cross-linked pages. | Yes |
| `wiki/index.md` | Map of Content (MoC) — the front door. | Yes |

---

## 3. Page template

Every file in `wiki/` (except `index.md`) MUST begin with this frontmatter and follow this
skeleton. Keep it short: if a page outgrows one screen, split it.

```markdown
---
title: Human Readable Title
tags: [topic, another-topic]
created: YYYY-MM-DD
updated: YYYY-MM-DD
status: seed | growing | evergreen
sources: [[raw/2026-06-18-some-source.md]]   # or external URLs; "none (seed)" if hand-written
---

# Human Readable Title

> One-sentence atomic claim. If you can't state the idea in a sentence, the page is not atomic yet.

## Context
Why this exists and where it sits relative to neighbouring ideas.

## Key points
- Bullet the load-bearing facts. Prefer fewer, sharper bullets.

## Related
- [[other-page]] — one clause on *why* it's related (never a bare link)

## Sources
- Where this came from (raw file links and/or external references).
```

---

## 4. `[[wiki-link]]` rules

1. A link target is the **filename without extension**, kebab-case: `[[openai-compatible-api]]`
   resolves to `wiki/openai-compatible-api.md`.
2. Links to captured sources are namespaced: `[[raw/2026-06-18-foo.md]]`.
3. Every `[[link]]` in a `## Related` section MUST be annotated with *why* (` — reason`).
4. **No orphans:** every `wiki/` page must be reachable from `[[index]]` and must link out to
   at least one other page.
5. **No dangling links:** never write `[[x]]` unless `wiki/x.md` exists (or you create it in the
   same change). Run the link check in §7 before finishing.
6. Prefer linking to duplicating. If two pages need the same explanation, extract a third page
   and link both to it.

---

## 5. The `raw/ → wiki/` pipeline (the "hook")

`raw/` is a watched inbox. When a new source lands there, run this pipeline (an agent executes
it; there is no magic — the "hook" is this documented procedure, see [[raw/README]]):

1. **Read** the new raw file end-to-end. Do not edit it.
2. **Extract atoms:** list every distinct, reusable idea it contains (one idea = one page).
3. For each atom:
   - If a matching `wiki/` page exists → **update** it (merge facts, bump `updated`, add the
     raw file to `sources`).
   - Else → **create** a new page from the §3 template.
4. **Link:** wire new/updated pages into the existing graph via `## Related`. Reuse existing
   pages aggressively; avoid near-duplicates.
5. **Index:** add any new page to the correct cluster in `[[index]]`.
6. **Validate:** run the §7 checks.
7. **Log:** append a session entry to `learnings.md` (what was added, what was hard, what to
   do differently).

---

## 6. Guardrails (hard rules)

- **Atomicity:** one concept per page. When in doubt, split.
- **Immutability of `raw/`:** treat it as write-once. Corrections live in `wiki/`, never by
  rewriting history in `raw/`.
- **No orphans, no dangling links** (see §4.4–4.5).
- **Cite or it didn't happen:** every non-obvious claim ties back to a `sources` entry.
- **Kebab-case filenames** that exactly match their `[[link]]` target.
- **Frontmatter is mandatory** and must stay valid YAML.
- **Brevity beats completeness.** A note you'll actually re-read > an exhaustive essay.
- **Don't invent structure** the schema doesn't define (no stray top-level folders).

---

## 7. Definition of done (run before every commit)

- [ ] New/changed `wiki/` pages follow the §3 template with valid frontmatter.
- [ ] Every `[[link]]` resolves to an existing file (no dangling links).
- [ ] No `wiki/` page is an orphan (reachable from `[[index]]`, links out ≥1).
- [ ] `[[index]]` lists every `wiki/` page.
- [ ] A session entry was appended to `learnings.md`.

A quick structural check (every wiki link target has a file) is in `learnings.md`'s tooling
note and can be re-run by hand.
