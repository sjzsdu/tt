---
name: "cmd2skill"
description: "Convert any CLI command into a skill file. Invoke when user wants to document a command, generate a skill from a CLI tool (git, docker, kubectl, etc.), or create LLM-friendly command reference."
---

# Command to Skill Converter

Convert any CLI command into a SKILL.md file that helps an LLM understand and use the command.

## Overview

The `tt cmd2skill` command parses a CLI command's `--help` output and generates a structured skill file. The generated skill captures:
- Command description and usage
- Available subcommands
- Flags with descriptions
- Usage examples

This enables LLMs to effectively use CLI tools without manual documentation lookup.

## Usage

```bash
tt cmd2skill [command] [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--target-dir` | string | `~/.agents/skills` | Directory to write skill files |
| `--dry-run` | bool | `false` | Print skill content to stdout instead of writing files |
| `--examples` | bool | `false` | Fetch examples for subcommands |

### Examples

Generate a skill for kubectl:
```bash
tt cmd2skill kubectl
```

Generate a skill for git with examples:
```bash
tt cmd2skill git --examples
```

Preview skill output without writing:
```bash
tt cmd2skill docker --dry-run
```

Write to custom directory:
```bash
tt cmd2skill gh --target-dir ./.forge/skills
```

## How It Works

1. **Help Parsing**: Runs `command --help` and parses the output
2. **Fallback**: If `--help` fails, attempts `man command`
3. **Extraction**: Extracts description, usage, subcommands, flags, examples
4. **Generation**: Creates a skill directory with SKILL.md in the target directory

## Generated Skill Structure

```
.trae/skills/
└── kubectl/
    └── SKILL.md
```

## Generated Skill Format

```markdown
---
name: "kubectl"
description: "kubectl controls the Kubernetes cluster manager"
---

# kubectl

**Description**: kubectl controls the Kubernetes cluster manager

**Usage**: `kubectl [OPTIONS] COMMAND [SUBCOMMAND]`

## Subcommands

- `apply`: Apply a configuration to a resource
- `get`: Display one or more resources
- `describe`: Show details of a resource
- ...

## Flags

| Flag | Description |
|------|-------------|
| --namespace | If present, the namespace scope for this request |
| --kubeconfig | Path to kubeconfig file |
| ...

## Examples

```
kubectl get pods
```
```

## Use Cases

1. **Document unfamiliar commands** - Generate a skill for any CLI tool you need to use
2. **Create command references** - Build a library of skills for common tools
3. **Enable LLM tool use** - Provide structured command documentation for LLM agents
4. **Quick command lookup** - Generate and review skill files for complex commands

## Supported Commands

Any command that provides:
- `--help` flag with parseable output
- `man` page entry

Common examples:
- Git: `git`, `gh`
- Containers: `docker`, `podman`, `kubectl`
- Cloud CLIs: `aws`, `gcloud`, `az`
- Package managers: `npm`, `pip`, `cargo`
- Any CLI tool with standard `--help` output

## Limitations

- Help output must be in English or Chinese (系统语言)
- Complex help formats may not parse completely
- Interactive commands (with prompts) are not fully supported
- Colorized output may affect parsing accuracy