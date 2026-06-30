---
name: slide-template-writer
description: Design, write, debug, and refine tt slide templates under .tt/slide/templates/<name>/. Use when changing template.json, template.css, template assets, typography, colors, backgrounds, cover/brand/closing visuals, or template-specific rendering behavior.
license: MIT
---

# Slide Template Writer

Use this skill when implementing or refining `tt slide` templates.

A slide template is a self-contained visual package. It decides how a template-agnostic `.slide` document looks: colors, fonts, spacing, backgrounds, logos, cover pages, brand pages, closing pages, and diagram/card styling.

The `.slide` document remains semantic. The template owns all visual decisions.

## Mental model

```text
.slide document          template package
------------------       ------------------------------
# Title                  template.json: metadata/defaults
.center                  template.css: visual rules
.end                     assets/: backgrounds/logos/etc.
Markdown content   --->  rendered presentation
```

Do not ask `.slide` authors to write template-specific asset paths, CSS classes, colors, or logo references. If a visual effect belongs to a brand or theme, put it in the template.

## Current architecture facts

- External templates live under `.tt/slide/templates/<name>/` for the current project.
- Global external templates live under `~/.tt/slide/templates/<name>/`.
- Project templates take precedence over global templates with the same name.
- Built-in fallback templates are still available: `dark`, `light`, `serif`, `white`.
- Default runtime template name is `magicloud`.
- The MagiCloud template is stored as `.tt/slide/templates/magicloud/`.
- `tt slide --template NAME` selects a template.
- `tt slide --list-templates` lists available project, global, and built-in templates.
- The slide server loads external templates through `/api/template/<name>` and serves template assets through `/template-assets/<name>/...`.
- Slide parsing and semantic directives live in `web/apps/slide/src/parser.ts`.
- Global app chrome styles live in `web/apps/slide/src/styles.css`.

## External template directory

Create one directory per template:

```text
.tt/slide/templates/customer-brand/
├── template.json
├── template.css
└── assets/
    ├── cover-bg.png
    ├── logo-dark.png
    ├── logo-white.svg
    └── texture.webp
```

Required files:

- `template.json`: template metadata and Reveal defaults.
- `template.css`: all template-specific visual styles.

Optional:

- `assets/`: backgrounds, logos, textures, icons, decorative images.

## `template.json`

Minimal valid template:

```json
{
  "name": "customer-brand",
  "revealTheme": "white",
  "css": "template.css",
  "defaults": {
    "theme": "light",
    "transition": "fade",
    "center": false,
    "margin": 0
  },
  "vars": {
    "slide-padding": "96px 112px 72px",
    "cover-background": "url(\"./assets/cover-bg.png\") center / cover no-repeat",
    "brand-logo-width": "280px"
  }
}
```

Fields:

| Field | Required | Meaning |
| --- | --- | --- |
| `name` | recommended | Template name. Should match directory name. |
| `revealTheme` | recommended | Reveal base theme, usually `white`, `black`, or `serif`. |
| `css` | optional | CSS file path relative to template dir. Defaults to `template.css`. |
| `defaults.theme` | optional | App theme for diagram/content rendering: `light` or `dark`. Defaults to `light`. |
| `defaults.transition` | optional | Reveal transition: `fade`, `slide`, `none`, `convex`, `concave`, `zoom`. Defaults to `fade`. |
| `defaults.center` | optional | Reveal vertical centering default. Usually `false` for designed 16:9 pages. |
| `defaults.margin` | optional | Reveal viewport margin. Usually `0`; use CSS padding variables for actual page margins. |
| `vars` | optional | Template-level CSS variables injected into `:root`. Use this for page padding, brand colors, background images, logo sizes, etc. |

The app renders every slide on a fixed 16:9 presentation canvas (`1600px × 900px`) and scales that whole canvas to fit the browser window or fullscreen viewport. Do not set `defaults.width` or `defaults.height`; templates should design inside the fixed canvas using CSS variables, section padding, and stable grid/flex layouts.

Do not put large CSS blocks inside JSON. Keep visual rules in `template.css`; use `vars` only for global knobs that a template owner may tune.

## Template-level `vars`

`vars` in `template.json` are converted to CSS variables before `template.css` is loaded:

```json
{
  "vars": {
    "slide-padding": "120px 112px 72px",
    "cover-background": "url(\"./assets/cover-bg.png\") center / cover no-repeat",
    "content-bg": "#ffffff",
    "logo-width": "280px"
  }
}
```

Then use them in `template.css`:

```css
.reveal .slides section {
  padding: var(--slide-padding);
  background: var(--content-bg);
}

.reveal:has(.slides section:first-child.present)::before {
  background: var(--cover-background);
}

.reveal .slides section:not(:first-child)::before {
  width: var(--logo-width);
}
```

