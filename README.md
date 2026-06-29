# tt

`tt` is a collection-style CLI toolkit for local task workflows. It provides small, focused commands for browsing/editing structured files, rendering markdown, inspecting configuration, running embedded Picoclaw agent workflows, and converting CLI commands to skill files.

## Features

- Local web UI for Markdown files with live reload.
- Local web UI for JSON files with formatted preview and editing.
- Local web UI for conversation-style JSON transcripts.
- Formula dashboard final report view with a dedicated follow-up coder chat.
- Light/dark theme switching for the formula dashboard and markdown web UI, with local preference persistence.
- Embedded Picoclaw agent runtime command.
- Embedded professional stock research discussion command with streamed turns and JSON archive.
- CLI command to skill file conversion.
- Directory mirroring for config sharing.
- LLM-backed commands show an elapsed loading status on terminal stderr while waiting for model responses.
- Global and project-level JSON configuration.

## Installation

### Build locally

```bash
git clone <repo-url>
cd tt
make build
./tt --help
```

### Install to user bin

By default this installs to `~/.local/bin/tt`:

```bash
make install
```

You can override the prefix:

```bash
make install PREFIX=/usr/local
```

### Run without installing

```bash
go run . --help
```

> Note: this repository currently uses a local Go module replacement for `github.com/sipeed/picoclaw`:
>
> ```go
> replace github.com/sipeed/picoclaw => ../picoclaw
> ```
>
> Make sure the sibling `../picoclaw` checkout exists. The CI workflow checks out <https://github.com/sjzsdu/picoclaw> at that sibling path so the local replacement resolves consistently.

## Usage

```bash
tt [command]
```

Available commands:

| Command | Description |
| --- | --- |
| `agent` | Run the embedded Picoclaw agent runtime and optimize embedded agents for a target repository. |
| `formula` | Author, validate, schedule, run, and inspect graph-first formula workflows in CLI and local dashboard flows. |
| `cmd2skill` | Convert CLI commands into skill files. |
| `repo2skill` | Convert repositories into agent-oriented library skills. |
| `config` | Inspect and initialize `tt` configuration. |
| `conversation` | Browse conversation-like JSON in a local web UI. |
| `debate` | Run a professional stock research discussion between embedded investor agents, then save the full JSON transcript. |
| `json` | Browse and edit JSON files in a local web UI. |
| `markdown` | Browse Markdown files in a local web UI. |
| `mirror` | Mirror selected files from a source directory. |
| `nvwa` | Generate role-specific `Agent.md` and `soul.md` prompts with an embedded LLM prompt designer. |
| `skill` | Browse and edit skill Markdown files. |
| `version` | Print the `tt` version. |

Use built-in help for full flag details:

```bash
tt --help
tt <command> --help
```

## Commands

### `tt markdown`

Start a local web service for browsing Markdown files in the current working tree. The UI is a React/Vite single-page app embedded into the Go binary at build time, while Go owns the local file APIs and websocket reloads.

```bash
tt markdown
tt markdown README.md
tt markdown docs --port 9595
tt markdown --pattern "**/*.md"
tt markdown --content "# Hello" --content-only
```

For development, run `make web-build` to rebuild embedded web assets or `cd web && npm run dev:markdown` for the Vite dev server.

Theme support:

- Use the light/dark theme toggle in the left file pane to switch the UI theme.
- The selected theme is saved in browser `localStorage` and restored on refresh.
- The markdown UI theme also drives Mermaid rendering and the main content/editor surfaces so diagrams and prose stay readable in both modes.

Flags:

- `-p, --port int`: service port, default `9595`.
- `-c, --content string`: render provided Markdown content directly.
- `--content-only`: only show provided Markdown content.
- `-f, --pattern strings`: filter Markdown files by glob patterns.

### `tt json`

Start a local web service for browsing and editing JSON files.

```bash
tt json
tt json data/config.json
tt json ~/projects/sample-data
tt json --file data/config.json --port 9696
```

Flags:

- `-p, --port int`: service port, default `9696`.
- `-f, --file string`: open a specific JSON file.

### `tt conversation`

Start a local web service for browsing JSON files that contain conversation-style message flows.

