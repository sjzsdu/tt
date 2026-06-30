---
name: slide-template-writer
description: Design, implement, and refine tt slide templates in web/apps/slide/src/templates/index.ts. Use when changing MagiCloud or other reveal.js slide themes, adding page directives, tuning typography, colors, backgrounds, transitions, overview behavior, or creating a new slide template.
license: MIT
---

# Slide Template Writer

Use this skill when implementing or refining `tt slide` templates.

A template is not only a color palette. It is a coordinated system of:

- Reveal.js defaults
- CSS variables
- typography
- page-level directives
- background layers
- content layouts
- overview behavior
- assets embedded by Vite

## Current architecture facts

- Template definitions live in `web/apps/slide/src/templates/index.ts`.
- Template type definitions live in `web/apps/slide/src/types/index.ts`.
- Slide parsing and page directives live in `web/apps/slide/src/parser.ts`.
- Global app styles live in `web/apps/slide/src/styles.css`.
- Slide React app lives in `web/apps/slide/src/components/SlideApp.tsx`.
- MagiCloud assets live under `web/apps/slide/src/assets/magicloud/`.
- Default template is `magicloud`.
- `tt slide --template NAME` and `tt slide -t NAME` override the runtime template.
- User/project templates should use `.tt/slide/templates/<name>/` as the external template package shape when that loader is implemented.

## Template implementation workflow

1. Inspect existing `TemplateConfig` shape before editing.
2. Decide whether the change is:
   - a CSS-only template change;
   - a parser directive change;
   - a React behavior change;
   - a global style change.
3. Keep template-specific visuals inside `templates/index.ts` whenever possible.
4. Keep generic controls and overview styles in `styles.css`.
5. Run `cd web/apps/slide && npm run build` after any template or frontend change.
6. Run `go test ./cmd` when changing CLI flags, file handling, or command behavior.

## TemplateConfig shape

A template has this form:

```ts
const example: TemplateConfig = {
  name: 'example',
  revealTheme: 'white',
  css: `
    :root {
      --slide-bg: #ffffff;
      --slide-fg: #1f2329;
      --slide-accent: #008D55;
    }

    .reveal .slides section {
      background: var(--slide-bg);
      color: var(--slide-fg);
    }
  `,
  defaults: {
    theme: 'light',
    transition: 'fade',
    center: false,
    width: 1600,
    height: 900,
    margin: 0,
  },
};
```

Register it in the template map at the bottom of `templates/index.ts`.

## `.tt` project template package shape

When designing user-defined templates, keep the package self-contained under `.tt`:

```text
.tt/slide/templates/<template-name>/
├── template.json
├── template.css
└── assets/
    ├── cover-bg.png
    ├── logo-dark.png
    └── logo-white.svg
```

`template.json` should mirror the frontend `TemplateConfig` shape without embedding CSS inline:

```json
{
  "name": "customer-brand",
  "revealTheme": "white",
  "css": "template.css",
  "defaults": {
    "theme": "light",
    "transition": "fade",
    "center": false,
    "width": 1600,
    "height": 900,
    "margin": 0
  }
}
```

`template.css` owns all visual styling and references assets by relative URL:

