package midgard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
	// whole lines, so `X: app` does not match `X: application`
	hasLine := func(text, line string) bool {
		return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(line) + `\s*$`).MatchString(text)
	}

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

	// the least privilege, the triggers and the time limit
	for _, want := range []string{"contents: read", "persist-credentials: false", "timeout-minutes: 15", "pull_request:", "push:", "branches: [main]"} {
		if !hasLine(wf, want) {
			t.Errorf("the workflow lacks the line %q", want)
		}
	}

	// the steps, and each npm run names a script of the template
	for _, want := range []string{"npm ci --ignore-scripts --no-audit --no-fund", "npm run lint", "npm run typecheck", "npm test", "npm run build"} {
		if !hasLine(wf, "run: "+want) {
			t.Errorf("the workflow has no step `run: %s`", want)
		}
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(read("templates", "web", "package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	if _, ok := pkg.Scripts["test"]; !ok {
		t.Error("package.json has no script test, which `npm test` runs")
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
