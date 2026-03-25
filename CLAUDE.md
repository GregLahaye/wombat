# Wombat — AI Agent Guide

Wombat manages Claude Code skills, agents, plugins, and permissions across multiple scoped directories from a single YAML config.

## Architecture

```
config.yaml ─→ Config (parsed, validated)
                  │
                  ├─→ source.Clone/Update (shallow git clones to ~/wombat/sources/)
                  │
                  ├─→ source.DiscoverSkills/DiscoverAgents (scan cloned repos)
                  │       │
                  │       ▼
                  ├─→ resolve.Items (compute effective scopes per item)
                  │       │
                  │       ▼
                  ├─→ apply.syncSymlinks (create/remove symlinks in scope dirs)
                  │
                  └─→ apply.syncSettings (merge plugins + permissions into JSON settings files)
```

### Package dependency graph

```
cmd/wombat/main.go → cli, config, tui
cli/*              → apply, config, resolve, source, tidy
tui/*              → apply, config, resolve
apply              → config, resolve, source
resolve            → config, source (pure logic, no I/O)
tidy               → config
source             → (no internal deps, shells out to git)
config             → (no internal deps)
```

## Key concepts

### Scopes

A scope is a named Claude Code settings directory. Each scope has a `path` (e.g., `~/.claude`) and a `settings_file` (e.g., `settings.json` or `settings.local.json`). The "global" scope is special: items enabled in global are not symlinked to other scopes (optimization to avoid duplication).

### Sources

Git repositories containing skills and/or agents. Cloned shallow (`--depth 1`) to `~/wombat/sources/<name>`. Source names are derived from URLs as `owner-repo`.

### Items (skills and agents)

Skills: directories containing a `SKILL.md` file.
Agents: `.md` files with YAML frontmatter containing both `name:` and `description:` fields. Known doc files (README, LICENSE, CONTRIBUTING, CHANGELOG) are skipped.

### Scope resolution chain (priority order)

1. `overrides[name].Enabled` — explicit override, highest priority
2. `skills[name].Enabled` or `agents[name].Enabled` — `nil` means fall through, `[]` means explicitly disabled
3. `sources[sourceName].DefaultScope` — inherited; `IsInherited=true` flag set

The distinction between `nil` and `[]` on `Enabled` is load-bearing. `nil` = "not configured, use default_scope". `[]` = "explicitly disabled, zero scopes". YAML omits the field for nil; an empty `enabled: []` produces the empty slice.

### Partial ownership model

Wombat only manages keys it owns in settings JSON files: `enabledPlugins` and `permissions.allow`/`permissions.deny`. User-added entries outside wombat's config are preserved. Delta detection uses `prevCfg` to know which rules were previously owned — only those get removed on change.

## File layout

```
~/wombat/                      # WOMBAT_HOME (overridable via env var)
├── config.yaml                # Single source of truth
└── sources/
    └── <owner-repo>/          # Shallow git clone
        ├── .git/
        ├── skills/            # Configured via source.skill_paths
        │   └── <skill-name>/
        │       └── SKILL.md
        └── agents/            # Configured via source.agent_path
            └── <agent-name>.md
```

Scope directories (e.g., `~/.claude/`, `~/work/.claude/`) contain:
- `skills/<name>` — symlink into `~/wombat/sources/<source>/<path>`
- `agents/<name>` — symlink into `~/wombat/sources/<source>/<path>`
- `settings.json` or `settings.local.json` — JSON with merged plugins/permissions

## Config schema (config.yaml)

```yaml
scopes:
  <name>:
    path: <string>              # Required. ~ expanded. Must be unique across scopes.
    settings_file: <string>     # Required. "settings.json" or "settings.local.json".

sources:
  <name>:
    git: <string>               # Required. Git clone URL.
    default_scope: [<scope>...] # Optional. Auto-enable all discovered items in these scopes.
    skill_paths: [<string>...]  # Optional. Subdirs to scan for skills. Default: repo root.
    agent_path: <string>        # Optional. Subdir to scan for agents. Default: repo root.

plugins:
  <name>:
    enabled: [<scope>...]       # Which scopes this plugin is active in.

skills:
  <name>:
    source: <source-name>       # Must reference a key in sources.
    enabled: [<scope>...]       # null = inherit default_scope; [] = disabled.

agents:
  <name>:
    source: <source-name>
    enabled: [<scope>...]

overrides:
  <name>:
    enabled: [<scope>...]       # Highest-priority scope override for any item.

permissions:
  allow:
    - rule: <string>            # e.g., "Read", "Bash(git push:*)"
      scopes: [<scope>...]
  deny:
    - rule: <string>
      scopes: [<scope>...]      # Duplicate rules within allow or deny are rejected.
```

## Validation rules (config.Validate)

- Every scope must have non-empty `path` and `settings_file`.
- Scope paths must be unique (after filepath.Clean).
- Every source must have non-empty `git`.
- All scope references (in sources, plugins, skills, agents, overrides, permissions) must name an existing scope.
- Skills and agents must reference an existing source.
- No duplicate rules within `permissions.allow` or `permissions.deny`.

## CLI commands

