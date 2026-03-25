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

	for _, name := range []string{"project-a", "project-b"} {
		os.MkdirAll(filepath.Join(root, name, ".git"), 0o755)
	}
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