```css
.reveal:has(.slides section:first-child.present)::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: url("./assets/cover-bg.png") center / cover no-repeat;
}

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

Asset rules:

1. Template-specific backgrounds, logos, textures, and decorative images go in `assets/` beside the template.
2. CSS should use relative paths like `url("./assets/cover-bg.png")`; do not use absolute local filesystem paths.
3. Business images referenced from `.slide` content are not template assets and should stay next to the `.slide` deck or in a deck-local image folder.
4. A template package must not require edits to `.slide` documents. `.slide` documents remain semantic and template-agnostic.
5. Project `.tt/slide/templates/<name>/` should take precedence over global `~/.tt/slide/templates/<name>/` if both exist.

Security/serving rule for implementation: serve only files inside the selected template directory, rewrite relative CSS asset URLs to the slide server asset endpoint, and reject `..` traversal or absolute paths.

## Adding a page directive

A new directive usually requires three edits:

1. `web/apps/slide/src/types/index.ts`
   - Add the layout name to `SlideLayout`.
2. `web/apps/slide/src/parser.ts`
   - Add the directive to `slideDirectivePattern`.
   - Map directive text to `layoutHint`.
   - Map layout to CSS class in `classForLayout`.
3. `web/apps/slide/src/templates/index.ts`
   - Add CSS for the generated class, e.g. `.slide-closing`.

Example directive mapping:

```ts
} else if (directive === 'closing' || directive === 'end' || directive === 'final') {
  layoutHint = 'closing';
}
```

Example class mapping:

```ts
if (layout === 'closing') return 'slide-closing';
```

## Background layers and transitions

Avoid putting full-screen moving backgrounds only on the slide section when the transition moves slides vertically or horizontally. It can produce visible cut edges.

Preferred pattern for special full-screen backgrounds:

1. Put the active background on a fixed `.reveal::before` layer using `:has(.slides section.some-class.present)`.
2. Fade the layer with `opacity` transition.
3. Make the actual section background transparent during normal playback.
4. Preserve section backgrounds in `.reveal.overview` so thumbnails remain recognizable.

Pattern:

```css
.reveal::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  opacity: 0;
  transition: opacity 260ms ease;
}

.reveal:has(.slides section.slide-closing.present)::before {
  opacity: 1;
  background:
    url("...") center / cover no-repeat,
    linear-gradient(135deg, #000000 0%, #00130D 20%, #00643C 58%, #008D55 100%);
}

.reveal:not(.overview) .slides section.slide-closing {
  background: transparent;
}
```

## MagiCloud visual rules

For MagiCloud, align with the PPT template direction:

- Primary greens: `#00643C`, `#00633B`, `#008D55`.
- Body gray: `#535E59` / `#595959`.
- Soft surfaces: `#F4F4F4`, borders `#DEE0E3`.
- Font stack: `Aptos`, `Aptos Display`, `Segoe UI`, `Arial`, `PingFang SC`, `Noto Sans SC`.
- Use clean rectangular surfaces more than heavy rounded cards.
- Keep normal page titles slightly below the logo/header area.
- Keep body text smaller, lighter, and more spacious than headings.

## Overview behavior

ESC overview should be a centered, fixed-size horizontal scroller:

- The overview container is fixed and visually floats above the current slide.
- The container scrolls horizontally.
- It must not alter or stretch the underlying page layout.
- Avoid CSS rules that override inline positioning from `SlideApp.tsx`, especially `inset: ... !important` on `.reveal.overview`.
- Always reset inline `transform`, `width`, `height`, `position`, and scroll state when overview exits.

## Validation checklist

After template work:

```bash
cd web/apps/slide && npm run build
```

If CLI flags or Go handlers changed:

```bash
go test ./cmd
```

Review these before finishing:

1. Template compiles with Vite.
2. No unsupported asset path is referenced.
3. Normal slides, cover slides, logo pages, and `.end` pages all remain readable.
4. ESC overview stays centered and does not affect the underlying page.
5. `--template` / `-t` still works if template names changed.
6. New directives are documented in `ai-docs/slide.md` when user-facing.

## Common mistakes

- Do not add a page directive only in CSS; the parser must emit the matching class.
- Do not use `.md` examples for slide decks; use `.slide`.
- Do not put global button/control styles inside one template unless they are template-specific.
- Do not use `!important` on layout properties unless necessary; it can fight inline overview positioning.
- Do not forget to preserve overview thumbnail recognizability when moving backgrounds to fixed layers.

## Handoff

When this skill has just been added or updated in the current session, tell the user that new sessions may need to reload skills before it is automatically selected.
