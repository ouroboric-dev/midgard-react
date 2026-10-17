package midgard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ouroboric-dev/gungnir/schema"
)

func prompt(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("prompts", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var fence = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")

// blocks returns the fenced code blocks of a prompt.
func blocks(text string) []string {
	var out []string
	for _, m := range fence.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

var planningPrompts = []string{"design.md", "spec.md", "conformance.md", "plan.md", "reuse-review.md"}

// sensible checks a prompt is a page or two and keeps the benchmark out of an agent's reach.
func sensible(t *testing.T, names []string) {
	t.Helper()
	for _, name := range names {
		text := prompt(t, name)
		if len(text) < 600 || len(text) > 12<<10 {
			t.Errorf("%s is %d bytes; a prompt is a page or two", name, len(text))
		}
		for _, bad := range []string{"huldra", "hidden suite", "ouroboric-private", "benchmark"} {
			if strings.Contains(strings.ToLower(text), bad) {
				t.Errorf("%s mentions %q; an agent that builds a product never learns what scores it", name, bad)
			}
		}
	}
}

func TestEveryPlanningPromptIsThereAndKeepsTheBenchmarkOutOfAgentsReach(t *testing.T) {
	sensible(t, planningPrompts)
}

func TestThePlanningPromptsSayWhereTheirDocumentsGo(t *testing.T) {
	for name, paths := range map[string][]string{
		"design.md":       {"out/1-product/design/<date>-<slug>.md"},
		"spec.md":         {"out/2-engineering/specs/<date>-<slug>.md", "out/leaves.yaml", "docs/1-product/design/"},
		"conformance.md":  {"out/conformance.json", "reviewed/2-engineering/specs/", "reviewed/leaves.yaml", "docs/1-product/design/"},
		"plan.md":         {"out/2-engineering/plans/<date>-<slug>/", "docs/leaves.yaml", "docs/2-engineering/specs/"},
		"reuse-review.md": {"out/2-engineering/reviews/<date>-<slug>-reuse-review.md", "reviewed/2-engineering/plans/"},
	} {
		text := prompt(t, name)
		for _, p := range paths {
			if !strings.Contains(text, p) {
				t.Errorf("%s does not mention %s", name, p)
			}
		}
	}
}

// What the conformance prompt shows as the answer is an answer Odin accepts.
func TestTheConformancePromptsExamplesAreVerdictsOdinReads(t *testing.T) {
	text := prompt(t, "conformance.md")
	for _, ex := range []string{`{"deviation": false}`, `{"deviation": true, "amendment": "<text>"}`} {
		if !strings.Contains(text, ex) {
			t.Fatalf("the prompt lacks the example %s", ex)
		}
		c, err := schema.ParseConformance([]byte(strings.ReplaceAll(ex, "<text>", "The spec adds offline mode.")))
		if err != nil {
			t.Errorf("%s: %v", ex, err)
		} else if c.Deviation != strings.Contains(ex, "true") {
			t.Errorf("%s was read as %+v", ex, c)
		}
	}
}

// The task the plan prompt shows is a task Odin reads, and a ready one; the leaves file the spec prompt shows
// is a leaves file Odin reads.
func TestThePlanAndSpecPromptsExamplesAreFilesOdinReads(t *testing.T) {
	var task string
	for _, b := range blocks(prompt(t, "plan.md")) {
		if strings.Contains(b, "**Named test:**") {
			task = b
		}
	}
	if task == "" {
		t.Fatal("the plan prompt shows no task")
	}
	task = strings.NewReplacer("<plan>", "gallery", "<title>", "the API stores galleries", "<a thing that is true when the task is done>", "a gallery can be created", "<another>", "a gallery has a name",
		"<the test file and test name that proves it>", "gallery_test.go TestCreateGallery").Replace(task)
	fs, err := schema.ParseTasks("tasks.md", []byte(task))
	if err != nil || len(fs) != 1 {
		t.Fatalf("ParseTasks() = %v, %v", fs, err)
	}
	if err := schema.CheckReady(fs, []string{"repos/api"}); err != nil {
		t.Errorf("the example task is not ready: %v", err)
	}
	var leaves string
	for _, b := range blocks(prompt(t, "spec.md")) {
		if strings.HasPrefix(b, "leaves:") {
			leaves = b
		}
	}
	l, err := schema.ParseLeaves("leaves.yaml", []byte(leaves))
	if err != nil {
		t.Fatalf("ParseLeaves() = %v", err)
	}
	if got := strings.Join(l.Paths(), ","); got != "repos/api,repos/web" || l.Leaves["repos/web"].Consumes[0] != "repos/api" {
		t.Errorf("leaves = %v", l.Leaves)
	}
}
