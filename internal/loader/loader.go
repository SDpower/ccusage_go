package loader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
)

// CostCalculator interface for optional cost calculation during loading
type CostCalculator interface {
	CalculateCost(entry *types.UsageEntry) error
}

// rawRetainKeys lists raw JSONL fields that must survive Raw cleanup.
// cache_* token fields are now first-class on UsageEntry, so they no longer
// need preserving in Raw. usage_limit_reset_time is still consumed by blocks.
var rawRetainKeys = []string{"usage_limit_reset_time"}

// clearRawExceptKeys drops every Raw entry except the named keys, keeping
// memory bounded after first-class fields are populated.
func clearRawExceptKeys(entry *types.UsageEntry, keep []string) {
	if entry.Raw == nil {
		return
	}
	preserved := make(map[string]interface{}, len(keep))
	for _, k := range keep {
		if v, ok := entry.Raw[k]; ok {
			preserved[k] = v
		}
	}
	if len(preserved) == 0 {
		entry.Raw = nil
		return
	}
	entry.Raw = preserved
}

// LoaderOptions configures optional loading behaviors
type LoaderOptions struct {
	OnlyActiveSession bool          // Only load active session data
	ModifiedWithin    time.Duration // Only load files modified within this duration
	MaxFiles          int           // Maximum number of files to load (0 = unlimited)
	StreamProcessing  bool          // Enable stream processing - calculate costs immediately after reading each file
	Calculator        CostCalculator // Optional calculator for stream processing
}

type Loader struct {
	maxWorkers int
	debug      bool
	timezone   *time.Location
}

func New() *Loader {
	return &Loader{
		maxWorkers: 1, // Single worker to minimize CPU and memory usage
		debug:      false,
		timezone:   time.Local,
	}
}

func (l *Loader) SetDebug(debug bool) {
	l.debug = debug
}

func (l *Loader) SetTimezone(timezone *time.Location) {
	l.timezone = timezone
}

// SetMaxWorkers sets the maximum number of concurrent file read workers
// This is useful for reducing CPU usage in live monitoring mode
func (l *Loader) SetMaxWorkers(workers int) {
	if workers > 0 {
		l.maxWorkers = workers
	}
}

func (l *Loader) LoadFromPath(ctx context.Context, path string) ([]types.UsageEntry, error) {
	// Use default options (load all files)
	return l.LoadFromPathWithOptions(ctx, path, nil)
}