```bash
tt conversation
tt conversation logs/*.json
tt conversation --file session.json
tt conversation --pattern "**/*.json" --port 9680
```

Flags:

- `-p, --port int`: service port, default `9680`.
- `-f, --file string`: open a specific JSON file.
- `--pattern strings`: filter JSON files by glob patterns.

### `tt formula`

Author, validate, compile, run, and inspect formula workflows. See `docs/formula/multi-run-product-shape.md` for the current product-shape research on running multiple formulas together with unified web monitoring. For the phase-1 decision record that narrows this into a concrete MVP boundary, see `docs/formula/mvp-direction-and-phase-boundaries-for-multi-run.md`. For the matching validation skeleton that defines the minimum scenario matrix for this MVP direction, see `docs/formula/minimal-validation-scenario-matrix-for-multi-run-mvp.md`. The dashboard final report view now includes a dedicated follow-up chat that starts a separate `coder` session and uses the final report as chat context.

Final report chat behavior:

- Open **Final report** after a run completes to view the report and the follow-up chat in the same modal.
- Click **Start chat** to create or recover a run-scoped session named like `<run-id>:final-report-chat`.
- The chat is fixed to the embedded `coder` agent for this feature; agent switching is not part of this flow.
- The current final report is injected as context for the chat, and subsequent turns continue in the same derived session.
- Chat history is persisted inside the formula snapshot, so it can reappear after dashboard refresh or reconnect.
- If no final report exists, the report view still opens, but the chat panel shows an unavailable state instead of blocking report access.

Scheduled runs:

- Use `tt formula schedule <name> --every 2m` to run a formula repeatedly in the foreground on a fixed interval.
- Use `tt formula schedule <name> --cron "*/2 * * * *"` for crontab-style scheduling. Quote the cron expression so your shell does not expand `*`.
- Scheduled runs default to no live dashboard so unattended jobs do not block between runs. Pass `--web` to enable the dashboard for each run.
- Add `--run-now` to execute immediately before waiting for the first scheduled tick, `--max-runs N` to stop after N runs, and `--stop-on-error` to halt after a failed run.

Examples:

```bash
tt formula schedule lark-auto-reply --every 2m --run-now
tt formula schedule lark-auto-reply --cron "*/2 * * * *"
tt formula schedule nightly-report --cron "0 9 * * 1-5" --var team=backend
```

Lark auto-reply formula:

```bash
# Preview only. Searches @me and P2P messages visible to lark-cli user identity.
tt formula run lark-auto-reply --var mode=dry-run

# Run every 2 minutes in dry-run mode.
tt formula schedule lark-auto-reply --every 2m --var mode=dry-run

# Low-cost watch loop: polls every 30s inside one formula run; calls agent only after a hit.
tt formula run lark-auto-reply-watch --no-web --var mode=dry-run

# Recommended: describe your role, responsibilities, repos, and reply style.
cp internal/formula/builtin/formulas/lark/persona-template.md ~/.tt/persona.md
# Optional override if you want a different file:
export TT_LARK_PERSONA_CONTEX=/path/to/persona.md

# Tune polling window/interval. Default max_polls=2880 is about 24h at 30s.
tt formula run lark-auto-reply-watch --no-web \
  --var poll_interval_seconds=30 \
  --var max_polls=2880 \
  --var mode=dry-run

# Restrict to specific chats and actually reply. Use with care.
tt formula schedule lark-auto-reply --every 2m \
  --var mode=auto \
  --var chat_ids=oc_xxx,oc_yyy \
  --var self_open_id=ou_xxx \
  --var project_context=.tt/lark-auto-reply/context.md
```

`lark-auto-reply` uses `lark-cli im +messages-search --is-at-me` by default, runs a reply gate against persona context using priority `TT_LARK_PERSONA_CONTEX` existing file, then `~/.tt/persona.md` if present, otherwise no persona file, plus `.tt/lark-auto-reply/context.md`, drafts a reply with the `coder` agent only when the gate allows it, and sends through `lark-cli im +messages-reply` only when `mode=auto`. `lark-auto-reply-watch` keeps the cheap search step in an internal loop and only calls the gate/reply agents after a relevant message is found. P2P search is disabled by default because broad direct-message search can time out on the Lark server; enable it with `--var include_direct=true` together with `--var chat_ids=oc_xxx`. It stores processed message IDs in `.tt/lark-auto-reply/state.json` by default to avoid duplicate replies.

