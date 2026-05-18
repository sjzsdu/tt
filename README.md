# tt

`tt` is a collection-style CLI toolkit for local task workflows. It provides small, focused commands for browsing/editing structured files, rendering markdown, inspecting configuration, running embedded Picoclaw agent workflows, and converting CLI commands to skill files.

## Features

- Local web UI for Markdown files with live reload.
- Local web UI for JSON files with formatted preview and editing.
- Local web UI for conversation-style JSON transcripts.
- Local web UI for skill Markdown files with frontmatter support.
- Embedded Picoclaw agent runtime command.
- Embedded casual stock investor chat command with streamed turns and JSON archive.
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
| `agent` | Run the embedded Picoclaw agent runtime. |
| `cmd2skill` | Convert CLI commands into skill files. |
| `config` | Inspect and initialize `tt` configuration. |
| `conversation` | Browse conversation-like JSON in a local web UI. |
| `debate` | Run a casual stock chat between an investing beginner and a market old hand, then save the full JSON transcript. |
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

### `tt skill`

Start a local web UI for skill files. It extracts frontmatter, renders the remaining Markdown body, and supports editing plus saving the full document.

```bash
tt skill
tt skill create-cmd
tt skill --file .forge/skills/create-cmd/SKILL.md --edit
tt skill --root ~/.config/skills
```

Flags:

- `-p, --port int`: service port, default `9695`.
- `-f, --file string`: open a specific skill Markdown file.
- `--root string`: override the skill root directory.
- `--edit`: open the current document in edit mode by default.

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

Run a stock-focused casual chat between embedded investor agents. The default participants are an optimistic investing beginner and an experienced market old hand. Their turns are printed as soon as each model call completes, while the full structured result is always saved as JSON under `./debates` unless `--out` is provided.

```bash
tt debate "贵州茅台接下来半年怎么看"
tt debate --topic "英伟达估值是否还能支撑上涨" --rounds 4
tt debate "比亚迪现在是机会还是风险" --out debates/byd.json
```

The embedded agent definitions live in `internal/agents/embedded/*.md` as Markdown files with YAML frontmatter. The `internal/agents` package loads them into Picoclaw `EmbeddedAgent` values so future embedded agents can be added without hardcoding large prompts in command files.

The embedded stock agents are configured with `tongstock-cli` and `agent-browser` skills, and research tools such as web/search/fetch/exec are enabled in the cloned Picoclaw runtime config. This command depends on Picoclaw config and models, defaulting to `~/.picoclaw/config.json` unless overridden.

Flags:

- `-t, --topic string`: stock discussion topic. Positional args are also supported.
- `--agents strings`: optional two investor agent ids or names. Defaults to embedded investing beginner / market old hand agents.
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

Generate OpenClaw/Picoclaw-style prompt content for a professional role. `nvwa` calls an embedded prompt-designer agent through the configured Picoclaw model, so the result is role-specific rather than a fixed template. By default it writes generated files; set `--write=false` to print to stdout. Use `--style embedded` to output the same YAML-frontmatter Markdown format used by `internal/agents/embedded/*.md`.

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
- `--research-tools`: set `enable_research_tools: true` in embedded output.
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