// LoadFromPathWithOptions loads usage data with optional filters
func (l *Loader) LoadFromPathWithOptions(ctx context.Context, path string, options *LoaderOptions) ([]types.UsageEntry, error) {
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if l.debug {
			fmt.Fprintf(os.Stderr, "Debug: Path does not exist: %s\n", path)
		}
		return nil, fmt.Errorf("path does not exist: %s", path)
	}
	
	// Look for JSONL files in projects subdirectory
	projectsPath := filepath.Join(path, "projects")
	if _, err := os.Stat(projectsPath); err == nil {
		path = projectsPath
	}
	
	// Find files with optional filtering
	var paths []string
	var err error
	if options != nil && (options.OnlyActiveSession || options.ModifiedWithin > 0) {
		paths, err = l.findJSONLFilesWithFilter(path, options)
	} else {
		paths, err = l.findJSONLFiles(path)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find JSONL files: %w", err)
	}

	// Apply MaxFiles limit if specified
	if options != nil && options.MaxFiles > 0 && len(paths) > options.MaxFiles {
		// Sort by modification time (newest first) and take top MaxFiles
		sortedPaths, _ := l.sortFilesByModTime(paths)
		paths = sortedPaths[:options.MaxFiles]
		if l.debug {
			fmt.Fprintf(os.Stderr, "Debug: Limited to %d most recent files\n", options.MaxFiles)
		}
	}

	if l.debug {
		fmt.Fprintf(os.Stderr, "Debug: Found %d JSONL files in %s\n", len(paths), path)
		if options != nil && options.ModifiedWithin > 0 {
			fmt.Fprintf(os.Stderr, "Debug: Filtered to files modified within %v\n", options.ModifiedWithin)
		}
		if len(paths) > 0 && len(paths) <= 5 {
			for _, p := range paths {
				fmt.Fprintf(os.Stderr, "  - %s\n", p)
			}
		}
	}

	if len(paths) == 0 {
		return nil, types.ErrDataNotFound
	}

	// Sort files by earliest timestamp (like TypeScript version)
	sortedPaths, err := l.sortFilesByTimestamp(paths)
	if err != nil {
		// If sorting fails, use original order
		sortedPaths = paths
		if l.debug {
			fmt.Fprintf(os.Stderr, "Debug: Failed to sort files by timestamp, using default order: %v\n", err)
		}
	} else {
		paths = sortedPaths
		if l.debug {
			fmt.Fprintf(os.Stderr, "Debug: Sorted files by timestamp\n")
		}
	}

	// Use LoadParallelWithOptions if stream processing is enabled
	var entries []types.UsageEntry
	if options != nil && options.StreamProcessing {
		entries, err = l.LoadParallelWithOptions(ctx, paths, options)
	} else {
		entries, err = l.LoadParallel(ctx, paths)
	}
	
	if l.debug {
		fmt.Fprintf(os.Stderr, "Debug: Loaded %d usage entries\n", len(entries))
		if options != nil && options.StreamProcessing {
			fmt.Fprintf(os.Stderr, "Debug: Stream processing enabled - costs calculated during loading\n")
		}
		
		// Count valid entries (any entry with timestamp is valid)
		validCount := 0
		for _, e := range entries {
			if !e.Timestamp.IsZero() {
				validCount++
			}
		}
		fmt.Fprintf(os.Stderr, "Debug: %d entries have valid timestamps\n", validCount)
	}
	
	return entries, err
}

func (l *Loader) LoadParallel(ctx context.Context, paths []string) ([]types.UsageEntry, error) {
	return l.LoadParallelWithOptions(ctx, paths, nil)
}

