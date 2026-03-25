# Wombat User Guide

Wombat helps you manage your Claude Code setup. If you use Claude Code in different places — at work, on personal projects, across different repos — Wombat keeps everything organized so you don't have to manually copy skills, agents, and permissions around.

## What does Wombat actually do?

Without Wombat, you'd need to:
- Manually install skills and agents into each Claude Code directory
- Keep track of which permissions you've allowed where
- Update everything by hand when a skill repo gets new features

Wombat does all of this for you. You tell it what you want and where, and it handles the rest.

## Getting started

### Step 1: Install

Build from source:

```
go build -o wombat ./cmd/wombat
```

Move the `wombat` binary somewhere on your PATH.

### Step 2: Set up your config

Run:

```
wombat init
```

Wombat will walk you through a few questions:

1. **Scope name** — Give a name to each place you use Claude Code. For example, "work" for your work projects directory, or "personal" for your personal stuff. Just press Enter when you're done adding scopes.

2. **Path** — Where that scope lives on your computer. For example, `~/work` for your work directory. Wombat automatically looks for the `.claude` folder inside it.

Wombat always adds a "global" scope pointing to `~/.claude` — this is your system-wide Claude Code configuration.

After answering, Wombat scans your existing Claude Code setup and imports what it finds: any skills or agents you already have installed (via symlinks), your permissions, and your plugins.

Your config is saved to `~/wombat/config.yaml`.

### Step 3: Apply

```
wombat apply
```

This is the command that makes things happen. It:
- Downloads any skill/agent repositories you've configured (if not already downloaded)
- Creates the right links so Claude Code can find everything — both in your scope directories and in every git project underneath them
- Updates your Claude Code settings files with the correct permissions and plugins

You'll see a summary of what changed:
```
  + ~/.claude/skills/my-skill        (created)
  - ~/work/.claude/skills/old-skill  (removed)
  ~ ~/.claude/settings.json          (updated)
Done: 1 created, 1 removed, 1 updated, 0 errors
```

If nothing changed, it says "Everything up to date."

## The interactive dashboard

Just run `wombat` with no arguments:

```
wombat
```

This opens a full-screen dashboard with five tabs:

| Tab | What it shows |
|-----|---------------|
| Plugins | Claude Code plugins you can turn on/off per scope |
| Skills | All available skills, grouped by where they came from |
| Agents | All available agents, grouped by where they came from |
| Permissions | Your allow and deny rules |
| Defaults | Which scopes each source auto-enables items in |

### Moving around

- **Switch tabs**: Press `Tab` / `Shift+Tab`, or press `1` through `5`
- **Move up/down**: Arrow keys, or `j`/`k`
- **Move between scopes**: Left/right arrows, or `h`/`l`
- **Toggle on/off**: Press `Space` on any item to enable or disable it for the currently selected scope
- **Search**: Press `/` to filter items by name, `Esc` to clear
- **Jump to top/bottom**: `g` / `G` (or Home/End)
- **Page up/down**: `Ctrl+U` / `Ctrl+D`

### Saving your changes

- Press `a` to save and apply your changes
- Press `p` to save, pull updates from source repos, and apply
- Press `q` or `Ctrl+C` to quit without saving

A `*` appears in the title bar when you have unsaved changes.

### Working with permissions

On the Permissions tab:
- Press `n` to add a new allow rule
- Press `N` (capital) to add a new deny rule
- Press `d` to delete the selected rule
- Use `Space` to toggle which scopes a rule applies to

### A note about "global"

When you enable something in the "global" scope, it automatically gets disabled in all other scopes (and vice versa). This is intentional — global means it applies everywhere, so having it also listed under specific scopes would be redundant.

### Resetting an override

If you've manually changed which scopes a skill or agent is enabled in, you can press `r` on the Skills or Agents tab to reset it back to the default (inherited from the source's default_scope setting).

## Adding skills and agents

### From the command line

To add a single skill from a GitHub repository:

```
wombat skill add mattpocock/skills/simplify --scope work
```

The format is `owner/repo/skill-name`. Wombat automatically downloads the repository if it hasn't already.

Same for agents:

```
wombat agent add someuser/agents/my-agent --scope global
```

If the repository isn't on GitHub, use the `--git` flag:

```
wombat skill add myorg/repo/skillname --scope work --git https://gitlab.com/myorg/repo
```

### Adding a whole source repository

If you want to add an entire repository of skills and agents:

```
wombat source add https://github.com/mattpocock/skills --default-scope work,global
```

The `--default-scope` flag means all skills and agents discovered in that repo will automatically be enabled in those scopes. You can override individual items later using the dashboard or the command line.

### Listing what you have

```
wombat skill list              # Show enabled skills
wombat skill list --all        # Show all discovered skills, including disabled ones
wombat skill list --scope work # Show only skills enabled in the "work" scope
wombat agent list              # Same for agents
wombat source list             # List configured source repositories
wombat source list -c          # Also check if updates are available
wombat scope list              # List your scopes
```

### Removing things

```
wombat skill remove my-skill
wombat agent remove my-agent
wombat source remove owner-repo
wombat scope remove work
```

A few things to know:
- You can't remove a source if skills or agents still reference it. Remove the items first.
- You can't remove a scope if anything references it (sources, skills, agents, plugins, or permissions). Clean up the references first.
- Removing a source from the config does **not** delete the downloaded files. They stay in `~/wombat/sources/` in case you need them. You can delete them manually if you want.

## Keeping things up to date

### Pull updates

```
wombat pull
```

This checks each source repository for updates, downloads any new changes, and then runs `apply` to sync everything. This is how you get the latest skills and agents from repositories you've added.

