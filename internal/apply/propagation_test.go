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

	drifts, err := CheckSettings(cfg, projDirs)
	if err != nil {
		t.Fatalf("CheckSettings: %v", err)
	}
	if len(drifts) == 0 {
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
