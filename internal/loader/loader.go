package loader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sdpower/ccusage-go/internal/types"
)

type Loader struct {
	maxWorkers int
}

func New() *Loader {
	return &Loader{
		maxWorkers: 10,
	}
}

func (l *Loader) LoadFromPath(ctx context.Context, path string) ([]types.UsageEntry, error) {
	paths, err := l.findJSONLFiles(path)
	if err != nil {
		return nil, fmt.Errorf("failed to find JSONL files: %w", err)
	}

	if len(paths) == 0 {
		return nil, types.ErrDataNotFound
	}

	return l.LoadParallel(ctx, paths)
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
					entries, err := l.loadFile(path)
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
	file, err := os.Open(path)
	if err != nil {
		return nil, types.LoaderError{Path: path, Err: err}
	}
	defer file.Close()

	var entries []types.UsageEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, types.ParseError{Line: lineNum, Err: err}
		}

		entry, err := l.parseEntry(raw)
		if err != nil {
			return nil, types.ParseError{Line: lineNum, Err: err}
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, types.LoaderError{Path: path, Err: err}
	}

	return entries, nil
}

func (l *Loader) parseEntry(raw map[string]interface{}) (types.UsageEntry, error) {
	entry := types.UsageEntry{Raw: raw}

	if id, ok := raw["id"].(string); ok {
		entry.ID = id
	}

	if ts, ok := raw["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			entry.Timestamp = t
		}
	}

	if projectPath, ok := raw["project_path"].(string); ok {
		entry.ProjectPath = projectPath
	}

	if model, ok := raw["model"].(string); ok {
		entry.Model = model
	}

	if inputTokens, ok := raw["input_tokens"].(float64); ok {
		entry.InputTokens = int(inputTokens)
	}

	if outputTokens, ok := raw["output_tokens"].(float64); ok {
		entry.OutputTokens = int(outputTokens)
	}

	if totalTokens, ok := raw["total_tokens"].(float64); ok {
		entry.TotalTokens = int(totalTokens)
	} else {
		entry.TotalTokens = entry.InputTokens + entry.OutputTokens
	}

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

	// Parse cache-related fields
	if cacheCreate, ok := raw["cache_creation_input_tokens"].(float64); ok {
		// Store in Raw for now
		entry.Raw["cache_creation_input_tokens"] = int(cacheCreate)
	}

	if cacheRead, ok := raw["cache_read_input_tokens"].(float64); ok {
		// Store in Raw for now
		entry.Raw["cache_read_input_tokens"] = int(cacheRead)
	}

	return entry, nil
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
