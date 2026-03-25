# Scope Propagation to Project Directories

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Non-global scopes propagate skills, agents, and settings into each git project's `.claude/` directory under the scope's parent, so Claude Code actually sees them.

**Architecture:** Add `DiscoverProjectDirs()` to walk scope parent directories for `.git` repos. Pass discovered dirs into `syncSymlinks` and `syncSettings` so they create symlinks and merge settings in project dirs alongside scope dirs. Extend doctor and tidy accordingly.

**Tech Stack:** Go stdlib (`filepath.WalkDir`, `os`, `io/fs`)

---

### Task 1: Add `DiscoverProjectDirs` function

**Files:**
- Create: `internal/apply/projects.go`
- Create: `internal/apply/projects_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/apply/projects_test.go
package apply

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
)

func TestDiscoverProjectDirs_FindsGitRepos(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")

	// Create two git repos under root.
	for _, name := range []string{"project-a", "project-b"} {
		os.MkdirAll(filepath.Join(root, name, ".git"), 0o755)
	}
	// Create a non-git directory (should be skipped).
	os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755)

	dirs := DiscoverProjectDirs(scopePath)
	slices.Sort(dirs)

	if len(dirs) != 2 {
		t.Fatalf("expected 2 project dirs, got %d: %v", len(dirs), dirs)
	}
	wantA := filepath.Join(root, "project-a", ".claude")
	wantB := filepath.Join(root, "project-b", ".claude")
	if dirs[0] != wantA || dirs[1] != wantB {
		t.Errorf("expected [%s, %s], got %v", wantA, wantB, dirs)
	}
}

func TestDiscoverProjectDirs_SkipsHiddenAndVendor(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")

	os.MkdirAll(filepath.Join(root, ".hidden", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "node_modules", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "vendor", ".git"), 0o755)

	dirs := DiscoverProjectDirs(scopePath)
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs (all skipped), got %v", dirs)
	}
}

func TestDiscoverProjectDirs_ExcludesScopePath(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")

	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "child", ".git"), 0o755)

	dirs := DiscoverProjectDirs(scopePath)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d: %v", len(dirs), dirs)
	}
	if dirs[0] != filepath.Join(root, "child", ".claude") {
		t.Errorf("expected child, got %s", dirs[0])
	}
}

func TestDiscoverProjectDirs_NestedRepos(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")

	os.MkdirAll(filepath.Join(root, "clients", "project-x", ".git"), 0o755)

	dirs := DiscoverProjectDirs(scopePath)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d: %v", len(dirs), dirs)
	}
	want := filepath.Join(root, "clients", "project-x", ".claude")
	if dirs[0] != want {
		t.Errorf("expected %s, got %s", want, dirs[0])
	}
}

func TestDiscoverProjectDirs_StopsAtGitBoundary(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")

	os.MkdirAll(filepath.Join(root, "outer", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "outer", "inner", ".git"), 0o755)

	dirs := DiscoverProjectDirs(scopePath)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir (outer only), got %d: %v", len(dirs), dirs)
	}
}

func TestDiscoverAllProjectDirs_SkipsGlobal(t *testing.T) {
	root := t.TempDir()
	workParent := filepath.Join(root, "work")
	os.MkdirAll(filepath.Join(workParent, "proj", ".git"), 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: filepath.Join(workParent, ".claude"), SettingsFile: "settings.local.json"},
			"global": {Path: filepath.Join(root, "home", ".claude"), SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	result := DiscoverAllProjectDirs(cfg)
	if _, ok := result["global"]; ok {
		t.Error("global scope should be skipped")
	}
	if len(result["work"]) != 1 {
		t.Errorf("expected 1 project dir under work, got %v", result["work"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/apply/ -run TestDiscoverProjectDirs -v`
Expected: FAIL — `DiscoverProjectDirs` undefined

- [ ] **Step 3: Write the implementation**

