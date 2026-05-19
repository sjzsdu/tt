---
name: "repo2skill"
description: "Convert a local or remote repository into an agent-oriented library skill for development usage, public API guidance, recipes, and evidence-backed references."
---

# Repository to Skill Converter

Use `tt repo2skill` when a user wants to turn a repository, GitHub URL, or local library checkout into a reusable agent skill for coding with that library.

## Usage

```bash
tt repo2skill [repo-path-or-url] [flags]
```

Examples:

```bash
tt repo2skill ./my-library
tt repo2skill https://github.com/colinhacks/zod
tt repo2skill github.com/gin-gonic/gin --dry-run
tt repo2skill ./repo --analyzer agent --model gpt-5.4
tt repo2skill ./repo --target-dir ./.agents/skills
```

## What it generates

```text
skills/<repo-name>/
  SKILL.md
  references/
    api.md
    recipes.md
    evidence.md
```

The generated skill is optimized for the `use-library` intent: help an agent understand what the repo solves, how to install it, which public APIs and entrypoints to start from, and which documented snippets/recipes are safe to reuse.

## Analyzer modes

- `--analyzer auto` uses the embedded Picoclaw `repo2skill` agent when configuration/model access is available, then falls back to deterministic heuristics.
- `--analyzer agent` requires Picoclaw and the embedded `repo2skill` agent. Use `--model` to override the model.
- `--analyzer heuristic` disables LLM analysis and uses only deterministic extraction.

## Current implementation

- Resolves local directories, full Git URLs, and GitHub shorthand.
- Collects package metadata from `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, and common build files.
- Reads README, docs, examples, tests, and language entrypoints.
- Extracts code snippets and public symbols using deterministic heuristics, including TypeScript re-exports and Python `__all__` patterns.
- Normalizes analyzer output, drops empty install hints, flags agent-suggested APIs that deterministic extraction did not verify, and renders validation notes in `evidence.md`.
- Renders `SKILL.md` plus API, recipes, and evidence references.
- Includes an embedded `repo2skill` Picoclaw agent for evidence-constrained library skill synthesis.

## Agent guidance

- Treat generated output as evidence-backed starting guidance, not a complete upstream manual.
- Prefer public exports, package entrypoints, README examples, and docs over internal implementation files.
- If the generated skill lacks an API or recipe, inspect the upstream repository before coding.
