package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
)

func TestSyncSymlinks_CreatesSymlinks(t *testing.T) {
	dir := t.TempDir()
	scopePath := filepath.Join(dir, "scope")
	sourcesDir := filepath.Join(dir, "sources")
	skillTarget := filepath.Join(sourcesDir, "my-source", "my-skill")
	os.MkdirAll(skillTarget, 0o755)
	os.WriteFile(filepath.Join(skillTarget, "SKILL.md"), []byte(""), 0o644)

	// Override SourcesDir for testing.
	origDir := os.Getenv("WOMBAT_HOME")
	os.Setenv("WOMBAT_HOME", dir)
	defer os.Setenv("WOMBAT_HOME", origDir)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	items := []resolve.ResolvedItem{
		{Name: "my-skill", SourceName: "my-source", SourcePath: "my-skill", Scopes: []string{"work"}, Kind: "skill"},
	}

	r := &Result{}
	syncSymlinks(cfg, items, "skills", r)

	if len(r.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}
	if len(r.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(r.Created))
	}

	link := filepath.Join(scopePath, "skills", "my-skill")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != skillTarget {
		t.Errorf("expected target %q, got %q", skillTarget, target)
	}
}

func TestSyncSymlinks_RemovesStaleSymlinks(t *testing.T) {
	dir := t.TempDir()
	scopePath := filepath.Join(dir, "scope")
	sourcesDir := filepath.Join(dir, "sources")
	os.MkdirAll(filepath.Join(sourcesDir, "my-source", "old-skill"), 0o755)

	origDir := os.Getenv("WOMBAT_HOME")
	os.Setenv("WOMBAT_HOME", dir)
	defer os.Setenv("WOMBAT_HOME", origDir)

	// Create a stale symlink pointing into sources/.
	skillsDir := filepath.Join(scopePath, "skills")
	os.MkdirAll(skillsDir, 0o755)
	staleTarget := filepath.Join(sourcesDir, "my-source", "old-skill")
	os.Symlink(staleTarget, filepath.Join(skillsDir, "old-skill"))

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	// No items desired — old-skill should be removed.
	r := &Result{}
	syncSymlinks(cfg, nil, "skills", r)

	if len(r.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d: %v", len(r.Removed), r.Removed)
	}
}

func TestSyncSymlinks_PreservesNonSourceSymlinks(t *testing.T) {
	dir := t.TempDir()
	scopePath := filepath.Join(dir, "scope")
	sourcesDir := filepath.Join(dir, "sources")
	os.MkdirAll(sourcesDir, 0o755)

	origDir := os.Getenv("WOMBAT_HOME")
	os.Setenv("WOMBAT_HOME", dir)
	defer os.Setenv("WOMBAT_HOME", origDir)

	// Create a symlink NOT pointing into sources/ (user-managed).
	skillsDir := filepath.Join(scopePath, "skills")
	os.MkdirAll(skillsDir, 0o755)
	userTarget := filepath.Join(dir, "user-skills", "custom")
	os.MkdirAll(userTarget, 0o755)
	os.Symlink(userTarget, filepath.Join(skillsDir, "custom"))

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work": {Path: scopePath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	r := &Result{}
	syncSymlinks(cfg, nil, "skills", r)

	// Should NOT be removed (not managed by wombat).
	if len(r.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d: %v", len(r.Removed), r.Removed)
	}

	// Verify symlink still exists.
	if _, err := os.Lstat(filepath.Join(skillsDir, "custom")); err != nil {
		t.Error("user symlink was removed")
	}
}

func TestSyncSymlinks_GlobalOptimization(t *testing.T) {
	dir := t.TempDir()
	workPath := filepath.Join(dir, "work-scope")
	globalPath := filepath.Join(dir, "global-scope")
	sourcesDir := filepath.Join(dir, "sources")
	skillTarget := filepath.Join(sourcesDir, "src", "my-skill")
	os.MkdirAll(skillTarget, 0o755)

	origDir := os.Getenv("WOMBAT_HOME")
	os.Setenv("WOMBAT_HOME", dir)
	defer os.Setenv("WOMBAT_HOME", origDir)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: workPath, SettingsFile: "settings.json"},
			"global": {Path: globalPath, SettingsFile: "settings.json"},
		},
	}
	cfg.EnsureMaps()

	// Item has both scopes — should only create at global.
	items := []resolve.ResolvedItem{
		{Name: "my-skill", SourceName: "src", SourcePath: "my-skill", Scopes: []string{"work", "global"}, Kind: "skill"},
	}

	r := &Result{}
	syncSymlinks(cfg, items, "skills", r)

	if len(r.Created) != 1 {
		t.Fatalf("expected 1 created (global only), got %d: %v", len(r.Created), r.Created)
	}
	// Should be in global, not work.
	if _, err := os.Lstat(filepath.Join(globalPath, "skills", "my-skill")); err != nil {
		t.Error("expected symlink at global scope")
	}
	if _, err := os.Lstat(filepath.Join(workPath, "skills", "my-skill")); err == nil {
		t.Error("did not expect symlink at work scope (global optimization)")
	}
}

func TestEnsureSymlink_RegularFileBlocks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.MkdirAll(target, 0o755)
	link := filepath.Join(dir, "blocker")

	// Create a regular file where the symlink should go.
	os.WriteFile(link, []byte("I am a regular file"), 0o644)

	_, err := ensureSymlink(link, target)
	if err == nil {
		t.Fatal("expected error when regular file blocks symlink, got nil")
	}
	// Error should mention that a non-symlink file is blocking.
	if !strings.Contains(err.Error(), "non-symlink") {
		t.Errorf("error should mention non-symlink file, got: %v", err)
	}

	// Original file should be preserved (not deleted).
	data, readErr := os.ReadFile(link)
	if readErr != nil {
		t.Fatalf("regular file was deleted: %v", readErr)
	}
	if string(data) != "I am a regular file" {
		t.Error("regular file contents were modified")
	}
}

func TestEnsureSymlink_Idempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	os.MkdirAll(target, 0o755)
	link := filepath.Join(dir, "link")

	// First call creates.
	created, err := ensureSymlink(link, target)
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	if !created {
		t.Error("expected created=true on first call")
	}

	// Second call is a no-op.
	created, err = ensureSymlink(link, target)
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	if created {
		t.Error("expected created=false on second call (idempotent)")
	}
}

func TestEnsureSymlink_UpdatesTarget(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old")
	new := filepath.Join(dir, "new")
	os.MkdirAll(old, 0o755)
	os.MkdirAll(new, 0o755)
	link := filepath.Join(dir, "link")

	ensureSymlink(link, old)

	created, err := ensureSymlink(link, new)
	if err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	if !created {
		t.Error("expected created=true when target changes")
	}
	got, _ := os.Readlink(link)
	if got != new {
		t.Errorf("expected target %q, got %q", new, got)
	}
}