func (l *Loader) LoadParallelWithOptions(ctx context.Context, paths []string, options *LoaderOptions) ([]types.UsageEntry, error) {
	type result struct {
		entries      []types.UsageEntry
		sessionNames map[string]string
		err          error
	}

	jobs := make(chan string, len(paths))
	results := make(chan result, len(paths))

	var wg sync.WaitGroup
	workers := l.maxWorkers
	if workers > len(paths) {
		workers = len(paths)
	}

	// Global deduplication map shared across all files
	var dedupeMutex sync.Mutex
	globalDedupeMap := make(map[string]bool)

	// Start workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					entries, sessionNames, err := l.loadFileWithGlobalDedupe(path, &dedupeMutex, globalDedupeMap)

					// Stream processing: calculate costs immediately if enabled
					if options != nil && options.StreamProcessing && options.Calculator != nil && err == nil {
						for i := range entries {
							options.Calculator.CalculateCost(&entries[i])
							clearRawExceptKeys(&entries[i], rawRetainKeys)
						}
					}

					results <- result{entries: entries, sessionNames: sessionNames, err: err}
				}
			}
		}()
	}

	// Send jobs
	go func() {
		defer close(jobs)
		for _, path := range paths {
			select {
			case <-ctx.Done():
				return
			case jobs <- path:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var allEntries []types.UsageEntry
	var errors []error
	globalSessionNames := make(map[string]string)

	for res := range results {
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			allEntries = append(allEntries, res.entries...)
			// Merge per-file session name maps (custom-title takes priority)
			for sid, name := range res.sessionNames {
				if _, exists := globalSessionNames[sid]; !exists {
					globalSessionNames[sid] = name
				}
			}
		}
	}

	if len(errors) > 0 && len(allEntries) == 0 {
		return nil, fmt.Errorf("failed to load any files: %v", errors[0])
	}

	// Global backfill: apply session names across all entries
	for i := range allEntries {
		if name, ok := globalSessionNames[allEntries[i].SessionID]; ok {
			allEntries[i].SessionName = name
		}
	}

	return allEntries, nil
}

func (l *Loader) loadFile(path string) ([]types.UsageEntry, map[string]string, error) {
	// Legacy function - redirect to new version with local dedupe
	dedupeMap := make(map[string]bool)
	return l.loadFileWithDedupe(path, dedupeMap)
}

func (l *Loader) loadFileWithGlobalDedupe(path string, dedupeMutex *sync.Mutex, globalDedupeMap map[string]bool) ([]types.UsageEntry, map[string]string, error) {
	return l.loadFileWithDedupe(path, globalDedupeMap, dedupeMutex)
}

// clearRawData removes Raw data from entries to save memory
func clearRawData(entries []types.UsageEntry) {
	for i := range entries {
		entries[i].Raw = nil
	}
}

func (l *Loader) loadFileWithDedupe(path string, dedupeMap map[string]bool, dedupeMutex ...*sync.Mutex) ([]types.UsageEntry, map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, types.LoaderError{Path: path, Err: err}
	}
	defer file.Close()

	// Extract project path from file path
	// File path format: /path/to/claude/projects/project-name/YYYY/MM/DD/file.jsonl
	projectPath := l.extractProjectPath(path)

	var entries []types.UsageEntry
	scanner := bufio.NewScanner(file)
	
	// Increase buffer size to handle very long lines (like TypeScript version)
	buf := make([]byte, 0, 64*1024)  // Start with 64KB
	scanner.Buffer(buf, 1024*1024)  // Allow up to 1MB per line
	
	lineNum := 0
	parseErrors := 0
	firstError := ""
	sessionNameMap := make(map[string]string)

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			parseErrors++
			if firstError == "" && l.debug {
				firstError = fmt.Sprintf("Line %d: JSON parse error: %v", lineNum, err)
			}
			continue // Skip malformed JSON lines
		}

		// Intercept custom-title and agent-name entries for session name mapping
		if typeStr, ok := raw["type"].(string); ok {
			if typeStr == "custom-title" {
				if title, ok := raw["customTitle"].(string); ok {
					if sid, ok := raw["sessionId"].(string); ok {
						sessionNameMap[sid] = title
					}
				}
				continue
			}
			if typeStr == "agent-name" {
				if name, ok := raw["agentName"].(string); ok {
					if sid, ok := raw["sessionId"].(string); ok {
						if _, exists := sessionNameMap[sid]; !exists {
							sessionNameMap[sid] = name
						}
					}
				}
				continue
			}
			// ai-title is the third-priority source for SessionName.
			// Order: custom-title > agent-name > ai-title.
			if typeStr == "ai-title" {
				if title, ok := raw["aiTitle"].(string); ok {
					if sid, ok := raw["sessionId"].(string); ok {
						if _, exists := sessionNameMap[sid]; !exists {
							sessionNameMap[sid] = title
						}
					}
				}
				continue
			}
		}

		// Try to parse entry according to TypeScript schema rules
		entry, err := l.parseEntry(raw, projectPath)
		entry.SourceFile = path
		if err != nil {
			// TypeScript version would skip this line silently
			// Only count as parse error if it's an actual JSON structure we expect to handle
			if l.shouldCountAsParseError(err, raw) {
				parseErrors++
				if firstError == "" && l.debug {
					firstError = fmt.Sprintf("Line %d: Entry parse error: %v", lineNum, err)
				}
			}
			continue // Skip entries that fail to parse
		}

		// Skip entries with zero timestamp (invalid date)
		if entry.Timestamp.IsZero() || entry.Timestamp.Year() < 2020 {
			continue
		}
		
		// Skip synthetic model entries (matches TypeScript behavior)
		if entry.Model == "<synthetic>" {
			continue
		}
		
		// Implement deduplication based on message ID and request ID (like TypeScript)
		uniqueHash := l.createUniqueHash(raw)
		if uniqueHash != "" {
			// Use mutex if provided (for global dedupe)
			if len(dedupeMutex) > 0 && dedupeMutex[0] != nil {
				dedupeMutex[0].Lock()
				if dedupeMap[uniqueHash] {
					dedupeMutex[0].Unlock()
					continue // Skip duplicate
				}
				dedupeMap[uniqueHash] = true
				dedupeMutex[0].Unlock()
			} else {
				// Local dedupe without mutex
				if dedupeMap[uniqueHash] {
					continue // Skip duplicate
				}
				dedupeMap[uniqueHash] = true
			}
		}

		// Drop bulky Raw fields; retain usage_limit_reset_time (used by blocks).
		clearRawExceptKeys(&entry, rawRetainKeys)

		entries = append(entries, entry)
	}

	if l.debug && parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "Debug: File %s had %d parse errors\n", filepath.Base(path), parseErrors)
		if firstError != "" {
			fmt.Fprintf(os.Stderr, "  First error: %s\n", firstError)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, types.LoaderError{Path: path, Err: err}
	}

	return entries, sessionNameMap, nil
}

