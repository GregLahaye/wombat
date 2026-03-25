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
