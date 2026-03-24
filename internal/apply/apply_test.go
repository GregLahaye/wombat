package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
)

// setupTestEnv creates a temporary wombat environment with a source
// containing one skill and one agent. Returns the root dir.
func setupTestEnv(t *testing.T) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()

	// Set WOMBAT_HOME so config.SourcesDir() points here.
	t.Setenv("WOMBAT_HOME", root)

	// Create source with a skill and an agent.
	srcDir := filepath.Join(root, "sources", "test-source")
	skillDir := filepath.Join(srcDir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644)

	agentContent := "---\nname: My Agent\ndescription: Does things\n---\nBody.\n"
	os.WriteFile(filepath.Join(srcDir, "my-agent.md"), []byte(agentContent), 0o644)

	// Create scope directories.
	workScope := filepath.Join(root, "work-scope")
	globalScope := filepath.Join(root, "global-scope")
	os.MkdirAll(workScope, 0o755)
	os.MkdirAll(globalScope, 0o755)

	cfg := &config.Config{
		Scopes: map[string]config.Scope{
			"work":   {Path: workScope, SettingsFile: "settings.local.json"},
			"global": {Path: globalScope, SettingsFile: "settings.json"},
		},
		Sources: map[string]config.Source{
			"test-source": {Git: "https://example.com/repo", DefaultScope: []string{"work"}},
		},
		Plugins: map[string]config.ScopeSet{
			"test-plugin": {Enabled: []string{"global"}},
		},
		Permissions: config.Permissions{
			Allow: []config.PermissionRule{
				{Rule: "Read", Scopes: []string{"work", "global"}},
			},
		},
	}
	cfg.EnsureMaps()
	return root, cfg
}

func TestApply_CreatesSymlinksAndSettings(t *testing.T) {
	_, cfg := setupTestEnv(t)

	r, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(r.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", r.Errors)
	}

	// Should create symlinks for skill and agent in work scope.
	workSkill := filepath.Join(cfg.Scopes["work"].Path, "skills", "my-skill")
	if _, err := os.Lstat(workSkill); err != nil {
		t.Errorf("expected skill symlink at %s", workSkill)
	}
	workAgent := filepath.Join(cfg.Scopes["work"].Path, "agents", "my-agent")
	if _, err := os.Lstat(workAgent); err != nil {
		t.Errorf("expected agent symlink at %s", workAgent)
	}

	// Should NOT create symlinks at global (default_scope is [work] only).
	globalSkill := filepath.Join(cfg.Scopes["global"].Path, "skills", "my-skill")
	if _, err := os.Lstat(globalSkill); err == nil {
		t.Error("did not expect skill symlink at global scope")
	}

	// Should create settings files.
	workSettings := filepath.Join(cfg.Scopes["work"].Path, "settings.local.json")
	data, err := ReadSettings(workSettings)
	if err != nil {
		t.Fatalf("ReadSettings: %v", err)
	}
	// Work scope should have Read permission.
	perms, _ := data["permissions"].(map[string]any)
	allow := ExtractStringSlice(perms, "allow")
	if len(allow) != 1 || allow[0] != "Read" {
		t.Errorf("expected [Read] in work settings, got %v", allow)
	}

	// Global scope should have plugin enabled.
	globalSettings := filepath.Join(cfg.Scopes["global"].Path, "settings.json")
	gData, _ := ReadSettings(globalSettings)
	plugins, _ := gData["enabledPlugins"].(map[string]any)
	if v, _ := plugins["test-plugin"].(bool); !v {
		t.Error("expected test-plugin=true in global settings")
	}
}

func TestApply_Idempotent(t *testing.T) {
	_, cfg := setupTestEnv(t)

	r1, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	created := len(r1.Created)
	if created == 0 {
		t.Fatal("expected some created items on first apply")
	}

	// Second apply should be a no-op.
	r2, err := Apply(cfg, nil)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(r2.Created) != 0 || len(r2.Removed) != 0 || len(r2.Updated) != 0 {
		t.Errorf("second apply should be no-op, got created=%d removed=%d updated=%d",
			len(r2.Created), len(r2.Removed), len(r2.Updated))
	}
}

func TestApply_RemovesStaleOnConfigChange(t *testing.T) {
	_, cfg := setupTestEnv(t)

	// First apply creates everything.
	Apply(cfg, nil)

	// Change default_scope from [work] to [global].
	prevCfg := cfg.Clone()
	src := cfg.Sources["test-source"]
	src.DefaultScope = []string{"global"}
	cfg.Sources["test-source"] = src

	r, err := Apply(cfg, prevCfg)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Global optimization: skill should be at global only.
	globalSkill := filepath.Join(cfg.Scopes["global"].Path, "skills", "my-skill")
	if _, err := os.Lstat(globalSkill); err != nil {
		t.Error("expected skill symlink at global scope")
	}

	// Work scope symlinks should be removed.
	workSkill := filepath.Join(cfg.Scopes["work"].Path, "skills", "my-skill")
	if _, err := os.Lstat(workSkill); err == nil {
		t.Error("expected work scope skill symlink to be removed")
	}

	if len(r.Removed) == 0 {
		t.Error("expected some removed items")
	}
}

func TestPrintResult_NoChanges(t *testing.T) {
	r := &Result{}
	out := PrintResult(r)
	if !strings.Contains(out, "Everything up to date") {
		t.Errorf("expected 'Everything up to date', got %q", out)
	}
}

func TestPrintResult_WithChanges(t *testing.T) {
	r := &Result{
		Created: []string{"/a/skills/foo"},
		Removed: []string{"/a/skills/bar"},
		Updated: []string{"/a/settings.json"},
	}
	out := PrintResult(r)
	if !strings.Contains(out, "+ /a/skills/foo") {
		t.Error("missing created entry")
	}
	if !strings.Contains(out, "- /a/skills/bar") {
		t.Error("missing removed entry")
	}
	if !strings.Contains(out, "~ /a/settings.json") {
		t.Error("missing updated entry")
	}
	if !strings.Contains(out, "1 created, 1 removed, 1 updated, 0 errors") {
		t.Errorf("unexpected summary: %s", out)
	}
}