```go
// internal/apply/projects.go
package apply

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
)

// DiscoverProjectDirs finds git repositories under the parent of scopePath.
// Returns .claude directory paths for each project found (e.g., ~/tim/project/.claude).
// The scopePath itself is excluded. Skips hidden dirs, node_modules, vendor,
// and does not descend into git repos.
func DiscoverProjectDirs(scopePath string) []string {
	parent := filepath.Dir(scopePath)
	var dirs []string

	filepath.WalkDir(parent, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		if path == parent {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			claudeDir := filepath.Join(path, ".claude")
			if filepath.Clean(claudeDir) != filepath.Clean(scopePath) {
				dirs = append(dirs, claudeDir)
			}
			return filepath.SkipDir
		}
		return nil
	})

	return dirs
}

// DiscoverAllProjectDirs returns project dirs for each non-global scope.
// Key is scope name, value is list of .claude directory paths.
func DiscoverAllProjectDirs(cfg *config.Config) map[string][]string {
	result := make(map[string][]string)
	for name, scope := range cfg.Scopes {
		if name == "global" {
			continue
		}
		if dirs := DiscoverProjectDirs(scope.Path); len(dirs) > 0 {
			result[name] = dirs
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/apply/ -run "TestDiscoverProjectDirs|TestDiscoverAllProjectDirs" -v`
Expected: all 6 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/apply/projects.go internal/apply/projects_test.go
git commit -m "feat: add DiscoverProjectDirs for scope propagation"
```

---

### Task 2: Modify `syncSymlinks`, `syncSettings`, `CheckSettings`, and `Apply` for project dir propagation

This task changes all four function signatures together so the code compiles at every step. Existing tests are updated inline.

**Files:**
- Modify: `internal/apply/symlinks.go`
- Modify: `internal/apply/settings.go`
- Modify: `internal/apply/apply.go`
- Modify: `internal/apply/symlinks_test.go` (update existing calls)
- Modify: `internal/apply/edge_test.go` (update existing calls)
- Create: `internal/apply/propagation_test.go`

- [ ] **Step 1: Write the new tests**

```go
// internal/apply/propagation_test.go
package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
)

func TestSyncSymlinks_PropagatesToProjectDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	srcDir := filepath.Join(dir, "sources", "my-source", "skills", "my-skill")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# Skill"), 0o644)

	scopePath := filepath.Join(dir, "work", ".claude")
	projDir := filepath.Join(dir, "work", "project-a")
	os.MkdirAll(scopePath, 0o755)
	os.MkdirAll(filepath.Join(projDir, ".git"), 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.local.json"},
		},
	}
	cfg.EnsureMaps()

	items := []resolve.ResolvedItem{{
		Name: "my-skill", SourceName: "my-source",
		SourcePath: "skills/my-skill", Scopes: []string{"work"}, Kind: "skill",
	}}

	projDirs := map[string][]string{
		"work": {filepath.Join(projDir, ".claude")},
	}

	r := &Result{}
	syncSymlinks(cfg, items, "skills", r, projDirs)

	scopeLink := filepath.Join(scopePath, "skills", "my-skill")
	if _, err := os.Lstat(scopeLink); err != nil {
		t.Errorf("missing scope symlink: %s", scopeLink)
	}
	projLink := filepath.Join(projDir, ".claude", "skills", "my-skill")
	if _, err := os.Lstat(projLink); err != nil {
		t.Errorf("missing project symlink: %s", projLink)
	}
	if len(r.Errors) > 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}
}

func TestSyncSymlinks_CleansStaleFromProjectDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	sourcesDir := filepath.Join(dir, "sources")
	os.MkdirAll(filepath.Join(sourcesDir, "old-source", "skills", "old-skill"), 0o755)

	scopePath := filepath.Join(dir, "work", ".claude")
	projDir := filepath.Join(dir, "work", "project-a")
	os.MkdirAll(filepath.Join(projDir, ".git"), 0o755)

	staleDir := filepath.Join(projDir, ".claude", "skills")
	os.MkdirAll(staleDir, 0o755)
	staleLink := filepath.Join(staleDir, "old-skill")
	os.Symlink(filepath.Join(sourcesDir, "old-source", "skills", "old-skill"), staleLink)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.local.json"},
		},
	}
	cfg.EnsureMaps()

	projDirs := map[string][]string{
		"work": {filepath.Join(projDir, ".claude")},
	}

	r := &Result{}
	syncSymlinks(cfg, nil, "skills", r, projDirs)

	if _, err := os.Lstat(staleLink); err == nil {
		t.Error("stale symlink in project dir should have been removed")
	}
	if len(r.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(r.Removed))
	}
}

