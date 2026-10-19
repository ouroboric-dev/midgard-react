package midgard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The product's first commit carries its own CI. It runs the stack's own commands on the stack's own
// image, so every value that belongs to the stack is read from the stack here, not written down twice.
func TestTemplateCI(t *testing.T) {
	read := func(p ...string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(p...))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	first := func(text, pattern, what string) []string {
		t.Helper()
		m := regexp.MustCompile(pattern).FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("%s: no match for %s", what, pattern)
		}
		return m
	}
	wf := read("templates", "web", ".github", "workflows", "ci.yml")
	cf := read("devcontainer", "Containerfile")
	// one job, named test
	jobs := first(wf, `(?m)^jobs:\n((?:[ ]+.*\n|\n)*)`, "the jobs")[1]
	if names := regexp.MustCompile(`(?m)^  ([A-Za-z0-9_-]+):\s*$`).FindAllStringSubmatch(jobs, -1); len(names) != 1 || names[0][1] != "test" {
		t.Errorf("jobs = %v; the workflow has the one job, test", names)
	}
	if !strings.Contains(wf, "\n  test:\n") {
		t.Error("no line `  test:` under jobs:")
	}

	// every action is pinned by commit SHA, with its version in a comment
	uses := regexp.MustCompile(`(?m)^[\s-]*uses:.*$`).FindAllString(wf, -1)
	if len(uses) == 0 {
		t.Error("the workflow uses no action; it should check out the code")
	}
	pinned := regexp.MustCompile(`uses:\s*\S+@[0-9a-f]{40}\s+# v`)
	for _, u := range uses {
		if !pinned.MatchString(u) {
			t.Errorf("%q is not pinned by a full commit SHA with its version in a comment", strings.TrimSpace(u))
		}
	}

	// the job runs in the image of the stack's web stage, and starts no service
	web := first(cf, `(?m)^FROM (\S+) AS web\s*$`, "the web stage of devcontainer/Containerfile")[1]
	if got := first(wf, `(?m)^    container:\n      image: (?:>-\n\s+)?(\S+)\s*$`, "the job container")[1]; got != web {
		t.Errorf("container image = %s, want the stack's %s", got, web)
	}
	if regexp.MustCompile(`(?m)^    services:`).MatchString(wf) {
		t.Error("the web template's CI has no service")
	}

	// the least privilege, the triggers, the time limit and the steps
	for _, m := range misplaced(wf) {
		t.Errorf("the workflow lacks %s", m)
	}

	// each npm run names a script of the template
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(read("templates", "web", "package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	if _, ok := pkg.Scripts["test"]; !ok {
		t.Error("package.json has no script test, which `npm test` runs")
	}
	// ESLint looks up the nearest config of each folder; the CI's lint reads the one protected config at the top
	// and no other, so a config planted in a subfolder changes nothing (the ladder's own lint ignores them too).
	if got, want := pkg.Scripts["lint"], "eslint --no-config-lookup -c eslint.config.js ."; got != want {
		t.Errorf("the lint script is %q, want %q", got, want)
	}
	runs := regexp.MustCompile(`npm run (\S+)`).FindAllStringSubmatch(wf, -1)
	if len(runs) == 0 {
		t.Error("the workflow runs no npm script")
	}
	for _, m := range runs {
		if _, ok := pkg.Scripts[m[1]]; !ok {
			t.Errorf("`npm run %s` names no script of templates/web/package.json", m[1])
		}
	}
	if strings.Contains(wf, "playwright") {
		t.Error("the product CI does not run Playwright")
	}
}

// ciLines are the lines the product's CI must have, each under its keys.
var ciLines = []struct {
	keys []string
	line string
}{
	{[]string{"on"}, "pull_request:"},
	{[]string{"on"}, "push:"},
	{[]string{"on", "push"}, "branches: [main]"},
	{[]string{"permissions"}, "contents: read"},
	{[]string{"jobs", "test"}, "timeout-minutes: 15"},
	{[]string{"jobs", "test", "steps", "with"}, "persist-credentials: false"},
	{[]string{"jobs", "test", "steps"}, "run: npm ci --ignore-scripts --no-audit --no-fund"},
	{[]string{"jobs", "test", "steps"}, "run: npm run lint"},
	{[]string{"jobs", "test", "steps"}, "run: npm run typecheck"},
	{[]string{"jobs", "test", "steps"}, "run: npm test"},
	{[]string{"jobs", "test", "steps"}, "run: npm run build"},
}

// misplaced names each of ciLines that the workflow wf lacks as a direct child of its keys.
func misplaced(wf string) []string {
	var out []string
	for _, c := range ciLines {
		if !slices.Contains(children(wf, c.keys...), c.line) {
			out = append(out, fmt.Sprintf("the line %q under %s", c.line, strings.Join(c.keys, ".")))
		}
	}
	return out
}

// children returns the direct children of the key path keys in the YAML text, the first key at the top
// level, as whole lines without their indent. A list item's dash counts as indent, so the keys of every item
// of a list are children of the list's key. Blank lines and comments are skipped.
func children(text string, keys ...string) []string {
	type line struct {
		indent int
		text   string
	}
	var lines []line
	for _, l := range strings.Split(text, "\n") {
		s := strings.TrimLeft(l, " ")
		for strings.HasPrefix(s, "- ") {
			s = strings.TrimLeft(s[2:], " ")
		}
		if t := strings.TrimSpace(s); t != "" && !strings.HasPrefix(t, "#") {
			lines = append(lines, line{len(l) - len(s), t})
		}
	}
	top := func(ls []line) int {
		n := -1
		for _, l := range ls {
			if n < 0 || l.indent < n {
				n = l.indent
			}
		}
		return n
	}
	for _, key := range keys {
		n := top(lines)
		var block []line
		for i, l := range lines {
			if l.indent == n && l.text == key+":" {
				for _, m := range lines[i+1:] {
					if m.indent <= n {
						break
					}
					block = append(block, m)
				}
			}
		}
		lines = block
	}
	var out []string
	n := top(lines)
	for _, l := range lines {
		if l.indent == n {
			out = append(out, l.text)
		}
	}
	return out
}

// A line counts only under its keys: `branches: [main]` under pull_request does not limit push, and
// `contents: read` under env grants nothing.
func TestMisplacedFindsALineUnderTheWrongKey(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("templates", "web", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	wf := string(b)
	if got := misplaced(wf); len(got) != 0 {
		t.Fatalf("the template: %v", got)
	}
	for _, c := range []struct{ name, old, new, want string }{
		{"branches under pull_request", "  pull_request:\n  push:\n    branches: [main]\n", "  pull_request:\n    branches: [main]\n  push:\n", "branches: [main]"},
		{"contents: read under a step", "permissions:\n  contents: read\n", "permissions: {}\nenv:\n  contents: read\n", "contents: read"},
		{"the time limit on the container", "    timeout-minutes: 15\n    container:\n", "    container:\n      timeout-minutes: 15\n", "timeout-minutes: 15"},
		{"a step outside the job", "      - name: build\n        run: npm run build\n", "", "run: npm run build"},
	} {
		changed := strings.Replace(wf, c.old, c.new, 1)
		if changed == wf {
			t.Fatalf("%s: the template does not have %q", c.name, c.old)
		}
		if c.new == "" {
			changed += "other:\n  steps:\n      - name: build\n        run: npm run build\n"
		}
		got := misplaced(changed)
		if len(got) != 1 || !strings.Contains(got[0], c.want) {
			t.Errorf("%s: misplaced = %v, want only %q", c.name, got, c.want)
		}
	}
}