Good candidates for `vars`:

- `slide-padding`, `cover-padding`, `content-padding`
- `content-bg`, `cover-background`, `closing-background`
- `logo-width`, `logo-height`, `logo-top`, `logo-left`
- brand colors such as `brand-primary`, `brand-accent`, `text-muted`

Keep fine-grained selectors and layout rules in `template.css`.

## `template.css` basics

Start by defining CSS variables and normal slide defaults:

```css
:root {
  --slide-bg: #ffffff;
  --slide-fg: #1f2329;
  --slide-title: #0f5132;
  --slide-accent: #008d55;
  --slide-muted: #667085;
  --slide-card-bg: #f5f7f8;
  --slide-card-border: #dde3e6;
}

.reveal {
  background: var(--slide-bg);
  color: var(--slide-fg);
  font-family: "Inter", "Segoe UI", "PingFang SC", sans-serif;
}

.reveal .slides section {
  height: 100%;
  min-height: 100%;
  overflow: hidden;
  padding: var(--slide-padding, 120px 112px 72px);
  background: var(--slide-bg);
  color: var(--slide-fg);
}

.reveal h1,
.reveal h2,
.reveal h3 {
  color: var(--slide-title);
  text-transform: none;
  letter-spacing: 0;
}

.reveal p,
.reveal li {
  color: var(--slide-fg);
  line-height: 1.55;
}
```

Recommended pattern:

1. Define variables in `:root`.
2. Style `.reveal` for global font/background.
3. Style `.reveal .slides section` for normal pages.
4. Style headings/body/list/table/code/blockquote.
5. Add rules for semantic page classes.
6. Add overview-safe background handling.

## Assets and URLs

Put template-owned resources in `assets/` and reference them with relative URLs:

```css
.reveal .slides section:not(:first-child)::before {
  content: "";
  position: absolute;
  top: 48px;
  left: 64px;
  width: 280px;
  height: 32px;
  background: url("./assets/logo-dark.png") left center / contain no-repeat;
}
```

The server rewrites relative URLs such as `./assets/logo-dark.png` to `/template-assets/<name>/assets/logo-dark.png`.

Asset rules:

- Use relative URLs only: `url("./assets/file.png")`.
- Do not use absolute local filesystem paths such as `/Users/.../logo.png`.
- Do not use `../` traversal.
- Do not put template asset paths in `.slide` documents.
- Business images from slide content are not template assets. Keep those beside the `.slide` deck and reference them with Markdown.

## Semantic page classes you can style

The parser maps `.slide` semantic directives to CSS classes:

| `.slide` directive | Generated class | Template responsibility |
| --- | --- | --- |
| first slide or `.cover` | first `section`, sometimes `slide-cover` if supported | Cover/title visual |
| `.center` | `.slide-center` | Big centered message |
| `.split` | `.slide-split` | Split layout / text plus evidence |
| `.two-column` / `.columns` | `.slide-two-column` | Balanced columns |
| `.brand` / `.logo` | `.slide-logo` | Brand/identity page |
| `.end` / `.closing` / `.final` | `.slide-closing` | End/closing page |

Do not invent a new `.slide` directive unless the parser supports it. For new semantics, update parser/types/docs/tests first.

## Cover page pattern

The first slide often acts as the cover. Style it without requiring `.slide` authors to add template-specific markup:

```css
.reveal:has(.slides section:first-child.present)::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    linear-gradient(135deg, rgba(0,0,0,0.8), rgba(0,141,85,0.75)),
    url("./assets/cover-bg.png") center / cover no-repeat;
}

.reveal .slides section:first-child {
  background: transparent;
  color: #ffffff;
}

.reveal .slides section:first-child .slide-content {
  position: relative;
  z-index: 1;
  padding-top: 360px;
}

.reveal .slides section:first-child h1 {
  color: #ffffff;
  font-size: 64px;
}
```

Why use `.reveal::before` or `:has(...present)::before` for full-screen backgrounds? It keeps backgrounds fixed during slide transitions and avoids visible cut edges.

## Brand/logo page pattern

For `.brand` / `.logo` slides:

```css
.reveal:has(.slides section.slide-logo.present)::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    url("./assets/texture.webp") center / cover no-repeat,
    #ffffff;
}

.reveal .slides section.slide-logo {
  background: transparent;
}

.reveal .slides section.slide-logo::before {
  content: "";
  position: absolute;
  top: 50%;
  left: 50%;
  width: 520px;
  height: 120px;
  transform: translate(-50%, -50%);
  background: url("./assets/logo-dark.png") center / contain no-repeat;
}
```

## Closing page pattern

For `.end` / `.closing` / `.final` slides:

