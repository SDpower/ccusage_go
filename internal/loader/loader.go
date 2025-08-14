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

type Loader struct {
	maxWorkers int
	debug      bool
	timezone   *time.Location
}

func New() *Loader {
	return &Loader{
		maxWorkers: 10,
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

func (l *Loader) LoadFromPath(ctx context.Context, path string) ([]types.UsageEntry, error) {
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
	
	paths, err := l.findJSONLFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to find JSONL files: %w", err)
	}

	if l.debug {
		fmt.Fprintf(os.Stderr, "Debug: Found %d JSONL files in %s\n", len(paths), path)
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

	entries, err := l.LoadParallel(ctx, paths)
	
	if l.debug {
		fmt.Fprintf(os.Stderr, "Debug: Loaded %d usage entries\n", len(entries))
		
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

func (l *Loader) loadFileWithDedupe(path string, dedupeMap map[string]bool, dedupeMutex ...*sync.Mutex) ([]types.UsageEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, types.LoaderError{Path: path, Err: err}
	}
	defer file.Close()

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
		entry, err := l.parseEntry(raw)
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

func (l *Loader) parseEntry(raw map[string]interface{}) (types.UsageEntry, error) {
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

	if projectPath, ok := raw["project_path"].(string); ok {
		entry.ProjectPath = projectPath
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
