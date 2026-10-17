package midgard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var buildPrompts = []string{"implementer.md", "reviewer-spec.md", "reviewer-simplicity.md", "reviewer-quality.md"}

func TestEveryBuildPromptIsThereAndKeepsTheBenchmarkOutOfAgentsReach(t *testing.T) {
	sensible(t, buildPrompts)
}

func TestThePromptsNameOnlyScriptsAndCommandsThatExist(t *testing.T) {
	pkg, err := os.ReadFile(filepath.Join("templates", "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct{ Scripts map[string]string }
	if err := json.Unmarshal(pkg, &p); err != nil {
		t.Fatal(err)
	}
	run := regexp.MustCompile("npm (?:run )?([a-z0-9:]+)")
	for _, name := range []string{"implementer.md"} {
		for _, m := range run.FindAllStringSubmatch(prompt(t, name), -1) {
			if m[1] == "test" || m[1] == "ci" {
				if m[1] == "test" && p.Scripts["test"] == "" {
					t.Errorf("%s names npm test, which the template lacks", name)
				}
				continue
			}
			if _, ok := p.Scripts[m[1]]; !ok {
				t.Errorf("%s names npm run %s, which the template's package.json lacks", name, m[1])
			}
		}
	}
	impl := prompt(t, "implementer.md")
	for _, want := range []string{"AGENTS.md", "Test first", "Do not run `git commit`", "contracts/api.openapi.yaml", "npm run gen:api", ".devcontainer/", "Findings to fix", "src/lib/api.ts"} {
		if !strings.Contains(impl, want) {
			t.Errorf("implementer.md lacks %q", want)
		}
	}
}

func TestTheReviewersSayWhatToLookAtAndHowToNameFindings(t *testing.T) {
	for name, ids := range map[string]string{"reviewer-spec.md": "spec-1", "reviewer-simplicity.md": "simple-1", "reviewer-quality.md": "quality-1"} {
		text := prompt(t, name)
		for _, want := range []string{"read-only", "git merge-base HEAD main", ids, "file:line", "**blocker**"} {
			if want == "**blocker**" && name == "reviewer-simplicity.md" {
				want = "**Must fix**"
			}
			if !strings.Contains(text, want) {
				t.Errorf("%s lacks %q", name, want)
			}
		}
		if strings.Contains(text, "git commit") {
			t.Errorf("%s talks of committing; a reviewer changes nothing", name)
		}
	}
	if !strings.Contains(prompt(t, "reviewer-simplicity.md"), "The skill below") || !strings.Contains(prompt(t, "reuse-review.md"), "The skill below") {
		t.Error("a prompt that relies on the vendored skill must say that the skill follows it")
	}
}
