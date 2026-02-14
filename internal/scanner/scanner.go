package scanner

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var supportedExtensions = map[string]bool{
	".go":     true,
	".py":     true,
	".js":     true,
	".ts":     true,
	".jsx":    true,
	".tsx":    true,
	".java":   true,
	".rb":     true,
	".sql":    true,
	".rs":     true,
	".prisma": true,
}

var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".git":         true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"dist":         true,
	"build":        true,
	"bin":          true,
}

// Scan walks a code repository and extracts SQL table references.
func Scan(repoPath string) (ScanResult, error) {
	result := ScanResult{RepoPath: repoPath}

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
			result.FilesSkipped++
			return nil
		}

		relPath, _ := filepath.Rel(repoPath, path)
		refs, err := scanFile(path, relPath)
		if err != nil {
			return fmt.Errorf("scan %s: %w", relPath, err)
		}

		result.Refs = append(result.Refs, refs...)
		result.FilesScanned++
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	result.Tables = uniqueTables(result.Refs)
	return result, nil
}

func scanFile(path, relPath string) ([]TableRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var refs []TableRef
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, m := range ScanLine(line) {
			refs = append(refs, TableRef{
				Table:   m.Table,
				Schema:  m.Schema,
				File:    relPath,
				Line:    lineNum,
				Pattern: m.Pattern,
				Context: m.Context,
			})
		}
	}

	return refs, scanner.Err()
}

func uniqueTables(refs []TableRef) []string {
	seen := make(map[string]bool)
	for _, r := range refs {
		seen[strings.ToLower(r.Table)] = true
	}

	tables := make([]string, 0, len(seen))
	for t := range seen {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	return tables
}
