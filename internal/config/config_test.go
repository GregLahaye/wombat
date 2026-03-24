package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClone(t *testing.T) {
	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "settings.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com/repo"}},
		Skills:  map[string]Item{"skill1": {Source: "src", Enabled: []string{"work"}}},
	}
	cfg.EnsureMaps()

	clone := cfg.Clone()

	// Modify original — clone should be unaffected.
	cfg.Skills["skill2"] = Item{Source: "src"}
	if _, ok := clone.Skills["skill2"]; ok {
		t.Error("clone was mutated when original changed")
	}
}

func TestEqual(t *testing.T) {
	a := &Config{
		Scopes: map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
	}
	a.EnsureMaps()
	b := a.Clone()

	if !a.Equal(b) {
		t.Error("identical configs should be equal")
	}

	b.Scopes["personal"] = Scope{Path: "/personal", SettingsFile: "s.json"}
	if a.Equal(b) {
		t.Error("different configs should not be equal")
	}
}

func TestScopeNames_GlobalLast(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{
			"global": {Path: "/global"},
			"alpha":  {Path: "/alpha"},
			"beta":   {Path: "/beta"},
		},
	}
	cfg.EnsureMaps()

	names := cfg.ScopeNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "global" {
		t.Errorf("expected [alpha, beta, global], got %v", names)
	}
}

func TestScopeNames_NoGlobal(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{
			"work":     {Path: "/work"},
			"personal": {Path: "/personal"},
		},
	}
	cfg.EnsureMaps()

	names := cfg.ScopeNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "personal" || names[1] != "work" {
		t.Errorf("expected [personal, work], got %v", names)
	}
}

func TestValidate_DuplicateScopePath(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{
			"work":    {Path: "/shared/.claude", SettingsFile: "s.json"},
			"dup":     {Path: "/shared/.claude", SettingsFile: "s.json"},
		},
	}
	cfg.EnsureMaps()

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate scope paths")
	}
}

func TestValidate_EmptyRule(t *testing.T) {
	cfg := &Config{
		Permissions: Permissions{
			Allow: []PermissionRule{{Rule: ""}},
		},
	}
	cfg.EnsureMaps()

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for empty rule")
	}
}

func TestValidate_DuplicateRule(t *testing.T) {
	cfg := &Config{
		Scopes: map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Permissions: Permissions{
			Allow: []PermissionRule{
				{Rule: "Read", Scopes: []string{"work"}},
				{Rule: "Read", Scopes: []string{"work"}},
			},
		},
	}
	cfg.EnsureMaps()

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate rule")
	}
}

func TestValidate_UnknownScope(t *testing.T) {
	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com", DefaultScope: []string{"nonexistent"}}},
	}
	cfg.EnsureMaps()

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for unknown scope reference")
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	if err := AtomicWrite(path, data, 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestExpandContractPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	expanded := ExpandPath("~/work")
	expected := filepath.Join(home, "work")
	if expanded != expected {
		t.Errorf("ExpandPath: expected %s, got %s", expected, expanded)
	}

	contracted := ContractPath(expanded)
	if contracted != "~/work" {
		t.Errorf("ContractPath: expected ~/work, got %s", contracted)
	}
}

func TestEnsureMaps(t *testing.T) {
	cfg := &Config{}
	cfg.EnsureMaps()

	if cfg.Scopes == nil || cfg.Sources == nil || cfg.Plugins == nil ||
		cfg.Skills == nil || cfg.Agents == nil || cfg.Overrides == nil {
		t.Error("EnsureMaps left nil map")
	}
}

func TestLoadSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		Scopes: map[string]Scope{
			"work":   {Path: "/work/.claude", SettingsFile: "settings.local.json"},
			"global": {Path: "/home/user/.claude", SettingsFile: "settings.json"},
		},
		Sources: map[string]Source{
			"my-source": {
				Git:          "https://github.com/user/repo",
				DefaultScope: []string{"work", "global"},
				SkillPaths:   []string{"skills"},
			},
		},
		Skills: map[string]Item{
			"my-skill": {Source: "my-source", Enabled: []string{"work"}},
		},
		Permissions: Permissions{
			Allow: []PermissionRule{{Rule: "Read", Scopes: []string{"global"}}},
			Deny:  []PermissionRule{{Rule: "Bash(rm -rf:*)", Scopes: []string{"global"}}},
		},
	}
	original.EnsureMaps()

	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Paths are expanded on load, so compare with expanded paths.
	if len(loaded.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(loaded.Scopes))
	}
	if len(loaded.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(loaded.Sources))
	}
	if len(loaded.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(loaded.Skills))
	}
	if len(loaded.Permissions.Allow) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(loaded.Permissions.Allow))
	}
	if len(loaded.Permissions.Deny) != 1 {
		t.Errorf("expected 1 deny rule, got %d", len(loaded.Permissions.Deny))
	}
	if loaded.Sources["my-source"].Git != "https://github.com/user/repo" {
		t.Errorf("source git URL mismatch: %s", loaded.Sources["my-source"].Git)
	}
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestValidate_HappyPath(t *testing.T) {
	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com", DefaultScope: []string{"work"}}},
		Skills:  map[string]Item{"skill1": {Source: "src", Enabled: []string{"work"}}},
		Agents:  map[string]Item{"agent1": {Source: "src", Enabled: []string{"work"}}},
		Plugins: map[string]ScopeSet{"plugin1": {Enabled: []string{"work"}}},
		Permissions: Permissions{
			Allow: []PermissionRule{{Rule: "Read", Scopes: []string{"work"}}},
		},
	}
	cfg.EnsureMaps()

	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass validation: %v", err)
	}
}

func TestSkillDirs_Default(t *testing.T) {
	s := Source{Git: "https://example.com"}
	dirs := s.SkillDirs()
	if len(dirs) != 1 || dirs[0] != "" {
		t.Errorf("expected [\"\"], got %v", dirs)
	}
}

func TestSkillDirs_Custom(t *testing.T) {
	s := Source{Git: "https://example.com", SkillPaths: []string{"skills", "workflows"}}
	dirs := s.SkillDirs()
	if len(dirs) != 2 || dirs[0] != "skills" || dirs[1] != "workflows" {
		t.Errorf("expected [skills, workflows], got %v", dirs)
	}
}

func TestScopeRefs(t *testing.T) {
	cfg := &Config{
		Scopes:  map[string]Scope{"work": {Path: "/work", SettingsFile: "s.json"}},
		Sources: map[string]Source{"src": {Git: "https://example.com", DefaultScope: []string{"work"}}},
		Skills:  map[string]Item{"skill1": {Source: "src", Enabled: []string{"work"}}},
	}
	cfg.EnsureMaps()

	refs := cfg.ScopeRefs("work")
	if len(refs) != 2 {
		t.Errorf("expected 2 refs, got %d: %v", len(refs), refs)
	}
}