| Command | Group | Description |
|---------|-------|-------------|
| `wombat` (no args) | — | Launch interactive TUI |
| `wombat init` | core | Interactive setup from existing Claude Code installation |
| `wombat apply` | core | Clone missing sources, sync symlinks, merge settings |
| `wombat pull` | core | Fetch source updates, then apply |
| `wombat scope add <name> <path>` | manage | `--settings-file` flag, auto-appends `.claude` if missing |
| `wombat scope list` | manage | |
| `wombat scope remove <name>` | manage | Fails if scope has references |
| `wombat source add <git-url>` | manage | `--name`, `--default-scope` flags; clones and auto-detects layout |
| `wombat source list` | manage | `--check-updates` / `-c` flag for remote check |
| `wombat source remove <name>` | manage | Fails if skills/agents reference it; does NOT delete cloned dir |
| `wombat skill add <owner/repo/name>` | manage | `--scope` (required), `--git` flags |
| `wombat skill list` | manage | `--scope`, `--source`, `--all` flags |
| `wombat skill remove <name>` | manage | Removes explicit entry or disables auto-discovered item |
| `wombat agent add/list/remove` | manage | Same interface as skill |
| `wombat doctor` | maintain | `--verbose` / `-v`, `--offline` flags |
| `wombat tidy` | maintain | `--yes` / `-y` to skip confirmation prompt |

## TUI

5 tabs: Plugins (0), Skills (1), Agents (2), Permissions (3), Defaults (4).

Key bindings: `q`/`Ctrl+C` quit, `a` apply+exit, `p` pull+apply+exit, `Tab`/`Shift+Tab` cycle tabs, `1-5` jump to tab, `j/k`/arrows navigate, `h/l` change scope column, `Space` toggle scope, `r` reset override (skills/agents only), `/` filter, `n`/`N` add allow/deny rule (permissions tab), `d` delete rule (permissions tab), `Esc` clear filter.

Global exclusivity: enabling "global" auto-disables all other scopes for that item, and vice versa.

TUI state is written to config on apply (`a`/`p`). The `original` snapshot detects dirty state for the `*` indicator in the title bar.

## Gotchas and constraints

1. **nil vs empty slice on Enabled**: `nil` falls through to `default_scope`. `[]` is "explicitly no scopes". Mixing these up silently changes behavior.
2. **Global scope optimization**: If an item has "global" in its scopes, symlinks are ONLY created in the global scope dir. Other scopes are skipped even if listed.
3. **Item name collisions across sources**: First source (alphabetical) wins. Later sources with the same item name are silently skipped. `wombat doctor` reports these.
4. **Source removal doesn't delete files**: `wombat source remove` only removes from config. The cloned directory at `~/wombat/sources/<name>` persists.
5. **Symlink cleanup is scoped**: Only symlinks pointing into `~/wombat/sources/` are managed. User-created symlinks pointing elsewhere are never touched.
6. **Settings merge is additive for unknown keys**: Wombat reads, patches its owned keys, and writes back. Any JSON keys it doesn't own survive untouched.
7. **Atomic writes**: Config and settings files use temp+rename. Safe against crashes but requires write permission to the parent directory.
8. **Path expansion**: `~` is expanded on load, contracted on save. Config files are portable across machines with different home dirs.
9. **Shallow clones only**: Sources use `--depth 1`. No history is available. `source.Update` does `fetch --depth 1` + `reset --hard FETCH_HEAD`.
10. **Agent discovery scans frontmatter**: Only the first 50 lines are checked for YAML frontmatter. Files without both `name:` and `description:` in frontmatter are skipped.
11. **Scope removal is blocked by references**: You must remove all skills, agents, plugins, permissions, sources, and overrides referencing a scope before removing it.
12. **Project dir propagation**: Non-global scopes create symlinks and settings in every git repo under the scope's parent directory. Always uses `settings.local.json` for project dirs to avoid git pollution.

## Project directory propagation

Non-global scopes propagate skills, agents, and settings to git project directories found under the scope's parent. Claude Code only reads skills/agents from `~/.claude/` (user) and `<project>/.claude/` (project root) — intermediate `.claude/` directories are invisible. Wombat bridges this gap.

### How it works

For each non-global scope (e.g., `tim` with path `~/tim/.claude`):
1. `wombat apply` walks `~/tim/` recursively to find directories containing `.git`
2. For each project found, creates symlinks in `<project>/.claude/skills/` and `<project>/.claude/agents/`
3. Merges settings into `<project>/.claude/settings.local.json` (always `settings.local.json`, regardless of scope's `settings_file`)

The walk skips hidden directories, `node_modules`, `vendor`, and stops at `.git` boundaries (doesn't descend into repos).

### Global scope is exempt

Items with "global" in their scopes only get symlinks at `~/.claude/`. Claude Code already reads this from everywhere — no propagation needed.

### Limitations

- No per-project exclusion. A scope applies to ALL projects under it.
- Symlinks appear in `git status`. Add `.claude/skills/` and `.claude/agents/` to `.gitignore` if desired.
- New projects require `wombat apply` to get symlinks.

## Build and test

```bash
go build -o wombat ./cmd/wombat
go test ./...
```

The `version` variable in `cmd/wombat/main.go` defaults to `"dev"` and can be set at build time via `-ldflags`.

## Environment variables

- `WOMBAT_HOME`: Override the wombat directory (default: `~/wombat`).
- `NO_COLOR`: When set to any non-empty value, `wombat doctor` outputs plain text instead of Unicode symbols.
