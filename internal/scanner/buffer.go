package scanner

import "strings"

// blockKind identifies the type of multi-line block being buffered.
type blockKind int

const (
	blockNone        blockKind = iota
	blockSQL                   // .sql file: accumulate until semicolon
	blockBacktick              // Go/JS/TS: backtick string literal
	blockTripleQuote           // Python/Java: triple-quote string
)

// sqlBuffer accumulates lines that belong to a multi-line SQL construct,
// normalizes them into a single-line string, and yields completed statements.
type sqlBuffer struct {
	kind      blockKind
	lines     []string
	startLine int
}

// bufferedStatement is a completed multi-line SQL string with its origin line.
type bufferedStatement struct {
	text    string
	lineNum int
}

// backtickExts are file extensions that use backtick multi-line strings.
var backtickExts = map[string]bool{
	".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
}

// tripleQuoteExts are file extensions that use triple-quote multi-line strings.
var tripleQuoteExts = map[string]bool{
	".py": true, ".java": true,
}

func newSQLBuffer() *sqlBuffer {
	return &sqlBuffer{}
}

func (b *sqlBuffer) active() bool {
	return b.kind != blockNone
}

func (b *sqlBuffer) reset() {
	b.kind = blockNone
	b.lines = nil
	b.startLine = 0
}

// feedSQL processes a line from a .sql file. Returns completed statements
// when semicolons are encountered.
func (b *sqlBuffer) feedSQL(lineNum int, line string) []bufferedStatement {
	if len(b.lines) == 0 {
		b.startLine = lineNum
		b.kind = blockSQL
	}

	parts := splitOnSemicolons(line)

	if len(parts) == 1 {
		// No semicolon — buffer the line
		b.lines = append(b.lines, line)
		return nil
	}

	var results []bufferedStatement
	for i, part := range parts {
		if i < len(parts)-1 {
			// Part before a semicolon — complete the statement
			b.lines = append(b.lines, part)
			text := normalize(b.lines)
			if text != "" {
				results = append(results, bufferedStatement{
					text:    text,
					lineNum: b.startLine,
				})
			}
			b.lines = nil
			b.startLine = lineNum
		} else {
			// After the last semicolon — start of next statement
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				b.lines = []string{part}
				b.startLine = lineNum
			} else {
				b.lines = nil
				b.kind = blockNone
			}
		}
	}

	return results
}

// feedCode processes a line from a code file. Returns a completed statement
// when a multi-line string block closes, and whether the line was buffered.
func (b *sqlBuffer) feedCode(lineNum int, line, ext string) (*bufferedStatement, bool) {
	// Inside a block — check for closing delimiter
	if b.active() {
		b.lines = append(b.lines, line)

		switch b.kind {
		case blockBacktick:
			if containsBacktick(line) {
				text := normalize(b.lines)
				text = trimAtBacktick(text)
				result := &bufferedStatement{text: text, lineNum: b.startLine}
				b.reset()
				return result, true
			}
		case blockTripleQuote:
			if containsTripleQuote(line) {
				text := normalize(b.lines)
				text = trimAtTripleQuote(text)
				result := &bufferedStatement{text: text, lineNum: b.startLine}
				b.reset()
				return result, true
			}
		}
		return nil, true
	}

	// Not in a block — check if this line opens one
	if backtickExts[ext] && opensBacktickBlock(line) {
		b.kind = blockBacktick
		b.startLine = lineNum
		b.lines = []string{extractAfterBacktick(line)}
		return nil, true
	}

	if tripleQuoteExts[ext] && opensTripleQuoteBlock(line) {
		b.kind = blockTripleQuote
		b.startLine = lineNum
		b.lines = []string{extractAfterTripleQuote(line)}
		return nil, true
	}

	return nil, false
}

// flush returns a statement from any remaining buffered content.
func (b *sqlBuffer) flush() *bufferedStatement {
	if len(b.lines) == 0 {
		return nil
	}
	text := normalize(b.lines)
	lineNum := b.startLine
	b.reset()
	if text == "" {
		return nil
	}
	return &bufferedStatement{text: text, lineNum: lineNum}
}

// normalize joins lines and collapses whitespace to a single space.
func normalize(lines []string) string {
	joined := strings.Join(lines, " ")
	var sb strings.Builder
	sb.Grow(len(joined))
	prevSpace := false
	for _, r := range joined {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				sb.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		sb.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(sb.String())
}

// splitOnSemicolons splits on ';' that are not inside single-quoted strings.
func splitOnSemicolons(line string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case ch == '\'':
			if inQuote && i+1 < len(line) && line[i+1] == '\'' {
				// Escaped single quote ('')
				current.WriteByte(ch)
				current.WriteByte(ch)
				i++
				continue
			}
			inQuote = !inQuote
			current.WriteByte(ch)
		case ch == ';' && !inQuote:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	parts = append(parts, current.String())
	return parts
}

// opensBacktickBlock returns true if the line has an odd number of unescaped
// backticks (meaning one is unclosed).
func opensBacktickBlock(line string) bool {
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			if i > 0 && line[i-1] == '\\' {
				continue
			}
			count++
		}
	}
	return count%2 == 1
}

// containsBacktick returns true if the line has an unescaped backtick.
func containsBacktick(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			if i > 0 && line[i-1] == '\\' {
				continue
			}
			return true
		}
	}
	return false
}

// opensTripleQuoteBlock returns true if the line has an opening triple-quote
// that is not closed on the same line.
func opensTripleQuoteBlock(line string) bool {
	for _, delim := range []string{`"""`, `'''`} {
		idx := strings.Index(line, delim)
		if idx >= 0 {
			rest := line[idx+3:]
			if !strings.Contains(rest, delim) {
				return true
			}
		}
	}
	return false
}

// containsTripleQuote returns true if the line contains """ or ”'.
func containsTripleQuote(line string) bool {
	return strings.Contains(line, `"""`) || strings.Contains(line, `'''`)
}

// extractAfterBacktick returns everything after the first unescaped backtick.
func extractAfterBacktick(line string) string {
	for i := 0; i < len(line); i++ {
		if line[i] == '`' && (i == 0 || line[i-1] != '\\') {
			return line[i+1:]
		}
	}
	return line
}

// extractAfterTripleQuote returns everything after the first """ or ”'.
func extractAfterTripleQuote(line string) string {
	for _, delim := range []string{`"""`, `'''`} {
		if idx := strings.Index(line, delim); idx >= 0 {
			return line[idx+3:]
		}
	}
	return line
}

// trimAtBacktick truncates text at the first unescaped backtick.
func trimAtBacktick(text string) string {
	for i := 0; i < len(text); i++ {
		if text[i] == '`' && (i == 0 || text[i-1] != '\\') {
			return text[:i]
		}
	}
	return text
}

// trimAtTripleQuote truncates text at the first """ or ”'.
func trimAtTripleQuote(text string) string {
	for _, delim := range []string{`"""`, `'''`} {
		if idx := strings.Index(text, delim); idx >= 0 {
			return text[:idx]
		}
	}
	return text
}
