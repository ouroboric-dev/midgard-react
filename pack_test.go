package midgard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ouroboric-dev/gungnir"
	"github.com/ouroboric-dev/gungnir/schema"
)

func loadPack(t *testing.T) *schema.Pack {
	t.Helper()
	p, err := schema.LoadPack(".")
	if err != nil {
		t.Fatalf("LoadPack() = %v", err)
	}
	return p
}

func TestTheRealmHoldsNoGraphAndNoStagePrompt(t *testing.T) {
	if _, err := os.Stat("graph.yaml"); !os.IsNotExist(err) {
		t.Errorf("graph.yaml is still here (%v); the graph lives in Voluspa", err)
	}
	if g := loadPack(t).Graph; g != "" {
		t.Errorf("the pack names the graph %q; a flavor holds none", g)
	}
	entries, err := os.ReadDir("prompts")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "implementer.md" {
		t.Errorf("prompts/ holds %v; want implementer.md only", names)
	}
}

func TestThePackLoadsAsAFlavorWithTwoStacksFromOneContext(t *testing.T) {
	p := loadPack(t)
	if p.Name != "midgard-react" || p.Graph != "" || p.Leaf == nil {
		t.Fatalf("pack = %+v", p)
	}
	web, err := p.StackRef(".")
	if err != nil {
		t.Fatal(err)
	}
	acc, err := p.AcceptanceRef(".")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^localhost/ygg-stack-midgard-react:[0-9a-f]{12}$`).MatchString(web) ||
		!regexp.MustCompile(`^localhost/ygg-stack-midgard-react-acceptance:[0-9a-f]{12}$`).MatchString(acc) || web == acc {
		t.Errorf("stacks %q and %q; the leaf's and the acceptance sandbox's are two images of one context", web, acc)
	}
	if _, ok := p.Checks["accept"]; !ok || p.Sandbox.Acceptance.Target == "" {
		t.Error("the pack defines no accept check or no acceptance stack; the pointer move runs them")
	}
	if p.Routing != nil {
		t.Errorf("routing = %+v; the graph pack's routing block is the one a move measures, so a realm sets none", p.Routing)
	}
	want := []string{"static", "unit", "tdd", "contract", "deps", "security", "e2e"}
	if strings.Join(p.Leaf.Checks, ",") != strings.Join(want, ",") {
		t.Errorf("the ladder is %v, want %v: cheap rungs first, the browser last", p.Leaf.Checks, want)
	}
	for name, argv := range p.Checks {
		if len(argv) != 1 || !strings.HasPrefix(argv[0], "/opt/ygg/checks/") {
			t.Errorf("check %s is %v; it must be one script in /opt/ygg/checks", name, argv)
			continue
		}
		if st, err := os.Stat(filepath.Join("devcontainer", "checks", filepath.Base(argv[0]))); err != nil || st.Mode()&0o111 == 0 {
			t.Errorf("check %s names %s, which devcontainer/checks does not hold as an executable: %v", name, argv[0], err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join("devcontainer", "checks"))
	for _, e := range entries {
		// docs.sh, web-protected.sh and web-configs.sh (sourced) are used by the web rungs; config/ holds the baked
		// tool configs web-configs.sh installs and the policy web-deps.sh reads, and lib/ holds the checker it runs.
		// protected-common is the list web-protected.sh reads.
		used := e.Name() == "docs.sh" || e.Name() == "web-protected.sh" || e.Name() == "protected-common" || e.Name() == "web-configs.sh" || e.Name() == "config" || e.Name() == "lib"
		for _, argv := range p.Checks {
			used = used || filepath.Base(argv[0]) == e.Name()
		}
		if !used {
			t.Errorf("devcontainer/checks/%s is baked into the image and no check runs it", e.Name())
		}
	}
	for _, h := range p.Sandbox.Egress {
		if !gungnir.ValidHost(h) {
			t.Errorf("egress %q is not a plain host", h)
		}
	}
	// The stages the pack asks for are stages of the Containerfile.
	cf, _ := os.ReadFile(filepath.Join("devcontainer", "Containerfile"))
	for _, stage := range []string{p.Sandbox.Target, p.Sandbox.Acceptance.Target} {
		if !regexp.MustCompile(`(?m)^FROM .* AS ` + stage + `$`).Match(cf) {
			t.Errorf("the pack builds the stage %q, which the Containerfile lacks", stage)
		}
	}
}

// Routing finds a contract by its name (Odin: a file name that ends in openapi.yaml). The web template's copy
// of the API contract must be named so, or the impact index never sees it.
func TestTheTemplatesContractIsNamedSoRoutingFindsIt(t *testing.T) {
	var found []string
	err := filepath.WalkDir("templates/web", func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), "openapi") {
			found = append(found, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || !strings.HasSuffix(found[0], "openapi.yaml") {
		t.Errorf("OpenAPI files in the web template = %v; want one, named *openapi.yaml", found)
	}
}
