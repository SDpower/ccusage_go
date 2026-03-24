package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
)

// FileState tracks the state of a single JSONL file
type FileState struct {
	ModTime time.Time
	Size    int64
}

// ProjectCache holds cached data for a single project directory
type ProjectCache struct {
	DirPath   string
	Files     map[string]FileState // filename → state
	Entries   []types.UsageEntry   // deduplicated, cost-calculated entries
	DedupeMap map[string]bool      // uniqueHash → seen (per-project dedup)
}

// IncrementalCache is the top-level cache keyed by project directory path
type IncrementalCache struct {
	projects      map[string]*ProjectCache
	mergedEntries []types.UsageEntry // last merged result
	dirty         bool               // whether any project changed
}

// NewIncrementalCache creates a new empty incremental cache
func NewIncrementalCache() *IncrementalCache {
	return &IncrementalCache{
		projects: make(map[string]*ProjectCache),
	}
}

// Update performs incremental loading and returns merged entries and whether data changed
func (ic *IncrementalCache) Update(
	l *Loader,
	calculator CostCalculator,
	basePath string,
	modifiedWithin time.Duration,
) (entries []types.UsageEntry, changed bool, err error) {
	ic.dirty = false

	// Resolve projects path
	projectsPath := filepath.Join(basePath, "projects")
	if _, statErr := os.Stat(projectsPath); statErr == nil {
		basePath = projectsPath
	}

	// Phase 1: Find all project directories
	projectDirs, err := l.findProjectDirectories(basePath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find project directories: %w", err)
	}

	cutoffTime := time.Now().Add(-modifiedWithin)

	// Phase 2: Detect removed projects
	currentProjects := make(map[string]bool, len(projectDirs))
	for _, dir := range projectDirs {
		currentProjects[dir] = true
	}
	for dir := range ic.projects {
		if !currentProjects[dir] {
			delete(ic.projects, dir)
			ic.dirty = true
		}
	}

	// Phase 3: Process each project
	for _, projectDir := range projectDirs {
		// Skip inactive projects
		if modifiedWithin > 0 && l.shouldSkipProject(projectDir, cutoffTime) {
			continue
		}

		dirEntries, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		// Collect current JSONL files with their state
		currentFiles := make(map[string]FileState)
		for _, de := range dirEntries {
			if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".jsonl") {
				continue
			}
			info, err := de.Info()
			if err != nil {
				continue
			}
			// Apply time filter
			if modifiedWithin > 0 && info.ModTime().Before(cutoffTime) {
				continue
			}
			currentFiles[de.Name()] = FileState{
				ModTime: info.ModTime(),
				Size:    info.Size(),
			}
		}

		if len(currentFiles) == 0 {
			// No relevant files in this project
			if _, exists := ic.projects[projectDir]; exists {
				delete(ic.projects, projectDir)
				ic.dirty = true
			}
			continue
		}

		pc, exists := ic.projects[projectDir]
		if !exists {
			// New project — full load
			pc = &ProjectCache{
				DirPath:   projectDir,
				Files:     make(map[string]FileState),
				DedupeMap: make(map[string]bool),
			}
			ic.projects[projectDir] = pc
		}

		// Detect deleted files → full reload of project
		needFullReload := false
		if exists {
			for name := range pc.Files {
				if _, ok := currentFiles[name]; !ok {
					needFullReload = true
					break
				}
			}
		}

		if needFullReload {
			// Reset project cache for full reload
			pc.Files = make(map[string]FileState)
			pc.Entries = nil
			pc.DedupeMap = make(map[string]bool)
		}

		// Find changed/new files
		var filesToLoad []string
		for name, state := range currentFiles {
			if oldState, ok := pc.Files[name]; !ok || oldState.ModTime != state.ModTime || oldState.Size != state.Size {
				filesToLoad = append(filesToLoad, filepath.Join(projectDir, name))
			}
		}

		if !exists || needFullReload {
			// For new or full-reload projects, load all current files
			filesToLoad = nil
			for name := range currentFiles {
				filesToLoad = append(filesToLoad, filepath.Join(projectDir, name))
			}
		}

		if len(filesToLoad) == 0 {
			continue // No changes in this project
		}

		// Load changed files
		ic.dirty = true
		for _, filePath := range filesToLoad {
			fileEntries, _, loadErr := l.loadFileWithDedupe(filePath, pc.DedupeMap)
			if loadErr != nil {
				if l.debug {
					fmt.Fprintf(os.Stderr, "Debug: Error loading file %s: %v\n", filePath, loadErr)
				}
				continue
			}

			// Calculate costs and clear Raw data
			if calculator != nil {
				for i := range fileEntries {
					calculator.CalculateCost(&fileEntries[i])
					if fileEntries[i].Raw != nil {
						cacheData := make(map[string]interface{})
						if cc, ex := fileEntries[i].Raw["cache_creation_input_tokens"]; ex {
							cacheData["cache_creation_input_tokens"] = cc
						}
						if cr, ex := fileEntries[i].Raw["cache_read_input_tokens"]; ex {
							cacheData["cache_read_input_tokens"] = cr
						}
						if resetTime, ex := fileEntries[i].Raw["usage_limit_reset_time"]; ex {
							cacheData["usage_limit_reset_time"] = resetTime
						}
						if len(cacheData) > 0 {
							fileEntries[i].Raw = cacheData
						} else {
							fileEntries[i].Raw = nil
						}
					}
				}
			}

			pc.Entries = append(pc.Entries, fileEntries...)
		}

		// Update file states
		for name, state := range currentFiles {
			pc.Files[name] = state
		}
	}

	// If nothing changed, return cached result
	if !ic.dirty {
		return ic.mergedEntries, false, nil
	}

	// Merge all project entries
	totalLen := 0
	for _, pc := range ic.projects {
		totalLen += len(pc.Entries)
	}

	merged := make([]types.UsageEntry, 0, totalLen)
	// Global dedup across projects (for safety, though cross-project dupes are rare)
	globalDedup := make(map[string]bool, totalLen)
	for _, pc := range ic.projects {
		for _, entry := range pc.Entries {
			key := entryDedupeKey(entry)
			if key != "" && globalDedup[key] {
				continue
			}
			if key != "" {
				globalDedup[key] = true
			}
			merged = append(merged, entry)
		}
	}

	ic.mergedEntries = merged
	return merged, true, nil
}

// entryDedupeKey generates a dedup key from an entry's ID and session
func entryDedupeKey(e types.UsageEntry) string {
	if e.ID != "" && e.SessionID != "" {
		return e.ID + ":" + e.SessionID
	}
	return ""
}

// Reset clears all cached data
func (ic *IncrementalCache) Reset() {
	ic.projects = make(map[string]*ProjectCache)
	ic.mergedEntries = nil
	ic.dirty = false
}

// Stats returns cache statistics for debugging
func (ic *IncrementalCache) Stats() (projectCount, totalEntries, totalFiles int) {
	projectCount = len(ic.projects)
	for _, pc := range ic.projects {
		totalEntries += len(pc.Entries)
		totalFiles += len(pc.Files)
	}
	return
}

// UpdateWithContext is like Update but accepts a context for cancellation
func (ic *IncrementalCache) UpdateWithContext(
	ctx context.Context,
	l *Loader,
	calculator CostCalculator,
	basePath string,
	modifiedWithin time.Duration,
) (entries []types.UsageEntry, changed bool, err error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
		return ic.Update(l, calculator, basePath, modifiedWithin)
	}
}
