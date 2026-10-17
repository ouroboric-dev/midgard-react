package midgard

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// copyTemplate copies templates/<name> into a temp dir, as it is in a product.
func copyTemplate(t *testing.T, name string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(filepath.Join("templates", name))); err != nil {
		t.Fatalf("templates/%s: %v", name, err)
	}
	return dst
}

func inDir(t *testing.T, dir string, tool string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(tool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// needPackages installs the template's packages, exactly as its lock file says, and skips the test when that
// can't be done: no npm, or no registry. YGG_REQUIRE_TEMPLATES makes it a failure instead (a CI that must not
// skip). It is the one place these tests need the network, and only on a machine without the npm cache.
func needPackages(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		if os.Getenv("YGG_REQUIRE_TEMPLATES") != "" {
			t.Fatal("npm is not installed")
		}
		t.Skip("npm is not installed (set YGG_REQUIRE_TEMPLATES to fail instead)")
	}
	if out, err := inDir(t, dir, "npm", "ci", "--ignore-scripts", "--no-audit", "--no-fund"); err != nil {
		if os.Getenv("YGG_REQUIRE_TEMPLATES") != "" {
			t.Fatalf("npm ci: %v\n%s", err, out)
		}
		t.Skipf("the template's packages can't be installed (set YGG_REQUIRE_TEMPLATES to fail instead): %v", err)
	}
}

func TestTheWebTemplateBuildsAndPassesItsOwnChecks(t *testing.T) {
	dir := copyTemplate(t, "web")
	needPackages(t, dir)
	for _, script := range []string{"typecheck", "lint", "test", "build"} {
		if out, err := inDir(t, dir, "npm", "run", script); err != nil {
			t.Errorf("npm run %s: %v\n%s", script, err, out)
		}
	}
	// The generated types are what the contract says: generating them again changes nothing.
	before, err := os.ReadFile(filepath.Join(dir, "src/api/schema.d.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := inDir(t, dir, "npm", "run", "gen:api"); err != nil {
		t.Fatalf("npm run gen:api: %v\n%s", err, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "src/api/schema.d.ts"))
	if string(before) != string(after) {
		t.Error("src/api/schema.d.ts is not what contracts/api.openapi.yaml generates; run npm run gen:api")
	}
}

func TestTheWebTemplateCarriesWhatARepoOfTheFactoryNeeds(t *testing.T) {
	dir := copyTemplate(t, "web")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("the template lacks %s: %v", name, err)
		}
		return string(b)
	}
	if lic := read("LICENSE"); !strings.HasPrefix(lic, "MIT License") {
		t.Errorf("LICENSE starts %q; a product may use a template under its own licence only if it is MIT", lic[:20])
	}
	if n := strings.Count(read("AGENTS.md"), "\n"); n > 100 {
		t.Errorf("AGENTS.md is %d lines; a short file that points into docs, about 100 lines (spec §4.2)", n)
	}
	for _, name := range []string{"init.sh", "package-lock.json", "contracts/api.openapi.yaml", "src/api/schema.d.ts", "e2e/smoke.spec.ts", "playwright.config.ts", ".gitignore"} {
		read(name)
	}
	// Every dependency is pinned to one version: no range, no tag.
	var pkg struct {
		Dependencies, DevDependencies map[string]string
	}
	if err := json.Unmarshal([]byte(read("package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name, v := range deps {
			if strings.ContainsAny(v, "^~<>*|x ") || v == "latest" || v == "" {
				t.Errorf("%s is %q; the template pins an exact version", name, v)
			}
		}
	}
	// The devcontainer.json is for people's editors: the factory never reads it, and nothing in it runs.
	var dc map[string]any
	if err := json.Unmarshal([]byte(read(".devcontainer/devcontainer.json")), &dc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"initializeCommand", "onCreateCommand", "updateContentCommand", "postCreateCommand", "postStartCommand",
		"postAttachCommand", "mounts", "runArgs", "privileged", "capAdd", "securityOpt", "dockerComposeFile", "build", "features", "containerEnv", "remoteEnv", "forwardPorts"} {
		if _, bad := dc[key]; bad {
			t.Errorf(".devcontainer/devcontainer.json sets %s, which runs or widens something when a person opens the repo", key)
		}
	}
}
