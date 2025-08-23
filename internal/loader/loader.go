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
		entries []types.UsageEntry
		err     error
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
					entries, err := l.loadFileWithGlobalDedupe(path, &dedupeMutex, globalDedupeMap)
					
					// Stream processing: calculate costs immediately if enabled
					if options != nil && options.StreamProcessing && options.Calculator != nil && err == nil {
						for i := range entries {
							options.Calculator.CalculateCost(&entries[i])
							// Clear most Raw data after cost calculation to save memory
							// Keep only cache token fields that are needed for aggregation
							if entries[i].Raw != nil {
								cacheData := make(map[string]interface{})
								if cc, exists := entries[i].Raw["cache_creation_input_tokens"]; exists {
									cacheData["cache_creation_input_tokens"] = cc
								}
								if cr, exists := entries[i].Raw["cache_read_input_tokens"]; exists {
									cacheData["cache_read_input_tokens"] = cr
								}
								if resetTime, exists := entries[i].Raw["usage_limit_reset_time"]; exists {
									cacheData["usage_limit_reset_time"] = resetTime
								}
								if len(cacheData) > 0 {
									entries[i].Raw = cacheData
								} else {
									entries[i].Raw = nil
								}
							}
						}
					}
					
					results <- result{entries: entries, err: err}
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

	for res := range results {
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			allEntries = append(allEntries, res.entries...)
		}
	}

	if len(errors) > 0 && len(allEntries) == 0 {
		return nil, fmt.Errorf("failed to load any files: %v", errors[0])
	}

	return allEntries, nil
}

func (l *Loader) loadFile(path string) ([]types.UsageEntry, error) {
	// Legacy function - redirect to new version with local dedupe
	dedupeMap := make(map[string]bool)
	return l.loadFileWithDedupe(path, dedupeMap)
}

func (l *Loader) loadFileWithGlobalDedupe(path string, dedupeMutex *sync.Mutex, globalDedupeMap map[string]bool) ([]types.UsageEntry, error) {
	return l.loadFileWithDedupe(path, globalDedupeMap, dedupeMutex)
}

// clearRawData removes Raw data from entries to save memory
func clearRawData(entries []types.UsageEntry) {
	for i := range entries {
		entries[i].Raw = nil
	}
}

func (l *Loader) loadFileWithDedupe(path string, dedupeMap map[string]bool, dedupeMutex ...*sync.Mutex) ([]types.UsageEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, types.LoaderError{Path: path, Err: err}
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

		// Try to parse entry according to TypeScript schema rules
		entry, err := l.parseEntry(raw, projectPath)
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

		// For stream processing, we can clear most of Raw data after parsing
		// Keep only cache token fields if they exist
		if entry.Raw != nil {
			cacheData := make(map[string]interface{})
			if cc, ok := entry.Raw["cache_creation_input_tokens"]; ok {
				cacheData["cache_creation_input_tokens"] = cc
			}
			if cr, ok := entry.Raw["cache_read_input_tokens"]; ok {
				cacheData["cache_read_input_tokens"] = cr
			}
			if len(cacheData) > 0 {
				entry.Raw = cacheData
			} else {
				entry.Raw = nil
			}
		}
		
		entries = append(entries, entry)
	}

	if l.debug && parseErrors > 0 {
		fmt.Fprintf(os.Stderr, "Debug: File %s had %d parse errors\n", filepath.Base(path), parseErrors)
		if firstError != "" {
			fmt.Fprintf(os.Stderr, "  First error: %s\n", firstError)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, types.LoaderError{Path: path, Err: err}
	}

	return entries, nil
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
	
	// Calculate total tokens (getTotalTokens function equivalent)
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

	// Parse cache-related fields (for flat structure)
	if cacheCreate, ok := raw["cache_creation_input_tokens"].(float64); ok {
		if entry.Raw == nil {
			entry.Raw = make(map[string]interface{})
		}
		entry.Raw["cache_creation_input_tokens"] = int(cacheCreate)
	}

	if cacheRead, ok := raw["cache_read_input_tokens"].(float64); ok {
		if entry.Raw == nil {
			entry.Raw = make(map[string]interface{})
		}
		entry.Raw["cache_read_input_tokens"] = int(cacheRead)
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

// shouldSkipProject checks if a project directory should be skipped based on activity
func (l *Loader) shouldSkipProject(projectPath string, cutoffTime time.Time) bool {
	// Check the most recent file modification time in the project
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
			// Early exit if we find a recent file
			if latestModTime.After(cutoffTime) {
				return false // Don't skip, has recent activity
			}
		}
	}
	
	// Skip if no JSONL files or all files are old
	return !hasJSONL || latestModTime.Before(cutoffTime)
}

