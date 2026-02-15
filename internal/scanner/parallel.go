package scanner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// fileResult holds the scan result for a single file.
type fileResult struct {
	refs     []TableRef
	colRefs  []ColumnRef
	err      error
	filePath string
}

// ScanParallel walks a code repository using N goroutines.
// workers=0 means runtime.NumCPU(). workers=1 is sequential.
func ScanParallel(repoPath string, workers int) (ScanResult, error) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers == 1 {
		return Scan(repoPath)
	}

	// Phase 1: collect file paths
	var paths []string
	skipped := 0

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			skipped++
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return ScanResult{RepoPath: repoPath}, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	// Phase 2: fan out to workers
	pathCh := make(chan string, len(paths))
	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)

	resultCh := make(chan fileResult, len(paths))
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathCh {
				relPath, _ := filepath.Rel(repoPath, path)
				refs, colRefs, err := scanFile(path, relPath)
				resultCh <- fileResult{
					refs:     refs,
					colRefs:  colRefs,
					err:      err,
					filePath: relPath,
				}
			}
		}()
	}

	wg.Wait()
	close(resultCh)

	// Phase 3: merge results
	result := ScanResult{
		RepoPath:     repoPath,
		FilesSkipped: skipped,
	}

	for fr := range resultCh {
		if fr.err != nil {
			return result, fmt.Errorf("scan %s: %w", fr.filePath, fr.err)
		}
		result.Refs = append(result.Refs, fr.refs...)
		result.ColumnRefs = append(result.ColumnRefs, fr.colRefs...)
		result.FilesScanned++
	}

	result.Tables = uniqueTables(result.Refs)
	result.Columns = uniqueColumns(result.ColumnRefs)
	return result, nil
}
