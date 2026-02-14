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
		refs, colRefs, err := scanFile(path, relPath)
		if err != nil {
			return fmt.Errorf("scan %s: %w", relPath, err)
		}

		result.Refs = append(result.Refs, refs...)
		result.ColumnRefs = append(result.ColumnRefs, colRefs...)
		result.FilesScanned++
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("walk %s: %w", repoPath, err)
	}

	result.Tables = uniqueTables(result.Refs)
	result.Columns = uniqueColumns(result.ColumnRefs)
	return result, nil
}

func scanFile(path, relPath string) ([]TableRef, []ColumnRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	ext := strings.ToLower(filepath.Ext(path))
	buf := newSQLBuffer()

	var refs []TableRef
	var colRefs []ColumnRef

	scanText := func(text string, line int) {
		for _, m := range ScanLine(text) {
			refs = append(refs, TableRef{
				Table:   m.Table,
				Schema:  m.Schema,
				File:    relPath,
				Line:    line,
				Pattern: m.Pattern,
				Context: m.Context,
			})
		}
		for _, cm := range ScanLineColumns(text) {
			colRefs = append(colRefs, ColumnRef{
				Table:   cm.Table,
				Column:  cm.Column,
				Schema:  cm.Schema,
				File:    relPath,
				Line:    line,
				Context: cm.Context,
			})
		}
	}

	sc := bufio.NewScanner(f)
	lineNum := 0

	if ext == ".sql" {
		for sc.Scan() {
			lineNum++
			for _, s := range buf.feedSQL(lineNum, sc.Text()) {
				scanText(s.text, s.lineNum)
			}
		}
	} else {
		for sc.Scan() {
			lineNum++
			line := sc.Text()

			stmt, buffered := buf.feedCode(lineNum, line, ext)
			if stmt != nil {
				scanText(stmt.text, stmt.lineNum)
			}
			if !buffered {
				scanText(line, lineNum)
			}
		}
	}

	// Flush any remaining buffered content
	if s := buf.flush(); s != nil {
		scanText(s.text, s.lineNum)
	}

	return refs, colRefs, sc.Err()
}

func uniqueColumns(refs []ColumnRef) []string {
	seen := make(map[string]bool)
	for _, r := range refs {
		key := strings.ToLower(r.Table) + "." + strings.ToLower(r.Column)
		seen[key] = true
	}

	cols := make([]string, 0, len(seen))
	for c := range seen {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
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