Reply personalization and gate files:

- Persona file: optional. Load priority is `TT_LARK_PERSONA_CONTEX` when it points to an existing file, then `~/.tt/persona.md` if present, otherwise no persona file. `--var persona_context=...` remains available as a manual fallback.
- `.tt/lark-auto-reply/context.md`: project-specific context, such as modules, current ownership, known issues, and safe troubleshooting guidance.
- Override project context with `--var project_context=...`.

Self-repair (StepFixer) behavior:

- Failed or validation-failed steps go through a `StepFixer` abstraction (`agentFixer` / `scriptFixer`) and may be retried up to 3 attempts.
- Whether a step may be retried is controlled by the `idempotent` flag on the step. `agent` and `external_agent` default to `true`; `script` defaults to `false` and must opt in with `idempotent = true`. `tool` / `aggregate` / `write_files` / `noop` / `human_input` / `loop` / `retry` are not in the fix path.
- Each attempt writes a `RepairRecord` (step, kind, attempt, status, reason, advice, formula update hint, original/fixed command, error, confirmation status) to the run store.
- Repair reports are persisted to `patches/<run-id>.json` (separate from `state.json` so they can be `git diff`-ed or applied later) and surfaced in the dashboard `Repairs` panel.
- The dashboard exposes a `Confirm reviewed` button for each repair; the runtime never auto-patches the formula file. The `FormulaUpdateHint` is a suggestion; the author decides when to apply changes.
- For a step-level overview see the `Self-Repair` section in `ai-docs/formula-system.md` and the `Self-Repair (StepFixer)` section in the `formula-writer` skill.

Current limitation:

- Because chat messages are stored in the snapshot and broadcast through the dashboard state, very long follow-up chats increase snapshot size and websocket payload size.

Theme support:

- Use the light/dark theme toggle in the dashboard header to switch the formula UI theme.
- The selected theme is saved in browser `localStorage` and restored on refresh.
- The theme source is shared across Ant Design components, the final report markdown view, Mermaid diagrams, and the React Flow graph panel/minimap so the main execution surfaces stay visually aligned.

### `tt agent`

Run the embedded Picoclaw agent runtime and reuse existing Picoclaw configuration, models, sessions, and skills without invoking the `picoclaw` binary.

```bash
tt agent -m "summarize this project"
tt agent "explain the current directory"
tt agent --session cli:tt --model gpt-5.4 -m "review this idea"
tt agent --picoclaw-home ~/.picoclaw-dev -m "list available skills"
```

Alias:

```bash
tt pc ...
```

Flags:

- `-m, --message string`: send a single message to the agent.
- `-s, --session string`: session key, default `cli:default` unless configured.
- `--agent string`: agent id or name to use.
- `--model string`: model override.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

#### `tt agent optimize`

Analyze a local or remote repository and optimize an existing embedded-agent Markdown file for that repository. The target can be a library, application, CLI, service, or full-stack product codebase. By default the source agent file is updated in place. Use `--copy` to create a new optimized agent next to the source agent instead.

First version limitation: repository input only. Website ingestion is not implemented yet.

```bash
tt agent optimize --target ./repo --agent .tt/agents/custom.md
tt agent optimize --target github.com/gin-gonic/gin --agent .tt/agents/custom.md --copy
tt agent optimize --target ./repo --agent coder --copy
```

Flags:

- `--target string`: target repository path or cloneable URL.
- `--agent string`: base agent id or local `.md` embedded-agent file. File-backed agents are updated in place by default.
- `--copy`: create a new optimized agent next to the source agent instead of updating it in place. Required when optimizing built-in agents such as `coder`.
- `-o, --output string`: advanced override to write output to an explicit file or existing directory.
- `-f, --force`: overwrite an existing copied or explicit output file.
- `--session string`: session key for optimization, default `cli:agent-optimize`.
- `--model string`: model override.
- `--max-files int`: maximum relevant files to collect, default `200`.
- `--max-file-size int`: maximum bytes per collected file, default `262144`.
- `--max-prompt-chars int`: maximum optimized prompt size, default `12000`, used to prevent repeated distillation from bloating an agent.
- `--timeout duration`: timeout for repository preparation and optimization, default `2m`.
- `--keep-temp`: keep temporary cloned repositories for debugging.
- `-d, --debug`: enable debug logging.

