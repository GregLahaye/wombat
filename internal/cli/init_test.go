package cli

import (
	"path/filepath"
	"strings"
	"testing"
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