func (l *Loader) parseEntry(raw map[string]interface{}, filePath string) (types.UsageEntry, error) {
	entry := types.UsageEntry{Raw: raw}

	// Debug: print first entry structure (simple approach for now)
	// This is just for debugging
	// TODO: use sync.Once for production code

	if id, ok := raw["id"].(string); ok {
		entry.ID = id
	}

	// Parse timestamp - try multiple formats
	if ts, ok := raw["timestamp"].(string); ok {
		// Try multiple time formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05.999Z",
		}
		
		var parsedTime time.Time
		var parseErr error
		for _, format := range formats {
			parsedTime, parseErr = time.Parse(format, ts)
			if parseErr == nil {
				entry.Timestamp = parsedTime
				break
			}
		}
		
		// If all formats fail, try parsing as Unix timestamp
		if parseErr != nil {
			if tsFloat, ok := raw["timestamp"].(float64); ok {
				entry.Timestamp = time.Unix(int64(tsFloat), 0)
			}
		}
	} else if tsFloat, ok := raw["timestamp"].(float64); ok {
		// Handle numeric timestamp
		entry.Timestamp = time.Unix(int64(tsFloat), 0)
	}

	// Apply timezone conversion and set DateKey (matching TypeScript's formatDate)
	if !entry.Timestamp.IsZero() && l.timezone != nil {
		timeInZone := entry.Timestamp.In(l.timezone)
		entry.DateKey = timeInZone.Format("2006-01-02")
	}

	if projectPath, ok := raw["project_path"].(string); ok && projectPath != "" {
		entry.ProjectPath = projectPath
	} else {
		// Use the project path extracted from file path if not in JSON
		entry.ProjectPath = filePath
	}

	if model, ok := raw["model"].(string); ok {
		entry.Model = model
	}

	// Validate entry according to TypeScript usageDataSchema
	if err := l.validateUsageData(raw, &entry); err != nil {
		return types.UsageEntry{}, err
	}

	// Top-level cache_* fields are exceedingly rare in real Anthropic JSONL
	// (cache tokens live under message.usage). Read them only as a final
	// fallback when validateUsageData did not populate the first-class fields.
	if entry.CacheCreationInputTokens == 0 {
		if v, ok := raw["cache_creation_input_tokens"].(float64); ok {
			entry.CacheCreationInputTokens = int(v)
		}
	}
	if entry.CacheReadInputTokens == 0 {
		if v, ok := raw["cache_read_input_tokens"].(float64); ok {
			entry.CacheReadInputTokens = int(v)
		}
	}

	// IsSidechain / IsMeta — preserve metadata for breakdown reports.
	// NEVER use these to filter; sub-agent calls are independently billed.
	if v, ok := raw["isSidechain"].(bool); ok {
		entry.IsSidechain = v
	}
	if v, ok := raw["isMeta"].(bool); ok {
		entry.IsMeta = v
	}

	// Calculate total tokens after all token fields are populated.
	l.calculateTotalTokens(&entry)

	if cost, ok := raw["cost"].(float64); ok {
		entry.Cost = cost
	} else if costUSD, ok := raw["costUSD"].(float64); ok {
		entry.Cost = costUSD
	}

	if sessionID, ok := raw["session_id"].(string); ok {
		entry.SessionID = sessionID
	}

	if blockType, ok := raw["block_type"].(string); ok {
		entry.BlockType = blockType
	}

	return entry, nil
}

