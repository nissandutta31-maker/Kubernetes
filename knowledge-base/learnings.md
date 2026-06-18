# learnings.md — Meta-log

Append-only log of *how the knowledge base itself is going* — what worked, what didn't, and
what to change next time. One entry per working session, **reverse-chronological** (newest on
top). This is the feedback loop that keeps the schema in [[CLAUDE]] honest.

Entry format:

```
## YYYY-MM-DD — short title
**Did:** what changed (pages added/updated, sources processed).
**Worked:** what went well / is worth repeating.
**Didn't:** friction, mistakes, broken links, things that felt wrong.
**Next:** concrete adjustment for next session (or a tweak to CLAUDE.md).
```

> Tooling note — link check: every `[[target]]` should map to `wiki/<target>.md`
> (or `raw/<...>`). A quick manual check is to list link targets and diff against the
> files in `wiki/`. Re-run this before committing (see CLAUDE.md §7).

---

## 2026-06-18 — Scaffold the knowledge base
**Did:** Created the initial structure — `CLAUDE.md` schema, this log, `raw/README.md`, and
the first six seed pages in `wiki/` plus the `index.md` Map of Content. Pages:
[[personal-knowledge-base]], [[how-i-work]], [[ai-coding-tools]], [[llm-api-proxy]],
[[openai-compatible-api]], [[deepseek-cloud-proxy]].
**Worked:** Defining the page template and link rules *before* writing pages kept every page
consistent and orphan-free. The LLM-infra cluster (proxy → compatible-api → deepseek) linked
together naturally.
**Didn't:** All six pages are `status: seed` — they assert ideas but cite no real `raw/`
sources yet, so claims lean on general knowledge. The `raw/` inbox is empty, so the
`raw/ → wiki/` pipeline hasn't actually been exercised end-to-end.
**Next:** Drop a real source into `raw/` (e.g. DeepSeek API docs) and run the pipeline from
CLAUDE.md §5 to promote [[deepseek-cloud-proxy]] from `seed` to `growing` with cited facts.
