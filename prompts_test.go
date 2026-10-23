package midgard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func prompt(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("prompts", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

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

// The release (gallery-runtime T17): the implementer prompt and the web template's AGENTS.md name release.sh and
// say to keep it working, as they do for run.sh.
func TestPromptNamesTheRelease(t *testing.T) {
	agents, err := os.ReadFile(filepath.Join("templates", "web", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"prompts/implementer.md": prompt(t, "implementer.md"), "templates/web/AGENTS.md": string(agents)} {
		if !strings.Contains(text, "release.sh") {
			t.Errorf("%s does not name release.sh", name)
		}
		if !strings.Contains(strings.ToLower(text), "keep release.sh working") && !strings.Contains(strings.ToLower(text), "keep all three working") {
			t.Errorf("%s does not say to keep release.sh working", name)
		}
	}
}
