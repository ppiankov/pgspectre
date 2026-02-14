package scanner

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"single line", []string{"SELECT * FROM users"}, "SELECT * FROM users"},
		{"multi line", []string{"SELECT", "  name,", "  email", "FROM users"}, "SELECT name, email FROM users"},
		{"tabs", []string{"SELECT\t*", "\tFROM\tusers"}, "SELECT * FROM users"},
		{"empty lines", []string{"", "  ", ""}, ""},
		{"collapse spaces", []string{"SELECT   *   FROM   users"}, "SELECT * FROM users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.lines)
			if got != tt.want {
				t.Errorf("normalize(%v) = %q, want %q", tt.lines, got, tt.want)
			}
		})
	}
}

func TestSplitOnSemicolons(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int // number of parts
	}{
		{"no semicolon", "SELECT 1", 1},
		{"one semicolon", "SELECT 1; SELECT 2", 2},
		{"two semicolons", "a; b; c", 3},
		{"trailing semicolon", "SELECT 1;", 2},
		{"in quotes", "INSERT INTO t VALUES ('a;b')", 1},
		{"escaped quote", "INSERT INTO t VALUES ('it''s;ok');", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitOnSemicolons(tt.line)
			if len(got) != tt.want {
				t.Errorf("splitOnSemicolons(%q) = %d parts, want %d: %v", tt.line, len(got), tt.want, got)
			}
		})
	}
}

func TestFeedSQL_SingleLine(t *testing.T) {
	buf := newSQLBuffer()
	stmts := buf.feedSQL(1, "SELECT * FROM users;")
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].text != "SELECT * FROM users" {
		t.Errorf("text = %q", stmts[0].text)
	}
	if stmts[0].lineNum != 1 {
		t.Errorf("lineNum = %d, want 1", stmts[0].lineNum)
	}
}

func TestFeedSQL_MultiLine(t *testing.T) {
	buf := newSQLBuffer()

	if stmts := buf.feedSQL(1, "SELECT"); len(stmts) != 0 {
		t.Fatalf("expected 0 statements after line 1, got %d", len(stmts))
	}
	if stmts := buf.feedSQL(2, "  name, email"); len(stmts) != 0 {
		t.Fatalf("expected 0 statements after line 2, got %d", len(stmts))
	}
	stmts := buf.feedSQL(3, "FROM users;")
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if stmts[0].lineNum != 1 {
		t.Errorf("lineNum = %d, want 1", stmts[0].lineNum)
	}
	// Should contain all parts normalized
	if want := "SELECT name, email FROM users"; stmts[0].text != want {
		t.Errorf("text = %q, want %q", stmts[0].text, want)
	}
}

func TestFeedSQL_MultiplePerLine(t *testing.T) {
	buf := newSQLBuffer()
	stmts := buf.feedSQL(1, "DROP TABLE foo; CREATE TABLE bar (id INT);")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(stmts))
	}
	if stmts[0].text != "DROP TABLE foo" {
		t.Errorf("stmt[0] = %q", stmts[0].text)
	}
	if stmts[1].text != "CREATE TABLE bar (id INT)" {
		t.Errorf("stmt[1] = %q", stmts[1].text)
	}
}

func TestFeedSQL_NoTrailingSemicolon(t *testing.T) {
	buf := newSQLBuffer()
	stmts := buf.feedSQL(1, "SELECT * FROM users")
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}
	if !buf.active() {
		t.Error("buffer should be active")
	}

	// Flush should return the buffered content
	flushed := buf.flush()
	if flushed == nil {
		t.Fatal("expected flushed statement")
	}
	if flushed.text != "SELECT * FROM users" {
		t.Errorf("flushed text = %q", flushed.text)
	}
}

func TestFeedSQL_EmptyStatement(t *testing.T) {
	buf := newSQLBuffer()
	stmts := buf.feedSQL(1, ";")
	if len(stmts) != 0 {
		t.Errorf("expected 0 non-empty statements, got %d", len(stmts))
	}
}

func TestFeedCode_BacktickSingleLine(t *testing.T) {
	buf := newSQLBuffer()
	stmt, buffered := buf.feedCode(1, "query := `SELECT * FROM users`", ".go")
	if buffered {
		t.Error("single-line backtick should not be buffered")
	}
	if stmt != nil {
		t.Error("single-line backtick should not produce a statement")
	}
}

