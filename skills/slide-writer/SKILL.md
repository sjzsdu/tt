---
name: slide-writer
description: Write, polish, refactor, and validate tt .slide presentation decks. Use when the user asks to create slides, improve a deck, make MagiCloud slides more beautiful, add cover/closing pages, tune slide structure, or convert notes into a polished .slide presentation.
license: MIT
---

# Slide Writer

Use this skill to author beautiful `.slide` decks for `tt slide`.

The goal is not to dump Markdown onto pages. The goal is to create a **presentation rhythm**: one idea per slide, clear visual hierarchy, concise text, and MagiCloud-compatible page types.

## Current product facts

- `tt slide` only scans and opens `.slide` files. Do not create `.md` files for slide decks.
- Start a deck with `tt slide path/to/deck.slide`.
- Slides are separated by `---`.
- Default template is `magicloud`.
- Template override is available with `--template` / `-t`.
- Supported page directives include:
  - `.center`
  - `.cover`
  - `.split`
  - `.two-column` / `.columns`
  - `.logo` / `.brand`
  - `.end` / `.closing` / `.final`
- MagiCloud closing page uses `.end`, `.closing`, or `.final` on its own slide.
- ESC overview is a centered horizontal scroller. Do not design content that depends on overview layout.
- Per-file slide position is persisted independently in browser localStorage.

## Default deck shape

For most polished decks, use this structure:

```text
1. Cover: title + subtitle + speaker/date
2. Problem / context
3. Key message / thesis
4. 3 to 5 body sections
5. Architecture / process / comparison slide if useful
6. Summary / next steps
7. .end closing page
```

Prefer 8 to 14 slides for a normal internal presentation. If the user gives long notes, split by narrative purpose, not by paragraph count.

## Style rules for beautiful MagiCloud slides

### 1. One slide, one job

Each slide should answer one question:

- What is the point?
- Why should the audience care?
- What should they remember?

If a slide has multiple unrelated points, split it.

### 2. Use short titles

Good titles are conclusions, not labels.

Prefer:

```markdown
# Compute moves from static clusters to elastic pools
```

Avoid:

```markdown
# Architecture
```

### 3. Keep text sparse

Target per normal slide:

- Title: 6 to 12 words.
- Body: 3 to 5 bullets.
- Bullet: 6 to 14 words.
- Paragraphs: 1 to 2 short paragraphs only.

Avoid full sentences when a crisp phrase works.

### 4. Make hierarchy obvious

Use Markdown hierarchy consistently:

```markdown
# Main conclusion

Short supporting sentence.

- Primary point
- Primary point
- Primary point
```

Use `**bold**` only for keywords, not whole sentences.

### 5. Prefer layout directives over dense prose

Use `.two-column` when comparing two concepts:

```markdown
---

.two-column

## Today

- Manual capacity planning
- Long queue times
- Siloed utilization

||

## MagiCloud

- Elastic compute pools
- Predictable throughput
- Shared utilization model
```

Use `.split` for text + image/diagram or concept + evidence.

Use `.center` for a single big message.

### 6. Always finish MagiCloud decks with `.end`

For a MagiCloud deck, add a final closing page:

```markdown
---

.end
```

Aliases `.closing` and `.final` are valid, but `.end` is the recommended default because it is short and readable.

## Authoring patterns

### Cover slide

The first slide automatically receives the MagiCloud cover treatment.

```markdown
# MagiCloud Platform Update

Elastic cloud HPC for engineering simulation

Name · Team · 2026-06-30
```

Keep cover content short. Do not add many bullets on the cover.

### Executive summary

```markdown
---

# Three changes define the next MagiCloud release

- **Elastic scheduling** reduces queue waiting for burst workloads
- **Unified observability** makes cost and performance visible together
- **Template-driven onboarding** shortens time from request to first run
```

### Two-column comparison

```markdown
---

.two-column

## Before

- Static cluster assumptions
- Fragmented job queues
- Manual environment setup

||

## After

- Elastic compute pools
- Policy-aware placement
- Repeatable run templates
```

### Center message slide

```markdown
---

.center

# The product promise is predictable engineering throughput

Less waiting, fewer manual steps, clearer operating data.
```

### Closing page

```markdown
---

.end
```

## Polishing checklist

Before calling the deck done:

1. Every slide has one clear purpose.
2. Titles read like takeaways.
3. No slide has more than 5 bullets unless it is a reference appendix.
4. Bullet lines are short and parallel.
5. `.two-column` slides have balanced left/right content.
6. The final slide is `.end` for MagiCloud decks.
7. The file extension is `.slide`.
8. Run `tt slide deck.slide` or at least ensure the syntax uses `---` correctly.

## Validation

When editing code or docs in this repo for slide behavior:

```bash
cd web/apps/slide && npm run build
go test ./cmd
```

When only writing a `.slide` deck, no build is required, but visually inspect with `tt slide` when possible.

## Common mistakes

- Do not create `.md` decks. Use `.slide`.
- Do not paste long Markdown reports as slides.
- Do not rely on raw HTML for layout before trying built-in directives.
- Do not use `.end` with extra visible text unless the user explicitly wants a custom closing page.
- Do not overuse bold, emoji, or code blocks in executive decks.

## Handoff

When this skill has just been added or updated in the current session, tell the user that new sessions may need to reload skills before it is automatically selected.
