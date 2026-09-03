// Package index builds and queries the FTS5 note index that backs both the
// `grep` command and the UserPromptSubmit hook. Retrieval is fully
// deterministic — no model call is involved.
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AgusRdz/local-mind/config"
	_ "modernc.org/sqlite"
)

// Column weights for bm25 — structural fields outrank incidental body hits.
// Used only for ordering; raw bm25 magnitude is corpus-dependent and not a
// stable confidence measure. Order must match the FTS5 table column order.
const bm25Weights = "10.0, 6.0, 8.0, 4.0, 1.0, 0.0, 0.0"

// Confidence weights: a query term matching a structural field (name/aliases/
// description/headings) is strong evidence; a body-only match is weak.
const (
	structuralHit = 1.0
	bodyHit       = 0.4
)

// Band labels.
const (
	BandBody = "body"
	BandDesc = "desc"
	BandLow  = "low"
)

// Result is one ranked match.
type Result struct {
	Name        string
	Description string
	Path        string
	Body        string
	Private     bool
	Modified    time.Time
	Conf        float64
	Band        string
	rank        float64 // raw bm25 (lower = better); internal tie-break only
}

// coverage scores confidence in 0..1 from how many query terms hit a note's
// structural fields vs. only its body — corpus-independent, unlike raw bm25.
func coverage(terms []string, name, aliases, description, headings, body string) float64 {
	if len(terms) == 0 {
		return 0
	}
	structural := strings.ToLower(name + " " + aliases + " " + description + " " + headings)
	lowerBody := strings.ToLower(body)
	var sum float64
	for _, t := range terms {
		switch {
		case strings.Contains(structural, t):
			sum += structuralHit
		case strings.Contains(lowerBody, t):
			sum += bodyHit
		}
	}
	conf := sum / float64(len(terms))
	if conf > 1 {
		conf = 1
	}
	return conf
}

// Index wraps the SQLite connection.
type Index struct {
	db *sql.DB
}

// Open opens (and creates if needed) the index database at the default path.
func Open() (*Index, error) {
	base, err := config.BaseDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, err
	}
	dbPath, _ := config.DBPath()
	return OpenAt(dbPath)
}

// OpenAt opens (and creates if needed) the index database at an explicit path.
// Used by tests and by callers that manage their own storage location.
func OpenAt(dbPath string) (*Index, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	idx := &Index{db: db}
	if err := idx.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return idx, nil
}

func (i *Index) Close() error { return i.db.Close() }

func (i *Index) ensureSchema() error {
	_, err := i.db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS notes USING fts5(
    name, description, aliases, headings, body,
    path UNINDEXED, private UNINDEXED,
    tokenize = 'porter unicode61'
);
CREATE TABLE IF NOT EXISTS meta (
    path TEXT PRIMARY KEY,
    modified INTEGER NOT NULL
);`)
	return err
}

// Rebuild indexes every note under the configured sources. When incremental is
// false the index is dropped and rebuilt; when true, only files whose mtime
// changed since the last run are re-indexed.
func (i *Index) Rebuild(cfg config.Config, incremental bool) (indexed, skipped int, err error) {
	if !incremental {
		if _, err = i.db.Exec(`DELETE FROM notes; DELETE FROM meta;`); err != nil {
			return 0, 0, err
		}
	}

	known := map[string]int64{}
	if incremental {
		rows, qerr := i.db.Query(`SELECT path, modified FROM meta`)
		if qerr == nil {
			for rows.Next() {
				var p string
				var m int64
				_ = rows.Scan(&p, &m)
				known[p] = m
			}
			rows.Close()
		}
	}

	tx, err := i.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	insNote, err := tx.Prepare(`INSERT INTO notes (name, description, aliases, headings, body, path, private) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, 0, err
	}
	defer insNote.Close()
	delNote, err := tx.Prepare(`DELETE FROM notes WHERE path = ?`)
	if err != nil {
		return 0, 0, err
	}
	defer delNote.Close()
	upMeta, err := tx.Prepare(`INSERT INTO meta (path, modified) VALUES (?, ?) ON CONFLICT(path) DO UPDATE SET modified=excluded.modified`)
	if err != nil {
		return 0, 0, err
	}
	defer upMeta.Close()

	for _, root := range cfg.Sources {
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
			if werr != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			if ignored(path, cfg.Ignore) {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			mtime := info.ModTime().Unix()

			if incremental {
				if prev, ok := known[path]; ok && prev == mtime {
					skipped++
					return nil
				}
				_, _ = delNote.Exec(path)
			}

			data, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			n := ParseNote(path, data, info.ModTime())
			priv := 0
			if n.Private {
				priv = 1
			}
			if _, eerr := insNote.Exec(n.Name, n.Description, n.Aliases, n.Headings, n.Body, n.Path, priv); eerr != nil {
				return eerr
			}
			if _, eerr := upMeta.Exec(path, mtime); eerr != nil {
				return eerr
			}
			indexed++
			return nil
		})
		if walkErr != nil {
			err = walkErr
			return indexed, skipped, err
		}
	}

	err = tx.Commit()
	return indexed, skipped, err
}