func TestFeedCode_BacktickMultiLine(t *testing.T) {
	buf := newSQLBuffer()

	stmt, buffered := buf.feedCode(1, "query := `SELECT", ".go")
	if !buffered {
		t.Error("opening backtick should be buffered")
	}
	if stmt != nil {
		t.Error("should not produce statement on open")
	}

	_, buffered = buf.feedCode(2, "  name, email", ".go")
	if !buffered {
		t.Error("continuation should be buffered")
	}

	stmt, buffered = buf.feedCode(3, "FROM users`", ".go")
	if !buffered {
		t.Error("closing line should be buffered")
	}
	if stmt == nil {
		t.Fatal("expected statement on close")
	}
	if stmt.lineNum != 1 {
		t.Errorf("lineNum = %d, want 1", stmt.lineNum)
	}
	// The extracted text should be the content between backticks
	want := "SELECT name, email FROM users"
	if stmt.text != want {
		t.Errorf("text = %q, want %q", stmt.text, want)
	}
}

func TestFeedCode_BacktickJS(t *testing.T) {
	buf := newSQLBuffer()

	buf.feedCode(1, "const q = `SELECT", ".ts")
	stmt, _ := buf.feedCode(2, "FROM orders`", ".ts")
	if stmt == nil {
		t.Fatal("expected statement")
	}
	if stmt.text != "SELECT FROM orders" {
		t.Errorf("text = %q", stmt.text)
	}
}

func TestFeedCode_BacktickNotInPython(t *testing.T) {
	buf := newSQLBuffer()
	_, buffered := buf.feedCode(1, "x = `something`", ".py")
	if buffered {
		t.Error("backtick should not be recognized in .py files")
	}
}

func TestFeedCode_TripleQuoteMultiLine(t *testing.T) {
	buf := newSQLBuffer()

	stmt, buffered := buf.feedCode(1, `query = """SELECT`, ".py")
	if !buffered {
		t.Error("opening triple-quote should be buffered")
	}
	if stmt != nil {
		t.Error("should not produce statement on open")
	}

	_, buffered = buf.feedCode(2, "  name", ".py")
	if !buffered {
		t.Error("continuation should be buffered")
	}

	stmt, buffered = buf.feedCode(3, `FROM users"""`, ".py")
	if !buffered {
		t.Error("closing line should be buffered")
	}
	if stmt == nil {
		t.Fatal("expected statement on close")
	}
	if stmt.lineNum != 1 {
		t.Errorf("lineNum = %d, want 1", stmt.lineNum)
	}
	want := "SELECT name FROM users"
	if stmt.text != want {
		t.Errorf("text = %q, want %q", stmt.text, want)
	}
}

func TestFeedCode_TripleQuoteSingleLine(t *testing.T) {
	buf := newSQLBuffer()
	_, buffered := buf.feedCode(1, `x = """SELECT * FROM users"""`, ".py")
	if buffered {
		t.Error("single-line triple-quote should not be buffered")
	}
}

func TestFeedCode_TripleQuoteNotInGo(t *testing.T) {
	buf := newSQLBuffer()
	_, buffered := buf.feedCode(1, `x = """something`, ".go")
	if buffered {
		t.Error("triple-quote should not be recognized in .go files")
	}
}

func TestFeedCode_SingleQuoteTriple(t *testing.T) {
	buf := newSQLBuffer()

	buf.feedCode(1, "query = '''SELECT", ".py")
	stmt, _ := buf.feedCode(2, "FROM users'''", ".py")
	if stmt == nil {
		t.Fatal("expected statement")
	}
	if stmt.text != "SELECT FROM users" {
		t.Errorf("text = %q", stmt.text)
	}
}

func TestFeedCode_UnsupportedExt(t *testing.T) {
	buf := newSQLBuffer()
	_, buffered := buf.feedCode(1, "query = `SELECT", ".rb")
	if buffered {
		t.Error("should not buffer for unsupported extension")
	}
}

func TestFlush_Empty(t *testing.T) {
	buf := newSQLBuffer()
	if buf.flush() != nil {
		t.Error("empty flush should return nil")
	}
}

func TestOpensBacktickBlock(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"`SELECT * FROM users`", false}, // even count
		{"query := `SELECT", true},       // odd count
		{"no backticks here", false},     // zero count
		{"a ` b ` c `", true},            // odd count (3)
		{"escaped \\` backtick", false},  // escaped doesn't count
	}
	for _, tt := range tests {
		got := opensBacktickBlock(tt.line)
		if got != tt.want {
			t.Errorf("opensBacktickBlock(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestOpensTripleQuoteBlock(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{`"""SELECT * FROM users"""`, false}, // open+close same line
		{`query = """SELECT`, true},          // open only
		{`no quotes`, false},
		{`query = '''SELECT`, true}, // single-quote triple
	}
	for _, tt := range tests {
		got := opensTripleQuoteBlock(tt.line)
		if got != tt.want {
			t.Errorf("opensTripleQuoteBlock(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
