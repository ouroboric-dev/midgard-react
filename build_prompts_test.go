package midgard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var buildPrompts = []string{"implementer.md"}

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