You can also press `p` in the dashboard to do the same thing.

### Health check

```
wombat doctor
```

This checks for common problems:
- Are all your source repositories properly downloaded?
- Do the configured paths inside those repos actually exist?
- Are all your symlinks pointing to the right places?
- Are there any stale or broken symlinks?
- Have your settings files drifted from what wombat expects?
- Are there updates available for your sources?

If something is wrong, you'll see messages like:

```
✗ source "mattpocock-skills": directory missing (run wombat apply)
⚠ source "other-repo": updates available (run wombat pull)
⚠ scope "work": settings drift (run wombat apply)
```

The fix is usually just running `wombat apply` or `wombat pull`.

Use `--verbose` (or `-v`) to also see passing checks. Use `--offline` to skip the remote update check (useful on slow or no network).

### Tidying up permissions

```
wombat tidy
```

Over time, you might end up with the same permission rules repeated across many project-level settings files. Tidy scans for these and suggests consolidating them into your wombat-managed scopes instead.

For example, if every project under your "work" scope has an `allow: Bash` rule, tidy will recommend moving that rule to the "work" scope so it's managed in one place.

It shows you the recommendations first and asks for confirmation before making changes. Use `--yes` (or `-y`) to skip the confirmation.

## Understanding your config file

Your config lives at `~/wombat/config.yaml` (or wherever `WOMBAT_HOME` points to). See `config.example.yaml` in the repo for a starting point. Here's what each section means:

### Scopes

```yaml
scopes:
  work:
    path: ~/work/.claude
    settings_file: settings.local.json
  global:
    path: ~/.claude
    settings_file: settings.json
```

Each scope is a Claude Code settings directory. The `settings_file` is which JSON file wombat reads and writes in that directory. `settings.local.json` is used for project-scoped settings; `settings.json` is for broader settings.

When you run `wombat apply`, each non-global scope automatically propagates skills, agents, and settings to every git project found under the scope's parent directory. For example, if your "work" scope is at `~/work/.claude`, wombat will find every git repo under `~/work/` and set up symlinks and settings in each project's `.claude/` directory. This is necessary because Claude Code only reads skills from `~/.claude/` (global) and `<project>/.claude/` (project root) — it doesn't check parent directories in between.

### Sources

```yaml
sources:
  mattpocock-skills:
    git: https://github.com/mattpocock/skills
    default_scope: [work, global]
    skill_paths: [skills]
```

A source is a git repository that contains skills and/or agents. The `default_scope` setting means "automatically enable everything from this repo in these scopes." The `skill_paths` and `agent_path` settings tell wombat where to look inside the repo — most of the time wombat detects this automatically.

### Skills and agents

```yaml
skills:
  simplify:
    source: mattpocock-skills
    enabled: [work]
```

This overrides the default scope for a specific item. If you leave out `enabled`, the item inherits from its source's `default_scope`.

### Plugins

```yaml
plugins:
  superpowers@anthropics-skills:
    enabled: [global]
```

Claude Code plugins, with which scopes they're enabled in.

### Permissions

```yaml
permissions:
  allow:
    - rule: "Read"
      scopes: [global]
    - rule: "Bash"
      scopes: [work]
  deny:
    - rule: "Bash(git push --force:*)"
      scopes: [global]
```

Permission rules that wombat writes into your Claude Code settings files. Scopes control where each rule applies. Wombat only manages rules listed here — any rules you've added directly to your settings files by other means are left alone.

### Overrides

```yaml
overrides:
  some-skill:
    enabled: [work]
```

Overrides let you change the scope for an auto-discovered item without adding it as an explicit skill/agent entry. This takes the highest priority when wombat decides where an item should be enabled.

## Troubleshooting

### "config not found — run wombat init"

You haven't set up wombat yet. Run `wombat init` to create your config.

### "config already exists"

You've already run `wombat init`. Your config is at `~/wombat/config.yaml`. Edit it directly or use the dashboard/CLI commands.

### "scope X is referenced by: ..."

You tried to remove a scope that's still in use. The error message tells you exactly what's referencing it. Remove those references first.

### "source X is referenced by: ..."

Same idea — remove the skills/agents that use this source before removing the source.

### "skill/agent X not found in source"

The item name you specified doesn't exist in the source repository. Check the spelling and make sure the repo is up to date (`wombat pull`).

### Symlinks are missing or broken

Run `wombat apply`. If that doesn't fix it, run `wombat doctor --verbose` to see what's going on.

### Settings seem out of sync

Run `wombat apply`. Wombat re-merges your settings files every time you apply. If you've made manual edits to settings files, wombat preserves anything it doesn't manage — but its own rules will be updated to match the config.

### Things look weird after updating a source

Run `wombat apply` after `wombat pull` (or just use `wombat pull`, which does both). New skills or agents from updated sources won't appear until you apply.

## Tips

- You can edit `~/wombat/config.yaml` directly with a text editor. Just run `wombat apply` afterward.
- Use `wombat doctor` periodically (or after something feels off) to catch issues early.
- The dashboard is the fastest way to toggle items across scopes. The CLI is better for scripting or quick one-off changes.
- After cloning a new repo into a scope directory, run `wombat apply` to set up skills and settings in it.
- Wombat-created symlinks will show up in `git status`. Add `.claude/skills/` and `.claude/agents/` to your `.gitignore` if you prefer.
- Set the `WOMBAT_HOME` environment variable if you want your config somewhere other than `~/wombat`.
- Set `NO_COLOR` (to any value) if you want plain text output from `wombat doctor` instead of symbols.