#### `tt agent info`

Show resolved agent runtime information as JSON:

```bash
tt agent info
tt agent info --picoclaw-home ~/.picoclaw-dev
```

### `tt translate`

Translate Chinese and English text using the embedded Picoclaw translate-master agent. This command depends on Picoclaw config and models, defaulting to `~/.picoclaw/config.json` unless overridden.

```bash
tt translate "Hello, world"
echo "你好，世界" | tt translate
tt translate --target ja "你好，世界"
tt translate --model gpt-5.4 "Improve developer productivity"
```

Flags:

- `--target string`: target language override, such as `zh`, `en`, `ja`, `ko`, `fr`.
- `--model string`: model override. Defaults to the Picoclaw default model.
- `-s, --session string`: session key, default `cli:translate`.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

### `tt debate`

Run a stock-focused professional research discussion between embedded investor agents. The default participants are a fundamental analyst and a risk controller, with a stock research host coordinating the discussion. Additional embedded roles include macro strategy, quantitative technical analysis, news/event analysis, and sector research. Turns are printed as soon as each model call completes, while the full structured result is always saved as JSON under `./debates` unless `--out` is provided.

```bash
tt debate "贵州茅台接下来半年怎么看"
tt debate --topic "英伟达估值是否还能支撑上涨" --rounds 4
tt debate "比亚迪现在是机会还是风险" --out debates/byd.json
```

The embedded agent definitions live in `internal/agents/embedded/<category>/*.md` as Markdown files with YAML frontmatter. The `internal/agents` package loads them into Picoclaw `EmbeddedAgent` values so future embedded agents can be added without hardcoding large prompts in command files.

The embedded stock agents are configured with `tongstock-cli` and `agent-browser` skills, and research tools such as web/search/fetch/exec are enabled in the cloned Picoclaw runtime config. This command depends on Picoclaw config and models, defaulting to `~/.picoclaw/config.json` unless overridden.

Flags:

- `-t, --topic string`: stock discussion topic. Positional args are also supported.
- `--agents strings`: optional two stock research agent ids or names. Defaults to embedded fundamental analyst / risk controller agents. Other available embedded roles include `stock-macro-strategist`, `stock-quant-technician`, `stock-news-event-analyst`, and `stock-sector-specialist`.
- `--judge string`: optional host agent id or name. Defaults to embedded stock discussion host for structured archival metadata.
- `-r, --rounds int`: maximum number of visible investor turns, default `3`.
- `-o, --output string`: output format, `text` or `json`, default `text`. Text mode streams visible investor turns; JSON mode also prints the final JSON.
- `--out string`: write full JSON result to a file. When omitted, JSON is auto-saved to `./debates/stock-discussion-<timestamp>.json`.
- `-s, --session string`: session key prefix, default `cli:debate` unless configured.
- `--model string`: model override for all participants.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

### `tt nvwa`

Generate OpenClaw/Picoclaw-style prompt content for a professional role. `nvwa` calls an embedded prompt-designer agent through the configured Picoclaw model, so the result is role-specific rather than a fixed template. By default it writes generated files and asks the model for a standard-length prompt: `Agent.md` about 900-1400 Chinese characters and `soul.md` about 400-700 Chinese characters. Set `--write=false` to print to stdout. Use `--style embedded` to output the same YAML-frontmatter Markdown format used by `internal/agents/embedded/<category>/*.md`.

```bash
tt nvwa 前端开发工程师
tt nvwa 产品经理 --context "偏增长型 SaaS"
tt nvwa "Go 后端工程师" --output .agents/go-backend
tt nvwa 数据分析师 --write=false --format agent --model gpt-5.4
tt nvwa 前端开发工程师 --style embedded --id frontend-engineer --skill agent-browser
```

Flags:

- `-w, --write`: write generated file(s), default `true`; set `--write=false` to print to stdout.
- `-o, --output string`: output directory when writing files, default `.`.
- `-f, --force`: overwrite existing files when writing.
- `--format string`: generate `agent`, `soul`, or `both`, default `both`. Only `both` is supported with `--style embedded`.
- `--style string`: output style, `files` or `embedded`, default `files`.
- `--id string`: embedded agent id when using `--style embedded`; defaults to an ASCII slug or `nvwa-agent`.
- `--name string`: embedded agent display name; defaults to the role.
- `--skill string`: embedded agent skill; repeat or comma-separate for multiple skills.
- `--no-history`: set `no_history: true` in embedded output.
- `--research-tools`: include `skills`, `find_skills`, `web_search`, `web_fetch`, and `exec` in the embedded agent `tools` allowlist.
- `--context string`: extra role context, target scenario, style, or constraints.
- `--model string`: model override. Defaults to the Picoclaw default model.
- `-s, --session string`: session key, default `cli:nvwa`.
- `--picoclaw-home string`: override `PICOCLAW_HOME` for this run.
- `--picoclaw-config string`: override `PICOCLAW_CONFIG` for this run.
- `-d, --debug`: enable debug logging.

### `tt cmd2skill`

Parse a CLI command and its subcommands into a structured command tree, then generate agent-oriented skill files with usage, command references, safety guidance, and deterministic output.

```bash
tt cmd2skill git
tt cmd2skill git --depth 2
tt cmd2skill docker --examples
tt cmd2skill kubectl --target-dir ./.forge/skills
tt cmd2skill git --markdown
```

Flags:

- `--target-dir string`: directory to write skill files, default `~/.agents/skills`.
- `--dry-run`: print skill content to stdout instead of writing files.
- `--examples`: include examples extracted from help output.
- `-d, --depth int`: recursion depth for subcommand help (0 = top-level only), default `2`. This keeps output files at the second command level while embedding discovered deeper command help inside the parent reference.
- `--timeout duration`: timeout for each help command, default `5s`.
- `--max-commands int`: maximum number of command help pages to discover, default `200`.
- `--markdown`: open generated skill content directly with markdown command instead of writing files.


### `tt repo2skill`

Analyze a local or remote repository and generate an agent-oriented skill for using that library in development. The generated skill emphasizes repository purpose, installation hints, public API starting points, documented recipes, best practices, avoid rules, and validation notes. It has first-class deterministic collector coverage for TypeScript/JavaScript, Python, Go, and Rust repository layouts.

```bash
tt repo2skill ./my-library
tt repo2skill https://github.com/colinhacks/zod
tt repo2skill github.com/gin-gonic/gin --dry-run
tt repo2skill ./repo --analyzer agent --model gpt-5.4
tt repo2skill ./repo --target-dir ./.agents/skills
```

Flags:

- `--target-dir string`: directory to write skill files, default `~/.agents/skills`.
- `--dry-run`: print generated skill content to stdout instead of writing files.
- `--markdown`: open generated skill content directly with the markdown command.
- `--intent string`: skill intent, default `use-library`. Other planned intents include `contribute`, `api-reference`, and `architecture`.
- `--language string`: preferred output language hint for future agent analysis.
- `--max-files int`: maximum relevant files to collect, default `200`.
- `--max-file-size int`: maximum bytes per collected file, default `262144`.
- `--timeout duration`: timeout for git clone and analysis steps, default `2m`.
- `--keep-temp`: keep cloned temporary repository for debugging.
- `--include-evidence`: write `references/evidence.md` and link it from `SKILL.md` for audit/debugging. Omitted by default to keep coding-agent skills focused.
- `--analyzer string`: analysis mode, `auto`, `agent`, or `heuristic`, default `auto`. Auto uses the embedded Picoclaw `repo2skill` agent when available and falls back to deterministic heuristics.
- `--model string`: Picoclaw model override for agent analysis.
- `--session string`: Picoclaw session key for agent analysis, default `cli:repo2skill`.
- `-d, --debug`: enable debug logging for agent analysis.

Generated structure:

```text
skills/<repo-name>/
  SKILL.md
  references/
    api.md
    recipes.md
    evidence.md  # only with --include-evidence
```

### `tt mirror`