func (l *Loader) createUniqueHash(raw map[string]interface{}) string {
	// Extract message ID and request ID for deduplication (matches TypeScript's createUniqueHash)
	var messageID, requestID string
	
	// Get message ID from nested message object (required)
	if message, ok := raw["message"].(map[string]interface{}); ok {
		if id, ok := message["id"].(string); ok {
			messageID = id
		}
	}
	
	// Get request ID (required)
	if id, ok := raw["requestId"].(string); ok {
		requestID = id
	}
	
	// TypeScript returns null if either ID is missing
	if messageID == "" || requestID == "" {
		return ""
	}
	
	// Create hash using same format as TypeScript: messageId:requestId
	return messageID + ":" + requestID
}

func (l *Loader) findJSONLFiles(basePath string) ([]string, error) {
	var files []string

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking, ignore inaccessible files
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".jsonl") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// findJSONLFilesWithFilter finds JSONL files with optional time-based filtering
func (l *Loader) findJSONLFilesWithFilter(basePath string, options *LoaderOptions) ([]string, error) {
	var files []string
	cutoffTime := time.Now().Add(-options.ModifiedWithin)
	
	// Two-phase scanning for better performance
	// Phase 1: Find all project directories
	projectDirs, err := l.findProjectDirectories(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find project directories: %w", err)
	}
	
	if l.debug {
		fmt.Fprintf(os.Stderr, "Debug: Found %d project directories\n", len(projectDirs))
	}
	
	// Phase 2: Filter projects and collect JSONL files
	for _, projectDir := range projectDirs {
		// Quick check if project has recent activity
		if options.ModifiedWithin > 0 {
			if shouldSkip := l.shouldSkipProject(projectDir, cutoffTime); shouldSkip {
				if l.debug {
					fmt.Fprintf(os.Stderr, "Debug: Skipping inactive project: %s\n", filepath.Base(projectDir))
				}
				continue
			}
		}
		
		// Collect JSONL files from active project
		projectFiles, err := l.collectProjectFiles(projectDir, cutoffTime, options.ModifiedWithin > 0)
		if err != nil {
			if l.debug {
				fmt.Fprintf(os.Stderr, "Debug: Error reading project %s: %v\n", filepath.Base(projectDir), err)
			}
			continue
		}
		
		files = append(files, projectFiles...)
		
		if l.debug && len(projectFiles) > 0 {
			fmt.Fprintf(os.Stderr, "Debug: Project %s has %d recent files\n", 
				filepath.Base(projectDir), len(projectFiles))
		}
	}
	
	return files, nil
}

// findProjectDirectories finds all project directories under the base path
func (l *Loader) findProjectDirectories(basePath string) ([]string, error) {
	var projectDirs []string
	
	// Read the projects directory
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	
	// Collect all subdirectories (these are project directories in flat structure)
	for _, entry := range entries {
		if entry.IsDir() {
			projectPath := filepath.Join(basePath, entry.Name())
			projectDirs = append(projectDirs, projectPath)
		}
	}
	
	return projectDirs, nil
}

