# Codeany

[![Release](https://img.shields.io/github/v/release/codeany-ai/codeany)](https://github.com/codeany-ai/codeany/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/codeany-ai/codeany)](https://go.dev/)
[![License](https://img.shields.io/github/license/codeany-ai/codeany)](LICENSE)

An open-source AI-powered terminal agent for software engineering. Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI and the [Open Agent SDK](https://github.com/codeany-ai/open-agent-sdk-go).

**41 slash commands** | **Skills & Plugins** | **MCP support** | **Chinese/IME input** | **Self-update**

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/codeany-ai/codeany/main/install.sh | sh
```

Or install from source:

```bash
go install github.com/codeany-ai/codeany/cmd/codeany@latest
```

## Setup

Set your API key:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
# Or for OpenRouter / custom providers:
export CODEANY_API_KEY="sk-or-..."
export CODEANY_BASE_URL="https://openrouter.ai/api"
export CODEANY_MODEL="anthropic/claude-sonnet-4-5"
```

## Usage

```bash
# Interactive mode
codeany

# With initial prompt
codeany "explain this codebase"

# Pipe mode
echo "what is 2+2" | codeany -p

# Print mode (non-interactive)
codeany --print -y "list files in src/"

# JSON output
echo "hello" | codeany -p -y --output-format json

# Skip permission prompts
codeany -y

# Use specific model
codeany -m opus-4-6
```

## Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/model [name]` | Switch model |
| `/fast` | Toggle faster model |
| `/cost` | Show token usage and cost |
| `/clear` | Clear conversation |
| `/compact [hint]` | Compact conversation |
| `/plan [task]` | Plan mode / plan a task |
| `/commit [msg]` | Git commit helper |
| `/review [target]` | Code review |
| `/diff` | Show git diff summary |
| `/bug <desc>` | Investigate a bug |
| `/test [target]` | Run tests |
| `/init` | Initialize project (create CODEANY.md) |
| `/doctor` | Environment diagnostics |
| `/mcp` | Manage MCP servers |
| `/skills` | List available skills |
| `/plugin` | List installed plugins |
| `/hooks` | Show configured hooks |
| `/context` | Show all context sources |
| `/session` | Session details |
| `/files` | Files accessed this session |
| `/resume` | List recent sessions |
| `/export` | Export conversation |
| `/config` | Show configuration |
| `/permissions` | Permission mode |
| `/status` | Session status |
| `/quit` | Exit |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Shift+Enter` | New line |
| `Ctrl+C` | Cancel / Exit |
| `Ctrl+D` | Exit (empty input) |
| `Ctrl+L` | Clear conversation |
| `Ctrl+O` | Toggle expand tool output |
| `Up/Down` | Input history |
| `PgUp/PgDown` | Scroll messages |
| `Tab` | Complete slash command |
| `Esc` | Clear input / close menu |
| `! cmd` | Run shell command |

## Configuration

Config directory: `~/.codeany/`

```
~/.codeany/
├── settings.json      # Main config (model, permissions, MCP, hooks)
├── config.yaml        # YAML config (alternative)
├── permissions.json   # Persisted permission rules
├── memory/            # Memory files
├── sessions/          # Session history
├── skills/            # User skills
│   └── my-skill/
│       └── SKILL.md
└── plugins/           # Plugins
    └── my-plugin/
        ├── plugin.json
        └── skills/
```

### settings.json

```json
{
  "model": "sonnet-4-6",
  "permissionMode": "default",
  "maxTurns": 100,
  "mcpServers": {
    "filesystem": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    }
  },
  "hooks": {
    "preToolUse": [],
    "postToolUse": []
  }
}
```

## Project Configuration

Create `CODEANY.md` (or `CLAUDE.md`) in your project root:

```markdown
# Project Instructions

## Commands
- `npm test` to run tests
- `npm run build` to build

## Code Style
- Use TypeScript strict mode
- Prefer functional components
```

Also supports:
- `CODEANY.local.md` / `CLAUDE.local.md` — personal, gitignored
- `.codeany/rules/*.md` / `.claude/rules/*.md` — modular rules

## Skills

Create custom skills in `.codeany/skills/<name>/SKILL.md`:

```markdown
---
name: deploy
description: Deploy to production
argumentHint: <environment>
---

Deploy the application to $ARGUMENTS environment.
Run the deployment script and verify health checks.
```

Invoke with: `/deploy staging`

## MCP Servers

Configure MCP servers in `settings.json` or manage with `/mcp`:

```bash
/mcp              # List servers
/mcp tools        # List available tools
/mcp reconnect X  # Reconnect server
```

## Update

```bash
codeany update
```

## License

MIT
