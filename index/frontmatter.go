package index

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Note is a parsed Markdown note ready for indexing.
type Note struct {
	Path        string
	Name        string
	Description string
	Aliases     string // space-joined for FTS indexing
	Headings    string // space-joined heading text
	Body        string
	Private     bool
	Modified    time.Time
}

var frontmatterDelim = []byte("---")

// ParseNote parses frontmatter + body from a Markdown file's bytes.
// It is defensive: real vault notes carry arbitrary frontmatter fields, so
// unknown keys are ignored and missing keys fall back to sensible defaults.
func ParseNote(path string, data []byte, mtime time.Time) Note {
	fmBytes, body := splitFrontmatter(data)

	n := Note{Path: path, Body: strings.TrimSpace(string(body)), Modified: mtime}

	if len(fmBytes) > 0 {
		var m map[string]any
		if err := yaml.Unmarshal(fmBytes, &m); err == nil {
			n.Name = str(m["name"])
			n.Description = str(m["description"])
			n.Aliases = strings.Join(strList(m["aliases"]), " ")
			n.Private = boolOf(m["private"])
			if t, ok := dateOf(m["date"]); ok {
				n.Modified = t
			}
		}
	}

	if n.Name == "" {
		base := filepath.Base(path)
		n.Name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if n.Description == "" {
		n.Description = firstProseLine(n.Body)
	}
	n.Headings = extractHeadings(n.Body)
	return n
}

// splitFrontmatter returns (frontmatterYAML, body). If there is no leading
// `---` block, frontmatter is empty and the whole input is the body.
func splitFrontmatter(data []byte) ([]byte, []byte) {
	trimmed := bytes.TrimLeft(data, "\uFEFF \t\r\n")
	if !bytes.HasPrefix(trimmed, frontmatterDelim) {
		return nil, data
	}
	// Find the closing delimiter on its own line after the opening one.
	rest := trimmed[len(frontmatterDelim):]
	rest = bytes.TrimLeft(rest, "\r\n")
	idx := findClosingDelim(rest)
	if idx < 0 {
		return nil, data
	}
	fm := rest[:idx]
	body := rest[idx:]
	// Drop the closing delimiter line from the body.
	if nl := bytes.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = nil
	}
	return fm, body
}

func findClosingDelim(b []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	offset := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimRight(line, "\r") == "---" {
			return offset
		}
		offset += len(sc.Bytes()) + 1 // +1 for the newline stripped by Scanner
	}
	return -1
}

func extractHeadings(body string) string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#") {
			out = append(out, strings.TrimLeft(line, "# "))
		}
	}
	return strings.Join(out, " ")
}

func firstProseLine(body string) string {
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
			continue
		}
		line = strings.TrimLeft(line, "-*> \t")
		if line != "" {
			if len(line) > 200 {
				line = line[:200]
			}
			return line
		}
	}
	return ""
}

// --- defensive coercion helpers ---

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// dateOf handles a frontmatter `date` that yaml.v3 may decode as a time.Time
// (bare YYYY-MM-DD) or leave as a string.
func dateOf(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if parsed, err := time.Parse(layout, strings.TrimSpace(t)); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func boolOf(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	}
	return false
}

func strList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, e := range t {
			if s := str(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		// tolerate a comma-separated string
		var out []string
		for _, p := range strings.Split(t, ",") {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
