package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GregLahaye/wombat/internal/config"
	"github.com/GregLahaye/wombat/internal/resolve"
)

// syncSymlinks creates and removes symlinks for resolved items.
// Only removes symlinks pointing into the sources/ directory (partial ownership).
func syncSymlinks(cfg *config.Config, items []resolve.ResolvedItem, subdir string, r *Result) {
	sourcesDir := config.SourcesDir()

	// Build desired state: scope path -> link filename -> target path.
	desired := make(map[string]map[string]string)
	for _, item := range items {
		// Skip items with no SourcePath (from addExplicit when configured
		// but not discovered). We can't create a valid symlink without
		// knowing the item's path within the source repo.
		if item.SourcePath == "" {
			continue
		}
		absPath := filepath.Join(sourcesDir, item.SourceName, item.SourcePath)
		scopes := item.Scopes

		// Global optimization: if "global" is in scopes, only create at global.
		if slices.Contains(scopes, "global") {
			scopes = []string{"global"}
		}

		ln := item.LinkName()
		for _, scopeName := range scopes {
			scope, ok := cfg.Scopes[scopeName]
			if !ok {
				continue
			}
			if desired[scope.Path] == nil {
				desired[scope.Path] = make(map[string]string)
			}
			desired[scope.Path][ln] = absPath
		}
	}

	// Create/update desired symlinks.
	for scopePath, items := range desired {
		dir := filepath.Join(scopePath, subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("creating %s: %w", dir, err))
			continue
		}

		for name, target := range items {
			link := filepath.Join(dir, name)
			created, err := ensureSymlink(link, target)
			if err != nil {
				r.Errors = append(r.Errors, fmt.Errorf("symlink %s: %w", link, err))
				continue
			}
			if created {
				r.Created = append(r.Created, link)
			}
		}
	}

	// Remove stale symlinks (only those pointing into sources/).
	sourcesPrefix := filepath.Clean(sourcesDir) + string(filepath.Separator)
	for _, scope := range cfg.Scopes {
		dir := filepath.Join(scope.Path, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			link := filepath.Join(dir, entry.Name())
			target, err := os.Readlink(link)
			if err != nil {
				continue // Not a symlink.
			}
			// Resolve relative targets to absolute for comparison.
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			target = filepath.Clean(target)
			if !strings.HasPrefix(target, sourcesPrefix) {
				continue // Not managed by wombat.
			}
			if d, ok := desired[scope.Path]; ok {
				if _, wanted := d[entry.Name()]; wanted {
					continue
				}
			}
			if err := os.Remove(link); err == nil {
				r.Removed = append(r.Removed, link)
			}
		}
	}
}

// ensureSymlink creates or updates a symlink. Returns true if created/changed.
func ensureSymlink(link, target string) (bool, error) {
	existing, err := os.Readlink(link)
	if err == nil {
		if existing == target {
			return false, nil // Already correct.
		}
		if err := os.Remove(link); err != nil {
			return false, fmt.Errorf("removing old symlink: %w", err)
		}
	} else if _, statErr := os.Lstat(link); statErr == nil {
		// Path exists but is not a symlink (regular file or directory).
		return false, fmt.Errorf("non-symlink file exists at %s (remove manually)", link)
	}

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return false, err
	}
	return true, os.Symlink(target, link)
}
