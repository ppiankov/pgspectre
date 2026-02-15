package postgres

import "strings"

// ResolveSchemas normalizes and expands schema filter values.
// Empty input means "all non-system schemas" (no filtering).
// "all" or "*" means the same. Otherwise returns the provided schemas.
func ResolveSchemas(schemas []string) []string {
	if len(schemas) == 0 {
		return nil
	}
	for _, s := range schemas {
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower == "all" || lower == "*" {
			return nil
		}
	}
	result := make([]string, 0, len(schemas))
	for _, s := range schemas {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// FilterSnapshot returns a new snapshot containing only objects in the given schemas.
// If schemas is nil or empty, the original snapshot is returned unmodified.
func FilterSnapshot(snap *Snapshot, schemas []string) *Snapshot {
	if len(schemas) == 0 {
		return snap
	}

	include := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		include[strings.ToLower(s)] = true
	}

	filtered := &Snapshot{}

	for _, t := range snap.Tables {
		if include[strings.ToLower(t.Schema)] {
			filtered.Tables = append(filtered.Tables, t)
		}
	}
	for _, c := range snap.Columns {
		if include[strings.ToLower(c.Schema)] {
			filtered.Columns = append(filtered.Columns, c)
		}
	}
	for _, idx := range snap.Indexes {
		if include[strings.ToLower(idx.Schema)] {
			filtered.Indexes = append(filtered.Indexes, idx)
		}
	}
	for i := range snap.Stats {
		if include[strings.ToLower(snap.Stats[i].Schema)] {
			filtered.Stats = append(filtered.Stats, snap.Stats[i])
		}
	}
	for _, c := range snap.Constraints {
		if include[strings.ToLower(c.Schema)] {
			filtered.Constraints = append(filtered.Constraints, c)
		}
	}

	return filtered
}
