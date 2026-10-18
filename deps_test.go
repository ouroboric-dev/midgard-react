package midgard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// web-deps.sh is rung 4's first half for a web leaf (v0-T258): the dependency policy. The policy is baked into
// the image and passed to the checker by path; the leaf's own copy, if it has one, is never read. These tests
// run it with the real node, and stand-ins for npm and osv-scanner.

// lock makes the text of a package-lock.json that resolves the given packages (path: version).
func lock(t *testing.T, pkgs map[string]string) string {
	t.Helper()
	p := map[string]any{"": map[string]any{"name": "t"}}
	for path, v := range pkgs {
		p[path] = map[string]any{"version": v}
	}
	b, err := json.Marshal(map[string]any{"name": "t", "lockfileVersion": 3, "packages": p})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func pkgJSON(deps string) string {
	return `{"name":"t","scripts":{"lint":"eslint ."},` + deps + `}`
}

func depsLeaf(t *testing.T, deps string, pkgs map[string]string) string {
	t.Helper()
	return repo(t, map[string]string{".gitignore": "node_modules/\n", "package.json": pkgJSON(deps), "package-lock.json": lock(t, pkgs)})
}

func TestDepsPassesACleanLeafAndRunsTheAuditsThePolicyNames(t *testing.T) {
	dir := depsLeaf(t, `"dependencies":{"react":"19.3.0"},"devDependencies":{"vite":"~8.3.2","vitest":"5.0.3"}`,
		map[string]string{"node_modules/react": "19.3.0", "node_modules/vite": "8.3.2", "node_modules/vitest": "5.0.3"})
	bin := stubs(t, "osv-scanner")
	webTools(t, bin, nil)
	code, out := script(t, dir, bin, "web-deps.sh")
	want := "npm ci --ignore-scripts --no-audit --no-fund\nnpm audit --audit-level=high\nosv-scanner scan source -L package-lock.json\n"
	if code != 0 || calls(bin) != want || !strings.Contains(out, "deps policy: ok") {
		t.Fatalf("exit %d, calls %q\n%s", code, calls(bin), out)
	}
	for _, env := range []string{"STUB_NPM_EXIT=1", "STUB_OSV_SCANNER_EXIT=1"} {
		bin := stubs(t, "osv-scanner")
		webTools(t, bin, nil)
		if code, _ := script(t, dir, bin, "web-deps.sh", env); code == 0 {
			t.Errorf("%s passed", env)
		}
	}
}

func TestDepsRefusesWhatIsNotPinnedToAMinorLineAndWhatIsBelowTheMinimum(t *testing.T) {
	cases := []struct {
		name, deps string
		pkgs       map[string]string
		want       string
	}{
		{"a caret range", `"dependencies":{"react":"^19.3.0"}`, nil, "react ^19.3.0 is not pinned"},
		{"a wildcard", `"devDependencies":{"vitest":"*"}`, nil, "vitest * is not pinned"},
		{"a dist-tag", `"dependencies":{"react":"latest"}`, nil, "react latest is not pinned"},
		{"a range", `"dependencies":{"react":">=19.0.0"}`, nil, "react >=19.0.0 is not pinned"},
		{"an x range", `"dependencies":{"react":"19.x"}`, nil, "react 19.x is not pinned"},
		{"a git url", `"dependencies":{"evil":"github:someone/evil#main"}`, nil, "evil github:someone/evil#main is not pinned"},
		{"a file path", `"devDependencies":{"evil":"file:../evil"}`, nil, "evil file:../evil is not pinned"},
		{"an alias", `"dependencies":{"react":"npm:other@1.2.3"}`, nil, "react npm:other@1.2.3 is not pinned"},
		{"an optional dependency", `"optionalDependencies":{"fsevents":"^2.3.0"}`, nil, "fsevents ^2.3.0 is not pinned"},
		{"vite below the minimum", `"devDependencies":{"vite":"8.3.1"}`, map[string]string{"node_modules/vite": "8.3.1"}, "vite 8.3.1 is below the minimum 8.3.2"},
		{"vite far below, a major", `"devDependencies":{"vite":"7.9.9"}`, map[string]string{"node_modules/vite": "7.9.9"}, "vite 7.9.9 is below the minimum 8.3.2"},
		{"a copy of vite nested in another package", `"dependencies":{"react":"19.3.0"}`, map[string]string{"node_modules/vite": "8.3.2", "node_modules/x/node_modules/vite": "8.0.0"}, "node_modules/x/node_modules/vite 8.0.0 is below the minimum 8.3.2"},
		{"a scoped name is matched by its last parts", `"dependencies":{"react":"19.3.0"}`, map[string]string{"node_modules/vite": "8.3.2", "node_modules/@s/vite": "1.0.0"}, ""},
	}
	for _, c := range cases {
		dir := depsLeaf(t, c.deps, c.pkgs)
		bin := stubs(t, "osv-scanner")
		webTools(t, bin, nil)
		code, out := script(t, dir, bin, "web-deps.sh")
		if c.want == "" {
			if code != 0 {
				t.Errorf("%s: exit %d\n%s", c.name, code, out)
			}
			continue
		}
		if code == 0 || !strings.Contains(out, c.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", c.name, code, c.want, out)
		}
		if calls(bin) != "" {
			t.Errorf("%s: npm or an audit tool ran after the policy failed: %s", c.name, calls(bin))
		}
	}
}

func TestDepsNeedsALockFileAndNeverReadsAPolicyOfTheLeaf(t *testing.T) {
	dir := repo(t, map[string]string{"package.json": pkgJSON(`"dependencies":{"react":"^19.3.0"}`)})
	bin := stubs(t, "osv-scanner")
	webTools(t, bin, nil)
	if code, out := script(t, dir, bin, "web-deps.sh"); code == 0 || !strings.Contains(out, "no package-lock.json") {
		t.Errorf("no lock file: exit %d\n%s", code, out)
	}
	files := map[string]string{
		".gitignore": "node_modules/\n", "package.json": pkgJSON(`"dependencies":{"react":"^19.3.0"},"devDependencies":{"vite":"8.0.0"}`),
		"package-lock.json": lock(t, map[string]string{"node_modules/vite": "8.0.0"}),
		// the leaf's own policy, which would pass all of the above
		"deps-policy.json": `{"schema":"ygg.deps/v1","pin":"minor","minimum":{},"audit":[]}`,
		"devcontainer/checks/config/deps-policy.json": `{"schema":"ygg.deps/v1","pin":"minor","minimum":{},"audit":[]}`,
	}
	code, out := script(t, repo(t, files), bin, "web-deps.sh")
	if code == 0 {
		t.Fatalf("a leaf with its own policy passed:\n%s", out)
	}
	for _, want := range []string{"react ^19.3.0 is not pinned", "vite 8.0.0 is below the minimum 8.3.2", "2 problem(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
}

// A package.json of a leaf cannot make the checker run its code: the checker reads files and runs nothing of the leaf.
func TestDepsCheckerRefusesAPolicyItDoesNotKnow(t *testing.T) {
	dir := repo(t, map[string]string{"package.json": "{}", "package-lock.json": lock(t, nil)})
	helper, err := filepath.Abs(filepath.Join("devcontainer", "checks", "lib", "webdeps.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	for name, policy := range map[string]string{
		"another schema":   `{"schema":"ygg.deps/v2","pin":"minor","minimum":{},"audit":[]}`,
		"another pin":      `{"schema":"ygg.deps/v1","pin":"major","minimum":{},"audit":[]}`,
		"an unknown audit": `{"schema":"ygg.deps/v1","pin":"minor","minimum":{},"audit":["govulncheck"]}`,
		"a bad minimum":    `{"schema":"ygg.deps/v1","pin":"minor","minimum":{"a":"one"},"audit":[]}`,
		"an unknown field": `{"schema":"ygg.deps/v1","pin":"minor","minimum":{},"audit":[],"extra":1}`,
		"not json":         `pin: minor`,
	} {
		p := filepath.Join(t.TempDir(), "policy.json")
		if err := os.WriteFile(p, []byte(policy), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("node", helper, "check", p)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "deps policy") {
			t.Errorf("%s: err %v\n%s", name, err, out)
		}
	}
}

func TestDepsPassesTheUnchangedTemplateWithTheRealNpm(t *testing.T) {
	dir := realLeaf(t, nil)
	bin := stubs(t, "osv-scanner") // OSV needs its database; the policy and npm's own audit are real
	code, out := script(t, dir, bin, "web-deps.sh")
	skipOffline(t, out)
	if code != 0 || !strings.Contains(out, "deps policy: ok") {
		t.Errorf("the template: exit %d\n%s", code, out)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("web-deps.sh left the leaf changed:\n%s", st)
	}
}

func TestTheBakedDepsPolicyIsValidAndTheRungReadsItFromBesideItself(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("devcontainer", "checks", "config", "deps-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema":"ygg.deps/v1"`, `"pin":"minor"`, `"vite":"8.3.2"`, `"npm-audit"`, `"osv"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the baked policy lacks %s", want)
		}
	}
	s, _ := os.ReadFile(filepath.Join("devcontainer", "checks", "web-deps.sh"))
	if !strings.Contains(string(s), `"$here/config/deps-policy.json"`) || !strings.Contains(string(s), `web-protected.sh`) {
		t.Error("web-deps.sh does not read the policy from beside itself, or does not run web-protected.sh first")
	}
}
