package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkills_WithSKILLmd(t *testing.T) {
	dir := t.TempDir()
	// Create a valid skill directory.
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0o644)

	// Create a directory without SKILL.md (should be ignored).
	os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755)

	items, err := DiscoverSkills(dir, "")
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(items))
	}
	if items[0].Name != "my-skill" {
		t.Errorf("expected name 'my-skill', got %q", items[0].Name)
	}
	if items[0].Kind != "skill" {
		t.Errorf("expected kind 'skill', got %q", items[0].Kind)
	}
	if items[0].Path != "my-skill" {
		t.Errorf("expected path 'my-skill', got %q", items[0].Path)
	}
}

func TestDiscoverSkills_Subpath(t *testing.T) {
	dir := t.TempDir()
	// Skills in a subdirectory.
	os.MkdirAll(filepath.Join(dir, "skills", "cool-skill"), 0o755)
	os.WriteFile(filepath.Join(dir, "skills", "cool-skill", "SKILL.md"), []byte(""), 0o644)

	items, err := DiscoverSkills(dir, "skills")
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(items))
	}
	if items[0].Path != filepath.Join("skills", "cool-skill") {
		t.Errorf("expected path 'skills/cool-skill', got %q", items[0].Path)
	}
}

func TestDiscoverSkills_NonexistentDir(t *testing.T) {
	items, err := DiscoverSkills("/nonexistent", "")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items, got %v", items)
	}
}

func TestDiscoverAgents_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\nname: My Agent\ndescription: Does things\n---\nBody here.\n"
	os.WriteFile(filepath.Join(dir, "my-agent.md"), []byte(content), 0o644)

	// File without frontmatter (should be ignored).
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Just notes."), 0o644)

	// Readme should be skipped.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("---\nname: Readme\ndescription: skip\n---\n"), 0o644)

	items, err := DiscoverAgents(dir, "")
	if err != nil {
		t.Fatalf("DiscoverAgents: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(items))
	}
	if items[0].Name != "my-agent" {
		t.Errorf("expected name 'my-agent', got %q", items[0].Name)
	}
	if items[0].Kind != "agent" {
		t.Errorf("expected kind 'agent', got %q", items[0].Kind)
	}
}

func TestDiscoverAgents_Subpath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)
	content := "---\nname: Sub Agent\ndescription: In subdir\n---\n"
	os.WriteFile(filepath.Join(dir, "agents", "sub.md"), []byte(content), 0o644)

	items, err := DiscoverAgents(dir, "agents")
	if err != nil {
		t.Fatalf("DiscoverAgents: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(items))
	}
	if items[0].Path != filepath.Join("agents", "sub.md") {
		t.Errorf("expected path 'agents/sub.md', got %q", items[0].Path)
	}
}

func TestDiscoverAgents_SkipsNonAgent(t *testing.T) {
	dir := t.TempDir()

	// Missing description.
	os.WriteFile(filepath.Join(dir, "no-desc.md"), []byte("---\nname: Foo\n---\n"), 0o644)

	// Missing name.
	os.WriteFile(filepath.Join(dir, "no-name.md"), []byte("---\ndescription: Bar\n---\n"), 0o644)

	// No frontmatter delimiter.
	os.WriteFile(filepath.Join(dir, "plain.md"), []byte("name: Foo\ndescription: Bar\n"), 0o644)

	// Skip files.
	for _, name := range []string{"README.md", "CHANGELOG.md", "LICENSE.md", "CONTRIBUTING.md"} {
		os.WriteFile(filepath.Join(dir, name), []byte("---\nname: X\ndescription: Y\n---\n"), 0o644)
	}

	items, err := DiscoverAgents(dir, "")
	if err != nil {
		t.Fatalf("DiscoverAgents: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 agents, got %d: %v", len(items), items)
	}
}

func TestDiscoverAgents_NonexistentDir(t *testing.T) {
	items, err := DiscoverAgents("/nonexistent", "")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items, got %v", items)
	}
}

func TestParseFrontmatter(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		wantName bool
		wantDesc bool
	}{
		{"both", "---\nname: Foo\ndescription: Bar\n---\n", true, true},
		{"name only", "---\nname: Foo\n---\n", true, false},
		{"desc only", "---\ndescription: Bar\n---\n", false, true},
		{"empty values", "---\nname:\ndescription:\n---\n", false, false},
		{"no frontmatter", "Hello world\n", false, false},
		{"unclosed frontmatter", "---\nname: Foo\ndescription: Bar\n", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".md")
			os.WriteFile(path, []byte(tt.content), 0o644)
			hasName, hasDesc := parseFrontmatter(path)
			if hasName != tt.wantName {
				t.Errorf("hasName: got %v, want %v", hasName, tt.wantName)
			}
			if hasDesc != tt.wantDesc {
				t.Errorf("hasDesc: got %v, want %v", hasDesc, tt.wantDesc)
			}
		})
	}
}