// Search runs a deterministic FTS5 match for the query and returns ranked,
// confidence-banded results (best first), capped at limit.
func (i *Index) Search(query string, bands config.Bands, limit int) ([]Result, error) {
	match := buildMatch(query)
	if match == "" {
		return nil, nil
	}
	sqlStr := fmt.Sprintf(`
SELECT name, description, aliases, headings, path, body, private,
       bm25(notes, %s) AS rank
FROM notes
WHERE notes MATCH ?
ORDER BY rank ASC
LIMIT ?`, bm25Weights)

	rows, err := i.db.Query(sqlStr, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	terms := extractTerms(query)
	var out []Result
	for rows.Next() {
		var r Result
		var aliases, headings string
		var priv int
		var rank float64
		if err := rows.Scan(&r.Name, &r.Description, &aliases, &headings, &r.Path, &r.Body, &priv, &rank); err != nil {
			return nil, err
		}
		r.Private = priv == 1
		r.rank = rank
		r.Conf = coverage(terms, r.Name, aliases, r.Description, headings, r.Body)
		r.Band = classify(r.Conf, bands)
		r.Modified = mtimeOf(i, r.Path)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Confidence drives banding; bm25 rank is the tie-break.
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Conf != out[b].Conf {
			return out[a].Conf > out[b].Conf
		}
		return out[a].rank < out[b].rank
	})
	return out, nil
}

func mtimeOf(i *Index, path string) time.Time {
	var m int64
	if err := i.db.QueryRow(`SELECT modified FROM meta WHERE path = ?`, path).Scan(&m); err == nil {
		return time.Unix(m, 0)
	}
	return time.Time{}
}

func classify(conf float64, b config.Bands) string {
	switch {
	case conf >= b.VeryHigh:
		return BandBody
	case conf >= b.High:
		return BandDesc
	default:
		return BandLow
	}
}

var (
	tokenRe   = regexp.MustCompile(`[a-z0-9]+`)
	stopwords = map[string]bool{
		"the": true, "and": true, "for": true, "why": true, "how": true,
		"did": true, "does": true, "was": true, "were": true, "what": true,
		"when": true, "with": true, "from": true, "that": true, "this": true,
		"out": true, "our": true, "are": true, "you": true, "your": true,
		"into": true, "last": true, "month": true, "week": true, "have": true,
		"has": true, "can": true, "not": true, "but": true, "get": true,
	}
)

// extractTerms tokenizes a prompt into distinct significant terms, dropping
// stopwords and very short tokens, capped for latency.
func extractTerms(query string) []string {
	toks := tokenRe.FindAllString(strings.ToLower(query), -1)
	seen := map[string]bool{}
	var kept []string
	for _, t := range toks {
		if len(t) < 3 || stopwords[t] || seen[t] {
			continue
		}
		seen[t] = true
		kept = append(kept, t)
		if len(kept) >= 24 {
			break
		}
	}
	return kept
}

// buildMatch turns a free-text prompt into a safe FTS5 OR query of quoted terms.
func buildMatch(query string) string {
	terms := extractTerms(query)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	return strings.Join(quoted, " OR ")
}

func ignored(path string, globs []string) bool {
	unix := filepath.ToSlash(path)
	for _, g := range globs {
		if matchGlob(g, unix) {
			return true
		}
	}
	return false
}

// matchGlob supports a leading/embedded ** as "any path segments".
func matchGlob(glob, path string) bool {
	if strings.Contains(glob, "**") {
		parts := strings.Split(glob, "**")
		idx := 0
		for pi, p := range parts {
			p = strings.Trim(p, "/")
			if p == "" {
				continue
			}
			at := strings.Index(path[idx:], p)
			if at < 0 {
				return false
			}
			idx += at + len(p)
			_ = pi
		}
		return true
	}
	ok, _ := filepath.Match(glob, filepath.Base(path))
	return ok
}