// shouldSkipProject checks if a project directory should be skipped based on activity.
// Also peeks into the subagents/ subdirectory because sub-agent JSONL files
// can be the only recent activity for a project.
func (l *Loader) shouldSkipProject(projectPath string, cutoffTime time.Time) bool {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return true // Skip on error
	}

	var latestModTime time.Time
	hasJSONL := false

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			hasJSONL = true
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestModTime) {
				latestModTime = info.ModTime()
			}
			if latestModTime.After(cutoffTime) {
				return false
			}
		}
	}

	// Also inspect subagents/ subdirectory
	subagentsDir := filepath.Join(projectPath, "subagents")
	if subEntries, err := os.ReadDir(subagentsDir); err == nil {
		for _, entry := range subEntries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
				continue
			}
			hasJSONL = true
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestModTime) {
				latestModTime = info.ModTime()
			}
			if latestModTime.After(cutoffTime) {
				return false
			}
		}
	}

	return !hasJSONL || latestModTime.Before(cutoffTime)
}

// collectProjectFiles collects JSONL files from a project directory.
// Also recurses one level into the subagents/ subdirectory, which holds
// sub-agent session JSONL files that contribute to the project's cost.
func (l *Loader) collectProjectFiles(projectPath string, cutoffTime time.Time, applyTimeFilter bool) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "subagents" {
				subFiles, err := l.collectSubagentFiles(filepath.Join(projectPath, entry.Name()), cutoffTime, applyTimeFilter)
				if err == nil {
					files = append(files, subFiles...)
				}
			}
			continue
		}

		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			continue
		}

		filePath := filepath.Join(projectPath, entry.Name())

		if applyTimeFilter {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffTime) {
				continue
			}
		}

		files = append(files, filePath)
	}

	return files, nil
}

// collectSubagentFiles lists JSONL files inside a subagents/ directory (one level only).
func (l *Loader) collectSubagentFiles(subagentsDir string, cutoffTime time.Time, applyTimeFilter bool) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(subagentsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			continue
		}

		filePath := filepath.Join(subagentsDir, entry.Name())
		if applyTimeFilter {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffTime) {
				continue
			}
		}
		files = append(files, filePath)
	}

	return files, nil
}

// isProjectDir checks if a directory is a project directory (not used in new implementation)
func isProjectDir(path string) bool {
	// This function is kept for backward compatibility but not used in optimized version
	// Check if path contains "projects" and is a direct child
	if !strings.Contains(path, "/projects/") {
		return false
	}
	
	// Split by /projects/ and check structure
	parts := strings.Split(path, "/projects/")
	if len(parts) < 2 {
		return false
	}
	
	// Project directories are direct children of projects/
	afterProjects := parts[1]
	slashCount := strings.Count(afterProjects, "/")
	return slashCount == 0
}

// sortFilesByModTime sorts files by modification time (newest first)
func (l *Loader) sortFilesByModTime(files []string) ([]string, error) {
	type fileWithModTime struct {
		path    string
		modTime time.Time
	}
	
	filesWithTime := make([]fileWithModTime, len(files))
	for i, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			// If we can't get file info, use zero time
			filesWithTime[i] = fileWithModTime{path: file, modTime: time.Time{}}
		} else {
			filesWithTime[i] = fileWithModTime{path: file, modTime: info.ModTime()}
		}
	}
	
	// Sort by modification time (newest first)
	sort.Slice(filesWithTime, func(i, j int) bool {
		return filesWithTime[i].modTime.After(filesWithTime[j].modTime)
	})
	
	// Extract sorted file paths
	result := make([]string, len(filesWithTime))
	for i, item := range filesWithTime {
		result[i] = item.path
	}
	
	return result, nil
}

type fileWithTimestamp struct {
	path      string
	timestamp *time.Time
}

