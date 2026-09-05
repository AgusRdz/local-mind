package index

import (
	"regexp"
	"sort"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// normalizeLink trims a raw `[[...]]` capture down to its resolution key:
// strip an optional `|display text`, strip a trailing `#heading` anchor,
// trim whitespace, and lowercase for case-insensitive matching.
func normalizeLink(raw string) string {
	if i := strings.IndexByte(raw, '|'); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	return strings.ToLower(strings.TrimSpace(raw))
}

// DanglingLinks scans every indexed note's body for `[[target]]` references
// and reports the ones that don't resolve to any note's name (frontmatter
// `name:`, or the filename stem when `name:` is absent — ParseNote already
// folds both into the `name` column). Result maps a note's path to its sorted,
// deduplicated list of unresolved targets; a clean vault yields an empty map.
func (i *Index) DanglingLinks() (map[string][]string, error) {
	rows, err := i.db.Query(`SELECT name, path, body FROM notes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type note struct{ name, path, body string }
	var notes []note
	known := map[string]bool{}
	for rows.Next() {
		var n note
		if err := rows.Scan(&n.name, &n.path, &n.body); err != nil {
			return nil, err
		}
		notes = append(notes, n)
		known[strings.ToLower(n.name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	dangling := map[string][]string{}
	for _, n := range notes {
		seen := map[string]bool{}
		self := strings.ToLower(n.name)
		var missing []string
		for _, m := range wikilinkRe.FindAllStringSubmatch(n.body, -1) {
			target := normalizeLink(m[1])
			if target == "" || target == self || known[target] || seen[target] {
				continue
			}
			seen[target] = true
			missing = append(missing, target)
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			dangling[n.path] = missing
		}
	}
	return dangling, nil
}