func TestSyncSettings_PropagatesToProjectDirs(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	projDir := filepath.Join(dir, "work", "project-a")
	os.MkdirAll(scopePath, 0o755)
	os.MkdirAll(filepath.Join(projDir, ".git"), 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	projDirs := map[string][]string{
		"work": {filepath.Join(projDir, ".claude")},
	}

	r := &Result{}
	err := syncSettings(cfg, nil, r, projDirs)
	if err != nil {
		t.Fatalf("syncSettings: %v", err)
	}

	// Scope settings uses scope's settings_file.
	if _, err := os.Stat(filepath.Join(scopePath, "settings.json")); err != nil {
		t.Errorf("missing scope settings")
	}
	// Project settings always uses settings.local.json.
	projSettings := filepath.Join(projDir, ".claude", "settings.local.json")
	if _, err := os.Stat(projSettings); err != nil {
		t.Errorf("missing project settings: %s", projSettings)
	}
	data, _ := ReadSettings(projSettings)
	perms, _ := data["permissions"].(map[string]any)
	allow := ExtractStringSlice(perms, "allow")
	if len(allow) != 1 || allow[0] != "Read" {
		t.Errorf("expected [Read] in project settings, got %v", allow)
	}
}

func TestSyncSettings_ProjectDirErrorNonFatal(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	os.MkdirAll(scopePath, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	readOnly := filepath.Join(dir, "readonly")
	os.MkdirAll(readOnly, 0o555)
	t.Cleanup(func() { os.Chmod(readOnly, 0o755) })

	projDirs := map[string][]string{
		"work": {filepath.Join(readOnly, ".claude")},
	}

	r := &Result{}
	err := syncSettings(cfg, nil, r, projDirs)
	if err != nil {
		t.Fatalf("project dir error should be non-fatal, got: %v", err)
	}
	if len(r.Errors) == 0 {
		t.Error("expected error in Result.Errors")
	}
}

func TestCheckSettings_IncludesProjectDirs(t *testing.T) {
	dir := t.TempDir()

	scopePath := filepath.Join(dir, "work", ".claude")
	projDir := filepath.Join(dir, "work", "project-a")
	os.MkdirAll(scopePath, 0o755)
	os.MkdirAll(filepath.Join(projDir, ".claude"), 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	projDirs := map[string][]string{
		"work": {filepath.Join(projDir, ".claude")},
	}

	drifted, err := CheckSettings(cfg, projDirs)
	if err != nil {
		t.Fatalf("CheckSettings: %v", err)
	}
	if len(drifted) == 0 {
		t.Error("expected drift for missing project settings")
	}
}

func TestApply_FullPropagation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WOMBAT_HOME", dir)

	srcDir := filepath.Join(dir, "sources", "my-source")
	skillDir := filepath.Join(srcDir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0o644)

	agentDir := filepath.Join(srcDir, "agents")
	os.MkdirAll(agentDir, 0o755)
	os.WriteFile(filepath.Join(agentDir, "my-agent.md"), []byte("---\nname: my-agent\ndescription: test\n---\n"), 0o644)

	scopeParent := filepath.Join(dir, "workspace")
	scopePath := filepath.Join(scopeParent, ".claude")
	os.MkdirAll(scopePath, 0o755)

	for _, name := range []string{"proj-a", "proj-b"} {
		os.MkdirAll(filepath.Join(scopeParent, name, ".git"), 0o755)
	}

	globalPath := filepath.Join(dir, "home", ".claude")
	os.MkdirAll(globalPath, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: scopePath, SettingsFile: "settings.local.json"},
			"global": {Path: globalPath, SettingsFile: "settings.json"},
		},
		Sources: map[string]config.Source{
			"my-source": {
				Git: "https://example.com/repo", DefaultScope: []string{"work"},
				SkillPaths: []string{"skills"}, AgentPath: "agents",
			},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	r, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(r.Errors) > 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}

	// Check both project dirs have symlinks and settings.
	for _, proj := range []string{"proj-a", "proj-b"} {
		projClaude := filepath.Join(scopeParent, proj, ".claude")
		for _, check := range []string{
			filepath.Join(projClaude, "skills", "my-skill"),
			filepath.Join(projClaude, "agents", "my-agent.md"),
		} {
			if _, err := os.Lstat(check); err != nil {
				t.Errorf("missing project symlink in %s: %s", proj, check)
			}
		}
		settings := filepath.Join(projClaude, "settings.local.json")
		data, _ := ReadSettings(settings)
		perms, _ := data["permissions"].(map[string]any)
		allow := ExtractStringSlice(perms, "allow")
		found := false
		for _, r := range allow {
			if r == "Read" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected 'Read' in project %s settings, got %v", proj, allow)
		}
	}

	// Idempotent.
	r2, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply (idempotent): %v", err)
	}
	if len(r2.Created) > 0 || len(r2.Removed) > 0 || len(r2.Updated) > 0 {
		t.Errorf("expected idempotent, got created=%d removed=%d updated=%d",
			len(r2.Created), len(r2.Removed), len(r2.Updated))
	}
}
```

- [ ] **Step 2: Modify `syncSymlinks` signature and implementation**

In `internal/apply/symlinks.go`, replace `syncSymlinks` entirely:

```go
// syncSymlinks creates and removes symlinks for resolved items.
// projDirs maps scope names to project .claude directories for propagation.
func syncSymlinks(cfg *config.Config, items []resolve.ResolvedItem, subdir string, r *Result, projDirs map[string][]string) {
	sourcesDir := config.SourcesDir()

	// Build desired state: .claude path -> link filename -> target path.
	desired := make(map[string]map[string]string)
	addDesired := func(claudePath, linkName, target string) {
		if desired[claudePath] == nil {
			desired[claudePath] = make(map[string]string)
		}
		desired[claudePath][linkName] = target
	}

	for _, item := range items {
		if item.SourcePath == "" {
			continue
		}
		absPath := filepath.Join(sourcesDir, item.SourceName, item.SourcePath)
		scopes := item.Scopes
		if slices.Contains(scopes, "global") {
			scopes = []string{"global"}
		}
		ln := item.LinkName()
		for _, scopeName := range scopes {
			scope, ok := cfg.Scopes[scopeName]
			if !ok {
				continue
			}
			addDesired(scope.Path, ln, absPath)
			for _, projDir := range projDirs[scopeName] {
				addDesired(projDir, ln, absPath)
			}
		}
	}

	// Create/update desired symlinks.
	for claudePath, items := range desired {
		dir := filepath.Join(claudePath, subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("creating %s: %w", dir, err))
			continue
		}
		for name, target := range items {
			link := filepath.Join(dir, name)
			created, err := ensureSymlink(link, target)
			if err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("symlink %s: %w", link, err))
				continue
			}
			if created {
				r.Created = append(r.Created, link)
			}
		}
	}

	// Remove stale symlinks (only those pointing into sources/).
	sourcesPrefix := filepath.Clean(sourcesDir) + string(filepath.Separator)
	cleanStale := func(claudePath string) {
		dir := filepath.Join(claudePath, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			link := filepath.Join(dir, entry.Name())
			target, err := os.Readlink(link)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			target = filepath.Clean(target)
			if !strings.HasPrefix(target, sourcesPrefix) {
				continue
			}
			if d, ok := desired[claudePath]; ok {
				if _, wanted := d[entry.Name()]; wanted {
					continue
				}
			}
			if err := os.Remove(link); err == nil {
				r.Removed = append(r.Removed, link)
			}
		}
	}

	for _, scope := range cfg.Scopes {
		cleanStale(scope.Path)
	}
	for _, dirs := range projDirs {
		for _, d := range dirs {
			cleanStale(d)
		}
	}
}
```

- [ ] **Step 3: Modify `syncSettings` and `CheckSettings`**

In `internal/apply/settings.go`, replace `syncSettings` and `CheckSettings`:

```go
func syncSettings(cfg, prevCfg *config.Config, r *Result, projDirs map[string][]string) error {
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]

		// Merge for scope dir.
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := ReadSettings(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		changed := mergePlugins(data, cfg, prevCfg, scopeName)
		changed = mergePermissions(data, cfg, prevCfg, scopeName) || changed
		if changed {
			if err := WriteSettings(path, data); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			r.Updated = append(r.Updated, path)
		}

		// Propagate to project dirs (always settings.local.json).
		for _, projDir := range projDirs[scopeName] {
			projPath := filepath.Join(projDir, "settings.local.json")
			projData, err := ReadSettings(projPath)
			if err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("reading %s: %w", projPath, err))
				continue
			}
			projChanged := mergePlugins(projData, cfg, prevCfg, scopeName)
			projChanged = mergePermissions(projData, cfg, prevCfg, scopeName) || projChanged
			if projChanged {
				if err := WriteSettings(projPath, projData); err != nil {
					r.Errors = append(r.Errors, fmt.Errorf("writing %s: %w", projPath, err))
					continue
				}
				r.Updated = append(r.Updated, projPath)
			}
		}
	}
	return nil
}

