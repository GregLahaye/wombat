package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/GregLahaye/wombat/internal/config"
)

// appendClaudeIfMissing mirrors the logic in scope add and init.
func appendClaudeIfMissing(p string) string {
	p = filepath.Clean(p)
	if !strings.HasSuffix(p, ".claude") {
		p = filepath.Join(p, ".claude")
	}
	return p
}

func TestAppendClaude_TrailingSlash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"~/work", "~/work/.claude"},
		{"~/work/.claude", "~/work/.claude"},
		{"~/work/.claude/", "~/work/.claude"},       // trailing slash
		{"/work/.claude///", "/work/.claude"},        // multiple trailing slashes
		{"/work/not-claude", "/work/not-claude/.claude"},
	}
	for _, tt := range tests {
		got := appendClaudeIfMissing(tt.input)
		if got != tt.want {
			t.Errorf("appendClaudeIfMissing(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestImportItem_MergesScopes(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()

	// First import: creates the item with "work" scope.
	importItem(cfg, "helper", "skill", "my-source", "work")
	if got := cfg.Skills["helper"].Enabled; len(got) != 1 || got[0] != "work" {
		t.Fatalf("expected [work], got %v", got)
	}

	// Second import of same item in different scope: should merge, not skip.
	importItem(cfg, "helper", "skill", "my-source", "personal")
	got := cfg.Skills["helper"].Enabled
	if len(got) != 2 {
		t.Fatalf("expected 2 scopes after merge, got %v", got)
	}
	if !slices.Contains(got, "work") || !slices.Contains(got, "personal") {
		t.Errorf("expected [work, personal], got %v", got)
	}

	// Duplicate scope should not be added again.
	importItem(cfg, "helper", "skill", "my-source", "work")
	if len(cfg.Skills["helper"].Enabled) != 2 {
		t.Errorf("expected 2 scopes (no duplicate), got %v", cfg.Skills["helper"].Enabled)
	}
}

func TestImportItem_Agent(t *testing.T) {
	cfg := &config.Config{}
	cfg.EnsureMaps()

	importItem(cfg, "my-agent", "agent", "my-source", "global")
	importItem(cfg, "my-agent", "agent", "my-source", "work")

	got := cfg.Agents["my-agent"].Enabled
	if len(got) != 2 {
		t.Fatalf("expected 2 scopes for agent, got %v", got)
	}
}

func TestSourceNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo", "owner-repo"},
		{"https://github.com/owner/repo.git", "owner-repo"},
		{"git@github.com:owner/repo.git", "owner-repo"},
		{"git@github.com:owner/repo", "owner-repo"},
		{"ssh://git@github.com/owner/repo.git", "owner-repo"},
		{"https://gitlab.com/group/subgroup/repo.git", "subgroup-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := sourceNameFromURL(tt.url)
			if got != tt.want {
				t.Errorf("sourceNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
