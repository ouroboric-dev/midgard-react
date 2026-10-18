package midgard

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

func loadGraph(t *testing.T) *schema.Graph {
	t.Helper()
	g, err := schema.LoadGraph("graph.yaml")
	if err != nil {
		t.Fatalf("LoadGraph() = %v", err)
	}
	return g
}

func TestThePackLoadsAsTheLeadRealmWithTwoStacksFromOneContext(t *testing.T) {
	p := loadPack(t)
	if p.Name != "midgard-react" || p.Graph != "graph.yaml" || p.Leaf == nil {
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
		used := e.Name() == "docs.sh" || e.Name() == "web-protected.sh" || e.Name() == "web-configs.sh" || e.Name() == "config" || e.Name() == "lib"
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

func TestTheSpecPromptNamesThisRealmAtTheVersionOfThisPack(t *testing.T) {
	want := "`midgard-react` " + loadPack(t).Version
	if !strings.Contains(prompt(t, "spec.md"), want) {
		t.Errorf("spec.md does not offer %s: a spec that names another version fails the leaves check", want)
	}
}

func TestTheGraphHasTheStagesOfSpecSection42InOrder(t *testing.T) {
	g := loadGraph(t)
	var ids []string
	for _, n := range g.Nodes {
		ids = append(ids, n.ID)
	}
	want := "design,design-gate,design-docs,spec,conformance,spec-gate,spec-docs,plan,reuse-review,plan-docs,reuse-docs,ready,leaves,plan-gate,tasks"
	if strings.Join(ids, ",") != want {
		t.Errorf("nodes = %v", ids)
	}
	by := map[string]schema.Node{}
	for _, n := range g.Nodes {
		by[n.ID] = n
	}
	if by["conformance"].Reviews != "spec" || by["spec-gate"].DeviationFrom != "conformance" || by["spec-gate"].Reruns != "spec" ||
		by["reuse-review"].Reviews != "plan" || by["ready"].BackTo != "plan" || by["plan-gate"].Reruns != "plan" {
		t.Errorf("the spec stage and the plan stage are not wired as spec §4.2 says: %+v", by)
	}
	if !slices.Contains(by["reuse-review"].Skills, "ponytail-review") {
		t.Error("the reuse review does not load the Ponytail skill")
	}
	// A planner works in a scratch workspace and publishes after its gate; none touches a leaf.
	for _, id := range []string{"design", "spec", "conformance", "plan", "reuse-review"} {
		if !by[id].Scratch || by[id].Repo != "" {
			t.Errorf("%s is not a scratch agent", id)
		}
	}
	for id, from := range map[string]string{"design-docs": "design", "spec-docs": "spec", "plan-docs": "plan", "reuse-docs": "reuse-review"} {
		if by[id].From != from || by[id].To != "docs" {
			t.Errorf("%s publishes %s to %q, want %s to docs", id, by[id].From, by[id].To, from)
		}
	}
}

func TestEveryTaskRunsTheLoopWithThreeReviewersThenTheGatesAndTheMoveWithAcceptance(t *testing.T) {
	g := loadGraph(t)
	body := g.Nodes[len(g.Nodes)-1].Body
	var ids []string
	for _, n := range body {
		ids = append(ids, n.ID)
	}
	if strings.Join(ids, ",") != "implement,feature-gate,merge-gate,land,pointer-gate,contract-gate,move" {
		t.Fatalf("the task shape = %v", ids)
	}
	impl := body[0]
	var rv []string
	for _, r := range impl.Reviewers {
		rv = append(rv, r.ID)
	}
	if impl.RealmFrom != schema.RealmLeaf || impl.Role != "implementer" || impl.Attempts != 3 || strings.Join(rv, ",") != "spec-compliance,simplicity,quality" {
		t.Errorf("implement = %+v; reviewers %v run in the spec's order (§9.5 rung 6)", impl, rv)
	}
	if !slices.Contains(impl.Reviewers[1].Skills, "ponytail-review") {
		t.Error("the simplicity reviewer does not load the Ponytail skill")
	}
	land, move := body[3], body[6]
	if land.BackTo != "implement" || move.BackTo != "implement" || !slices.Equal(move.Acceptance, []string{"accept"}) {
		t.Errorf("land %+v, move %+v: a refusal goes back to the implementer, and the move runs the acceptance check", land, move)
	}
	if _, ok := loadPack(t).Checks["accept"]; !ok {
		t.Error("the graph's acceptance check is not defined in the pack")
	}
}

func TestEveryPromptIsUsedAndEveryPromptTheGraphNamesIsThere(t *testing.T) {
	g := loadGraph(t)
	used := map[string]bool{"implementer.md": true} // the leaf recipe's
	roles := map[string]bool{}
	for _, n := range g.All() {
		if n.Prompt != "" {
			used[filepath.Base(n.Prompt)] = true
			if _, err := os.Stat(n.Prompt); err != nil {
				t.Errorf("node %s names the prompt %s: %v", n.ID, n.Prompt, err)
			}
		}
		if n.Role != "" {
			roles[n.Role] = true
		}
		for _, r := range n.Reviewers {
			used[filepath.Base(r.Prompt)] = true
			roles[r.Role] = true
			if _, err := os.Stat(r.Prompt); err != nil {
				t.Errorf("reviewer %s names the prompt %s: %v", r.ID, r.Prompt, err)
			}
		}
	}
	entries, _ := os.ReadDir("prompts")
	for _, e := range entries {
		if !used[e.Name()] {
			t.Errorf("prompts/%s is used by no node", e.Name())
		}
	}
	for r := range roles {
		if !slices.Contains([]string{"planner", "implementer", "reviewer"}, r) {
			t.Errorf("the graph uses the role %q, which an operator's factory.yaml is not told to define", r)
		}
	}
}

// Spec §3.4: a move that changes a contract is the pointer-contract approval point. The gate judges the move
// that follows it, so it comes after the leaf has landed and before the move. The move also ships the routing
// indexes (spec §8): Odin builds REPOS.md's wiring block and .ygg/impact.json, and the shrink guard reads them.
func TestTheContractGateComesBeforeTheMoveAndTheMoveShipsTheRoutingIndexes(t *testing.T) {
	body := loadGraph(t).Nodes[len(loadGraph(t).Nodes)-1].Body
	by := map[string]int{}
	for i, n := range body {
		by[n.ID] = i
	}
	gate, move := body[by["contract-gate"]], body[by["move"]]
	if gate.Kind != schema.KindGate || gate.Gate != gungnir.GatePointerContract || by["contract-gate"] != by["move"]-1 || by["contract-gate"] < by["land"] {
		t.Errorf("contract-gate = %+v at %d; want the pointer-contract gate right before the move and after the land", gate, by["contract-gate"])
	}
	if !slices.Equal(move.Generated, []string{"REPOS.md", ".ygg/impact.json"}) {
		t.Errorf("the move generates %v; it must list both routing indexes so Odin builds them and the shrink guard reads them", move.Generated)
	}
	if loadPack(t).Routing != nil {
		t.Error("the pack sets a routing threshold; a project needs its bench first (docs/architecture.md)")
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