```css
.reveal:has(.slides section.slide-closing.present)::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    url("./assets/cover-bg.png") center / cover no-repeat,
    linear-gradient(135deg, #000000, #008d55);
}

.reveal .slides section.slide-closing {
  background: transparent;
  color: #ffffff;
}

.reveal .slides section.slide-closing .slide-content {
  display: flex;
  align-items: center;
  justify-content: center;
}
```

The `.slide` file should only contain:

```markdown
---

.end
```

The template decides what the closing page looks like.

## Two-column and split layouts

Style parser-generated classes, not author-specific markup:

```css
.slide-two-column .slide-content {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 56px;
  align-items: start;
}

.slide-two-column .slide-part-column {
  min-width: 0;
}

.slide-split .slide-content {
  display: grid;
  grid-template-columns: 0.9fr 1.1fr;
  gap: 48px;
  align-items: center;
}
```

## Overview compatibility

ESC overview renders thumbnails. Preserve recognizable backgrounds there.

Rules:

- Avoid global `.reveal.overview { inset: ... !important; }` style fights.
- Avoid `!important` unless unavoidable.
- If normal playback uses a fixed `.reveal::before` background, also provide section-level fallback backgrounds for overview thumbnails if needed.
- Keep pseudo-elements `pointer-events: none`.

## Viewport adaptation

Do not rely on Go to know the user's display size. The Go command starts the local server, but the actual viewport is known only in the browser. Template adaptation should happen in CSS and Reveal configuration:

- Prefer full-size containers: `height: 100%`, `width: 100%`, flex/grid layouts.
- Use viewport-aware limits such as `max-height: 72vh`, `min()`, `clamp()`, and CSS variables.
- Use template `vars` for tunable padding/background/logo sizes.
- Avoid hardcoding canvas `width` / `height` unless a template absolutely requires a fixed design canvas.
- Let diagram-heavy slides use available space via `.slide-diagram-heavy` and `.slide-diagram-only` classes.
- Preserve `.diagram-viewport`, `.diagram-panzoom`, `.diagram-svg`, and `.diagram-toolbar` behavior; these provide full-fit display plus zoom/pan controls for Mermaid and D2 diagrams.

The goal is that the same `.slide` document automatically looks balanced on projector screens, browser windows, and fullscreen mode.

## How to create a new template

1. Pick a lowercase name: `customer-brand`.
2. Create `.tt/slide/templates/customer-brand/`.
3. Add `template.json` with defaults.
4. Add `template.css` with variables and normal slide styling.
5. Put assets under `assets/` and reference them with `url("./assets/...")`.
6. Run:

```bash
tt slide --list-templates
tt slide deck.slide --template customer-brand
```

7. Check normal slides, first slide, `.center`, `.two-column`, `.brand`, `.end`, code blocks, tables, diagrams, and ESC overview.

## Updating MagiCloud

The MagiCloud template lives at:

```text
.tt/slide/templates/magicloud/
├── template.json
├── template.css
└── assets/
```

When changing it:

- Keep the `.slide` syntax template-agnostic.
- Put all MagiCloud-specific colors, logos, mesh backgrounds, and typography in `template.css` / `assets/`.
- Use `tt slide --template magicloud` to inspect.
- Use `tt slide --list-templates` to confirm it resolves as `project`.

## Validation checklist

After template work:

```bash
go test ./cmd
cd web && npm run build:slide
tt slide --list-templates
```

Review before finishing:

1. `tt slide --list-templates` shows the template with the expected source.
2. `template.json` parses and points to the right CSS file.
3. Every `url("./assets/...")` asset exists.
4. No CSS references absolute local paths or `../` traversal.
5. Normal slides, cover, brand, and closing pages remain readable.
6. Code blocks, tables, diagrams, and images remain readable.
7. ESC overview thumbnails are recognizable.
8. The template does not require `.slide` authors to write template-specific syntax.

## Common mistakes

- Putting `template:` in `.slide` front matter instead of using `--template`.
- Putting template assets next to the `.slide` deck instead of in `.tt/slide/templates/<name>/assets/`.
- Referencing assets with absolute paths.
- Styling only `.present` sections and leaving overview thumbnails blank.
- Adding a new directive only in CSS without parser/type/docs support.
- Using overly broad selectors that break Reveal controls or the file/overview panels.
- Forgetting to run `tt slide --list-templates` after adding a new template.

## Boundary with slide-writer

Use `slide-template-writer` for template packages, CSS, assets, visual design, and template-specific behavior.

Use `slide-writer` for `.slide` document content and syntax. The writer should not put visual implementation details into `.slide` files.

## Handoff

When this skill has just been added or updated in the current session, tell the user that new sessions may need to reload skills before it is automatically selected.