func CheckSettings(cfg *config.Config, projDirs map[string][]string) ([]string, error) {
	var drifted []string
	for _, scopeName := range cfg.ScopeNames() {
		scope := cfg.Scopes[scopeName]

		// Check scope dir.
		path := filepath.Join(scope.Path, scope.SettingsFile)
		data, err := ReadSettings(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		clone := cloneMap(data)
		if clone == nil {
			clone = make(map[string]any)
		}
		if mergePlugins(clone, cfg, nil, scopeName) || mergePermissions(clone, cfg, nil, scopeName) {
			drifted = append(drifted, scopeName)
			continue
		}

		// Check project dirs.
		for _, projDir := range projDirs[scopeName] {
			projPath := filepath.Join(projDir, "settings.local.json")
			projData, err := ReadSettings(projPath)
			if err != nil {
				continue // Non-fatal for project dirs.
			}
			projClone := cloneMap(projData)
			if projClone == nil {
				projClone = make(map[string]any)
			}
			if mergePlugins(projClone, cfg, nil, scopeName) || mergePermissions(projClone, cfg, nil, scopeName) {
				drifted = append(drifted, scopeName)
				break
			}
		}
	}
	return drifted, nil
}
```

- [ ] **Step 4: Update `Apply` in `apply.go`**

```go
func Apply(cfg, prevCfg *config.Config) (*Result, error) {
	r := &Result{}

	discovered := DiscoverAll(cfg)
	skills, agents := resolve.Items(cfg, discovered, false)

	projDirs := DiscoverAllProjectDirs(cfg)

	syncSymlinks(cfg, skills, "skills", r, projDirs)
	syncSymlinks(cfg, agents, "agents", r, projDirs)

	if err := syncSettings(cfg, prevCfg, r, projDirs); err != nil {
		return r, err
	}

	return r, nil
}
```

- [ ] **Step 5: Update all existing test calls to match new signatures**

In `internal/apply/symlinks_test.go`, add `nil` as last arg to every `syncSymlinks` call:
```go
syncSymlinks(cfg, items, "skills", r, nil)
```

In `internal/apply/edge_test.go`, add `nil` as last arg to the `syncSettings` call:
```go
err := syncSettings(cfg, nil, r, nil)
```

In `internal/cli/doctor.go`, update the `CheckSettings` call:
```go
drifted, _ := apply.CheckSettings(cfg, nil)
```
(Doctor's full update comes in Task 3, but this keeps it compiling.)

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/apply/symlinks.go internal/apply/settings.go internal/apply/apply.go internal/apply/propagation_test.go internal/apply/symlinks_test.go internal/apply/edge_test.go internal/cli/doctor.go
git commit -m "feat: propagate symlinks and settings to project dirs under non-global scopes"
```

---

### Task 3: Update doctor for project dir validation

**Files:**
- Modify: `internal/cli/doctor.go`

- [ ] **Step 1: Update doctor to compute projDirs and use them throughout**

The full changes to `doctor.go`:

1. After `discovered := apply.DiscoverAll(cfg)` (line 111), add:
```go
projDirs := apply.DiscoverAllProjectDirs(cfg)

// Report discovered projects in verbose mode.
for _, scopeName := range cfg.ScopeNames() {
	if scopeName == "global" {
		continue
	}
	if dirs := projDirs[scopeName]; len(dirs) > 0 {
		report("info", fmt.Sprintf("scope %q: %d project(s) discovered", scopeName, len(dirs)))
	}
}
```

2. Replace the `addDesired` closure to include project dirs:
```go
addDesired := func(items []resolve.ResolvedItem, subdir string) {
	for _, item := range items {
		if item.SourcePath == "" {
			continue
		}
		scopes := item.Scopes
		if slices.Contains(scopes, "global") {
			scopes = []string{"global"}
		}
		for _, scopeName := range scopes {
			scope := cfg.Scopes[scopeName]
			link := filepath.Join(scope.Path, subdir, item.LinkName())
			desired[link] = true
			for _, projDir := range projDirs[scopeName] {
				link := filepath.Join(projDir, subdir, item.LinkName())
				desired[link] = true
			}
		}
	}
}
```

3. After the existing unmanaged symlinks loop (which scans `cfg.ScopeNames()`), add a second loop for project dirs:
```go
for scopeName := range cfg.Scopes {
	if scopeName == "global" {
		continue
	}
	for _, projDir := range projDirs[scopeName] {
		for _, subdir := range []string{"skills", "agents"} {
			dir := filepath.Join(projDir, subdir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				link := filepath.Join(dir, entry.Name())
				target, err := os.Readlink(link)
				if err != nil {
					continue
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				target = filepath.Clean(target)
				if !strings.HasPrefix(target, sourcesPrefix) {
					continue
				}
				if !desired[link] {
					report("warning", fmt.Sprintf("unmanaged symlink: %s -> %s", link, target))
				}
			}
		}
	}
}
```

4. Update the `CheckSettings` call:
```go
drifted, _ := apply.CheckSettings(cfg, projDirs)
```

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: all PASS

- [ ] **Step 3: Commit**

```bash
git add internal/cli/doctor.go
git commit -m "feat: doctor checks project dirs for symlinks and settings drift"
```

---

### Task 4: Fix tidy to skip wombat-managed rules

**Files:**
- Modify: `internal/tidy/tidy.go`
- Modify: `internal/tidy/tidy_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tidy/tidy_test.go`:

```go
func TestScan_SkipsWombatManagedRules(t *testing.T) {
	root := t.TempDir()
	scopePath := filepath.Join(root, ".claude")
	os.MkdirAll(scopePath, 0o755)

	writeProjectSettings(t, filepath.Join(root, "project-a"), "settings.local.json", map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Read", "CustomRule"},
		},
	})

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: scopePath, SettingsFile: "settings.local.json"},
			"global": {Path: filepath.Join(t.TempDir(), ".claude"), SettingsFile: "settings.json"},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	result, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	for _, rec := range result.Recommendations {
		if rec.Rule == "Read" {
			t.Errorf("should not recommend 'Read' — already managed by wombat")
		}
	}
	if len(result.Recommendations) != 1 {
		t.Errorf("expected 1 recommendation (CustomRule only), got %d", len(result.Recommendations))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tidy/ -run TestScan_SkipsWombatManagedRules -v`
Expected: FAIL — "Read" is recommended

- [ ] **Step 3: Add wombat-managed rule filtering**

In `internal/tidy/tidy.go`, add `"slices"` to imports, then after `scopeProjects` is built (after the first `for` loop, before "Step 1" comment), add:

```go
// Build set of rules already managed by wombat per scope.
managedRules := make(map[string]map[string]bool) // scopeName -> "kind\x00rule" -> true
for _, scopeName := range cfg.ScopeNames() {
	m := make(map[string]bool)
	for _, r := range cfg.Permissions.Allow {
		if slices.Contains(r.Scopes, scopeName) {
			m["allow\x00"+r.Rule] = true
		}
	}
	for _, r := range cfg.Permissions.Deny {
		if slices.Contains(r.Scopes, scopeName) {
			m["deny\x00"+r.Rule] = true
		}
	}
	managedRules[scopeName] = m
}
```

Then in the Step 1 loop, add the skip check:

```go
for scopeName, projects := range scopeProjects {
	managed := managedRules[scopeName]
	for _, kind := range []string{"allow", "deny"} {
		counts := countRules(projects, kind)
		for rule, paths := range counts {
			if managed[kind+"\x00"+rule] {
				continue
			}
			result.Recommendations = append(result.Recommendations, Recommendation{
				Rule:        rule,
				Type:        kind,
				FoundIn:     paths,
				TargetScope: scopeName,
				Reason:      fmt.Sprintf("Found in %d project(s) under %s", len(paths), scopeName),
			})
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tidy/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tidy/tidy.go internal/tidy/tidy_test.go
git commit -m "fix: tidy skips rules already managed by wombat config"
```

---

### Task 5: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add project propagation documentation**

Add after the "Gotchas and constraints" section:

```markdown
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
```

Also add to the "Gotchas and constraints" list:

```markdown
12. **Project dir propagation**: Non-global scopes create symlinks and settings in every git repo under the scope's parent directory. Always uses `settings.local.json` for project dirs to avoid git pollution.
```

- [ ] **Step 2: Run full suite with race detector**

Run: `go test -race ./...`
Expected: all PASS, no races

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document scope propagation to project directories"
```