func (l *Loader) sortFilesByTimestamp(files []string) ([]string, error) {
	filesWithTimestamps := make([]fileWithTimestamp, len(files))
	
	// Get earliest timestamp for each file
	for i, file := range files {
		timestamp, err := l.getEarliestTimestamp(file)
		if err != nil {
			// If we can't get timestamp, still include the file
			filesWithTimestamps[i] = fileWithTimestamp{path: file, timestamp: nil}
		} else {
			filesWithTimestamps[i] = fileWithTimestamp{path: file, timestamp: &timestamp}
		}
	}
	
	// Sort by timestamp (files without timestamp go last)
	sort.Slice(filesWithTimestamps, func(i, j int) bool {
		a, b := filesWithTimestamps[i], filesWithTimestamps[j]
		
		// Files without timestamp go to the end
		if a.timestamp == nil && b.timestamp == nil {
			return false
		}
		if a.timestamp == nil {
			return false
		}
		if b.timestamp == nil {
			return true
		}
		
		// Sort by timestamp (earliest first)
		return a.timestamp.Before(*b.timestamp)
	})
	
	// Extract sorted file paths
	result := make([]string, len(filesWithTimestamps))
	for i, item := range filesWithTimestamps {
		result[i] = item.path
	}
	
	return result, nil
}

func (l *Loader) getEarliestTimestamp(filePath string) (time.Time, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	var earliestTime time.Time
	
	// Scan first few lines to find earliest timestamp
	lineCount := 0
	for scanner.Scan() && lineCount < 100 { // Only check first 100 lines for performance
		lineCount++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		
		// Try to parse timestamp
		if ts, ok := raw["timestamp"].(string); ok {
			if parsedTime, err := time.Parse(time.RFC3339, ts); err == nil {
				if earliestTime.IsZero() || parsedTime.Before(earliestTime) {
					earliestTime = parsedTime
				}
			}
		}
	}
	
	if earliestTime.IsZero() {
		return time.Time{}, fmt.Errorf("no valid timestamp found in file")
	}
	
	return earliestTime, nil
}

// validateUsageData validates entry according to TypeScript usageDataSchema
func (l *Loader) validateUsageData(raw map[string]interface{}, entry *types.UsageEntry) error {
	// timestamp is required (already validated in parseEntry)
	
	// message object is required
	message, ok := raw["message"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing required message object")
	}
	
	// message.usage is required
	usage, ok := message["usage"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing required message.usage object")
	}
	
	// input_tokens is required (must be number, can be 0)
	inputTokens, hasInput := usage["input_tokens"]
	if !hasInput {
		return fmt.Errorf("missing required input_tokens")
	}
	if inputFloat, ok := inputTokens.(float64); ok {
		entry.InputTokens = int(inputFloat)
	} else {
		return fmt.Errorf("input_tokens must be a number")
	}
	
	// output_tokens is required (must be number, can be 0)
	outputTokens, hasOutput := usage["output_tokens"]
	if !hasOutput {
		return fmt.Errorf("missing required output_tokens")
	}
	if outputFloat, ok := outputTokens.(float64); ok {
		entry.OutputTokens = int(outputFloat)
	} else {
		return fmt.Errorf("output_tokens must be a number")
	}
	
	// Optional fields
	if model, ok := message["model"].(string); ok {
		entry.Model = model
	}
	
	// cache_creation_input_tokens — flat takes precedence; fall back to
	// nested usage.cache_creation.{ephemeral_1h,ephemeral_5m}_input_tokens
	// (Anthropic 2026 schema).
	if cacheCreate, ok := usage["cache_creation_input_tokens"].(float64); ok {
		entry.CacheCreationInputTokens = int(cacheCreate)
	} else if nested, ok := usage["cache_creation"].(map[string]interface{}); ok {
		var sum int
		if v, ok := nested["ephemeral_1h_input_tokens"].(float64); ok {
			sum += int(v)
		}
		if v, ok := nested["ephemeral_5m_input_tokens"].(float64); ok {
			sum += int(v)
		}
		entry.CacheCreationInputTokens = sum
	}

	// cache_read_input_tokens is optional
	if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
		entry.CacheReadInputTokens = int(cacheRead)
	}

	// server_tool_use billing — Anthropic web tools (web_search / web_fetch).
	if stu, ok := usage["server_tool_use"].(map[string]interface{}); ok {
		if v, ok := stu["web_search_requests"].(float64); ok {
			entry.WebSearchRequests = int(v)
		}
		if v, ok := stu["web_fetch_requests"].(float64); ok {
			entry.WebFetchRequests = int(v)
		}
	}

	// diagnostics.cache_miss_reason — optional Anthropic diagnostic payload.
	if diag, ok := message["diagnostics"].(map[string]interface{}); ok {
		if reason, ok := diag["cache_miss_reason"].(map[string]interface{}); ok {
			cmr := &types.CacheMissReason{}
			if v, ok := reason["type"].(string); ok {
				cmr.Type = v
			}
			if v, ok := reason["cache_missed_input_tokens"].(float64); ok {
				cmr.CacheMissedInputTokens = int(v)
			}
			if cmr.Type != "" || cmr.CacheMissedInputTokens != 0 {
				entry.CacheMissReason = cmr
			}
		}
	}
	
	// costUSD is optional
	if cost, ok := raw["costUSD"].(float64); ok {
		entry.Cost = cost
	} else if cost, ok := raw["cost"].(float64); ok {
		entry.Cost = cost
	}
	
	// sessionId is optional (various field names)
	if sessionID, ok := raw["sessionId"].(string); ok {
		entry.SessionID = sessionID
	} else if sessionID, ok := raw["session_id"].(string); ok {
		entry.SessionID = sessionID
	}
	
	return nil
}