Mirror keeps a project-local directory in sync with a fuller source directory. It is useful for sharing and selectively importing tool configs such as opencode agents and commands.

```bash
tt mirror source
tt mirror target
tt mirror apply
tt mirror apply agents.foo commands.bar
tt mirror prune
tt mirror config
tt mirror config --set-source-dir ~/.config/opencode-full
```

Subcommands:

| Subcommand | Description |
| --- | --- |
| `source` | Show source directory tree. |
| `target` | Show target directory tree. |
| `apply` | Mirror all or selected keys from source to target. |
| `prune` | Remove mirrored target entries. |
| `config` | Show or set mirror paths in project tt config. |

Flags:

- `--source-dir string`: source directory path (default `~/.config/opencode-full`).
- `--target-dir string`: target directory path (default `.opencode`).
- `--config-file string`: config file name (default `opencode.json`).

`tt mirror source` / `tt mirror target`:
- `-l, --level int`: show items up to this depth level, 0 for all.

`tt mirror config`:
- `--set-source-dir string`: set source directory path.
- `--set-target-dir string`: set target directory path.
- `--set-config-file string`: set config file name.

### `tt config`

Inspect resolved configuration paths, initialize config files, or show merged configuration.

```bash
tt config
tt config --show
tt config --init-global
tt config --init-project
```

Flags:

- `--show`: show merged `tt` configuration.
- `--init-global`: create `~/.tt/config.json` if missing.
- `--init-project`: create project `.tt/config.json` if missing.

### `tt version`

```bash
tt version
```

## Configuration

`tt` loads configuration from two levels:

1. Global config: `~/.tt/config.json`, or `TT_CONFIG` when set.
2. Project config: nearest project `.tt/config.json`, or `TT_PROJECT_CONFIG` when set.

Project configuration overrides global configuration.

Example:

```json
{
  "picoclaw": {
    "home": "~/.picoclaw",
    "config": "~/.picoclaw/config.json"
  },
  "agent": {
    "session": "cli:default",
    "agent": "default",
    "model": "gpt-5.4",
    "debug": false
  },
  "debate": {
    "rounds": 3,
    "output": "text"
  },
  "markdown": {
    "port": 9595,
    "content_only": false,
    "patterns": ["**/*.md"]
  },
  "conversation": {
    "port": 9680,
    "patterns": ["**/*.json"]
  },
  "mirror": {
    "source_dir": "~/.config/opencode-full",
    "target_dir": ".opencode",
    "config_file": "opencode.json"
  }
}
```

Initialize a starter config:

```bash
tt config --init-global
tt config --init-project
```

## Releases

When `cmd/version.go` changes on `main`, GitHub Actions builds Linux, macOS, and Windows binaries for `amd64` and `arm64`, then publishes them to a GitHub Release tagged with the version, for example `v0.1.0`.

## Development

Frontend theme implementation notes:

- `web/apps/formula` and `web/apps/markdown` both use the same minimal theme pattern: root-level light/dark state, `document.documentElement.dataset.theme`, and browser `localStorage` persistence.
- Ant Design theme algorithms are switched from the same theme state used by app-level CSS variables, instead of maintaining a separate component-library theme source.
- Mermaid rendering is re-initialized from the active app theme so diagrams match the surrounding UI.
- The current implementation intentionally favors CSS variables and targeted overrides over a full design-system migration.

Common targets:

Common targets:

```bash
make build          # Build ./tt
make install        # Install to $(PREFIX)/bin/tt, default ~/.local/bin/tt
make install-system # Install to /usr/local/bin/tt
make clean          # Remove local binary
make run            # Run with go run
make fmt            # Format Go files
make tidy           # Tidy Go modules
```

Run checks:

```bash
go test ./...
```

## Project layout

```text
.
├── cmd/                 # Cobra commands
├── internal/dirmirror/  # Directory mirror utilities
├── internal/mdutil/     # Markdown utilities
├── internal/picoclaw/   # Embedded Picoclaw integration
├── internal/ttconfig/   # tt config loading and merging
├── internal/webui/      # Web UI templates
├── web/                 # Web frontend assets
├── main.go              # CLI entrypoint
├── Makefile
├── go.mod
└── go.sum
```

## License

No license file is currently included in this repository.