// collectProjectFiles collects JSONL files from a project directory
func (l *Loader) collectProjectFiles(projectPath string, cutoffTime time.Time, applyTimeFilter bool) ([]string, error) {
	var files []string
	
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil, err
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories in flat structure
		}
		
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			continue // Skip non-JSONL files
		}
		
		filePath := filepath.Join(projectPath, entry.Name())
		
		// Apply time filter if enabled
		if applyTimeFilter {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoffTime) {
				continue // Skip old files
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
	
	// cache_creation_input_tokens is optional
	if cacheCreate, ok := usage["cache_creation_input_tokens"].(float64); ok {
		if entry.Raw == nil {
			entry.Raw = make(map[string]interface{})
		}
		entry.Raw["cache_creation_input_tokens"] = int(cacheCreate)
	}
	
	// cache_read_input_tokens is optional
	if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
		if entry.Raw == nil {
			entry.Raw = make(map[string]interface{})
		}
		entry.Raw["cache_read_input_tokens"] = int(cacheRead)
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

func (l *Loader) extractProjectPath(filePath string) string {
	// Extract project path from file path
	// File path format: /path/to/claude/projects/project-name/YYYY/MM/DD/file.jsonl
	// We want to return the full path including project-name
	
	// Remove the filename first
	dir := filepath.Dir(filePath)
	parts := strings.Split(dir, string(os.PathSeparator))
	
	// Find "projects" directory and include everything up to and including the project
	for i := 0; i < len(parts); i++ {
		if parts[i] == "projects" && i+1 < len(parts) {
			// Check if the structure after projects looks like project/YYYY/MM/DD
			// If so, we want to include the project directory
			if i+4 < len(parts) {
				// Check if parts[i+2], parts[i+3], parts[i+4] look like YYYY/MM/DD
				possibleYear := parts[i+2]
				possibleMonth := parts[i+3]
				possibleDay := parts[i+4]
				
				if isNumeric(possibleYear) && len(possibleYear) == 4 &&
				   isNumeric(possibleMonth) && len(possibleMonth) <= 2 &&
				   isNumeric(possibleDay) && len(possibleDay) <= 2 {
					// This looks like the expected structure
					// Return path up to and including the project directory
					projectPath := strings.Join(parts[:i+2], string(os.PathSeparator))
					return projectPath
				}
			}
			// Otherwise just return up to the project directory
			projectPath := strings.Join(parts[:i+2], string(os.PathSeparator))
			return projectPath
		}
	}
	
	// If no "projects" directory, look for common project patterns
	// Remove date structure from the end if present (YYYY/MM/DD)
	if len(parts) >= 3 {
		// Check last 3 parts for date pattern
		possibleYear := parts[len(parts)-3]
		possibleMonth := parts[len(parts)-2]
		possibleDay := parts[len(parts)-1]
		
		if isNumeric(possibleYear) && len(possibleYear) == 4 &&
		   isNumeric(possibleMonth) && len(possibleMonth) <= 2 &&
		   isNumeric(possibleDay) && len(possibleDay) <= 2 {
			// Remove date parts to get project directory
			projectPath := strings.Join(parts[:len(parts)-3], string(os.PathSeparator))
			return projectPath
		}
	}
	
	// Fallback: return the directory path as is
	return dir
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// calculateTotalTokens matches TypeScript's getTotalTokens function
func (l *Loader) calculateTotalTokens(entry *types.UsageEntry) {
	total := entry.InputTokens + entry.OutputTokens
	
	// Add cache tokens if present
	if entry.Raw != nil {
		if cc, ok := entry.Raw["cache_creation_input_tokens"].(int); ok {
			total += cc
		}
		if cr, ok := entry.Raw["cache_read_input_tokens"].(int); ok {
			total += cr
		}
	}
	
	entry.TotalTokens = total
}

// shouldCountAsParseError determines if an error should be counted as parse error
func (l *Loader) shouldCountAsParseError(err error, raw map[string]interface{}) bool {
	errMsg := err.Error()
	
	// Don't count as parse error if it's just missing usage data for non-assistant types
	if strings.Contains(errMsg, "missing required message.usage object") {
		// Check if this might be a user or summary type that legitimately doesn't have usage
		if typeStr, ok := raw["type"].(string); ok {
			if typeStr == "user" || typeStr == "summary" {
				return false // These types legitimately don't have usage data
			}
		}
	}
	
	// Don't count as parse error if it's missing message object entirely (like summary entries)
	if strings.Contains(errMsg, "missing required message object") {
		if typeStr, ok := raw["type"].(string); ok {
			if typeStr == "summary" {
				return false
			}
		}
	}
	
	// All other errors should be counted
	return true
}