// extractProjectPath returns the project directory portion of a JSONL file path.
// Modern ~/.claude/projects/ uses a flat layout (project/file.jsonl or
// project/subagents/file.jsonl); the legacy YYYY/MM/DD nesting is gone.
func (l *Loader) extractProjectPath(filePath string) string {
	dir := filepath.Dir(filePath)
	parts := strings.Split(dir, string(os.PathSeparator))

	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return strings.Join(parts[:i+2], string(os.PathSeparator))
		}
	}

	// Fallback: trim trailing "subagents" if present so that subagent files
	// still report the parent project path.
	if len(parts) > 0 && parts[len(parts)-1] == "subagents" {
		return strings.Join(parts[:len(parts)-1], string(os.PathSeparator))
	}
	return dir
}

// calculateTotalTokens matches TypeScript's getTotalTokens function.
// Reads first-class cache token fields populated by validateUsageData.
func (l *Loader) calculateTotalTokens(entry *types.UsageEntry) {
	entry.TotalTokens = entry.InputTokens + entry.OutputTokens +
		entry.CacheCreationInputTokens + entry.CacheReadInputTokens
}

// nonUsageEntryTypes lists JSONL entry types that legitimately have no
// message.usage payload. They must not be counted as parse errors when
// validateUsageData rejects them.
var nonUsageEntryTypes = map[string]bool{
	"user":                  true,
	"summary":               true,
	"attachment":            true,
	"system":                true,
	"last-prompt":           true,
	"file-history-snapshot": true,
	"permission-mode":       true,
	"agent-setting":         true,
	"queue-operation":       true,
	"ai-title":              true,
}

// shouldCountAsParseError determines if an error should be counted as parse error
func (l *Loader) shouldCountAsParseError(err error, raw map[string]interface{}) bool {
	errMsg := err.Error()

	if strings.Contains(errMsg, "missing required message.usage object") ||
		strings.Contains(errMsg, "missing required message object") {
		if typeStr, ok := raw["type"].(string); ok {
			if nonUsageEntryTypes[typeStr] {
				return false
			}
		}
	}

	return true
}
