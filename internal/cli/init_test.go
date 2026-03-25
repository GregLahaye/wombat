package cli

import "testing"

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
