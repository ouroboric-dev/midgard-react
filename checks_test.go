package midgard

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scripts of devcontainer/checks are baked into the stack image and run in a leaf. These tests run them
// on small leaves made in a temp dir, with the real go, gofmt and git, and stand-ins for the tools that only
// the image has (golangci-lint, sqlc, oasdiff, vacuum, govulncheck, osv-scanner), which log how they were
// called and exit as the test says.

// leaf writes files (path: body) into a new temp dir and returns it.
func leaf(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	put(t, dir, files)
	return dir
}

func put(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stubs makes a folder of executables named tools. Each logs "<tool> <args>" to calls.log in the folder and
// exits with the number in the variable STUB_<TOOL>_EXIT (0 when it is unset), after printing STUB_<TOOL>_OUT.
func stubs(t *testing.T, tools ...string) (bin string) {
	t.Helper()
	bin = t.TempDir()
	for _, tool := range tools {
		v := strings.ToUpper(strings.ReplaceAll(tool, "-", "_"))
		body := "#!/bin/sh\necho \"" + tool + " $*\" >> \"$(dirname \"$0\")/calls.log\"\n" +
			"[ -z \"$STUB_" + v + "_OUT\" ] || echo \"$STUB_" + v + "_OUT\"\nexit \"${STUB_" + v + "_EXIT:-0}\"\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func calls(bin string) string {
	b, _ := os.ReadFile(filepath.Join(bin, "calls.log"))
	return string(b)
}

// script runs devcontainer/checks/name in dir with bin first on PATH and the given extra environment
// (KEY=value), and returns its exit code and its output.
func script(t *testing.T, dir, bin, name string, env ...string) (int, string) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("devcontainer", "checks", name))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", abs)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if bin != "" {
		cmd.Env = append(cmd.Env, "PATH="+bin+":"+os.Getenv("PATH"))
	}
	cmd.Env = append(cmd.Env, env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), out.String()
	}
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return 0, out.String()
}

// repo makes a git repo with the given files committed on main, and returns it.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := leaf(t, files)
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "start")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.invalid"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// work puts files on a branch of the repo made from main, committed.
func work(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	git(t, dir, "switch", "-q", "-c", "work")
	put(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "work")
}

// stub writes one tool of a stub folder with a body of its own.
func stub(t *testing.T, bin, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDocsAcceptsDocumentsAndNamesEveryProblemOfTheOthers(t *testing.T) {
	good := "---\ntitle: A\nstatus: draft\ndate: 2026-10-17\n---\n# A\n\nSee [b](b.md) and [the web](https://example.com/x).\n"
	if code, out := script(t, leaf(t, map[string]string{"docs/a.md": good, "docs/b.md": good}), "", "docs.sh"); code != 0 || !strings.Contains(out, "docs: ok") {
		t.Errorf("good documents: exit %d\n%s", code, out)
	}
	code, out := script(t, leaf(t, map[string]string{"docs/a.md": strings.Replace(good, "[b](b.md)", "[gone](gone.md)", 1), "docs/No Plain.md": "# x\n"}), "", "docs.sh")
	if code == 0 || !strings.Contains(out, "points at nothing") || !strings.Contains(out, "file name must be") || !strings.Contains(out, "starts without front matter") {
		t.Errorf("bad documents: exit %d\n%s", code, out)
	}
}

func TestStaticInstallsOnlyWhatIsMissingAndStopsAtTheFirstProblem(t *testing.T) {
	bin := stubs(t, "npm")
	if code, out := script(t, leaf(t, map[string]string{"package.json": "{}"}), bin, "web-static.sh"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	want := "npm ci --no-audit --no-fund\nnpm run lint\nnpm run typecheck\nnpm run build\n"
	if calls(bin) != want {
		t.Errorf("calls = %q, want %q: install, lint, type-check, build", calls(bin), want)
	}
	bin = stubs(t, "npm")
	if code, _ := script(t, leaf(t, map[string]string{"node_modules/.keep": ""}), bin, "web-static.sh"); code != 0 || strings.Contains(calls(bin), "npm ci") {
		t.Errorf("installed packages were installed again: %q", calls(bin))
	}
	// A failing step ends it: the later ones do not run.
	bin = stubs(t, "npm")
	stub(t, bin, "npm", `echo "npm $*" >> "$(dirname "$0")/calls.log"
[ "$*" = "run typecheck" ] && { echo "src/a.ts(1,1): error TS2322"; exit 2; }
exit 0
`)
	code, out := script(t, leaf(t, map[string]string{"node_modules/.keep": ""}), bin, "web-static.sh")
	if code == 0 || !strings.Contains(out, "TS2322") || strings.Contains(calls(bin), "run build") {
		t.Errorf("a type error: exit %d, calls %q\n%s", code, calls(bin), out)
	}
	if code, out := script(t, leaf(t, map[string]string{"node_modules/.keep": "", "docs/Bad Name.md": "x"}), stubs(t, "npm"), "web-static.sh"); code == 0 || !strings.Contains(out, "docs:") {
		t.Errorf("a bad document: exit %d\n%s", code, out)
	}
}

func TestUnitRunsTheTests(t *testing.T) {
	bin := stubs(t, "npm")
	if code, _ := script(t, leaf(t, map[string]string{"node_modules/.keep": ""}), bin, "web-unit.sh"); code != 0 || calls(bin) != "npm test\n" {
		t.Errorf("calls = %q", calls(bin))
	}
	if code, _ := script(t, leaf(t, map[string]string{"node_modules/.keep": ""}), stubs(t, "npm"), "web-unit.sh", "STUB_NPM_EXIT=1"); code == 0 {
		t.Error("a failing test passed")
	}
}

func TestTDDWantsTheNewTestsToFailOnTheCommitTheTaskStartedFrom(t *testing.T) {
	start := map[string]string{"node_modules/.keep": "", ".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n", "src/a.test.ts": "// test\n"}
	// npx stands in for vitest: it fails on the parent when the parent lacks the code the new test needs, as a
	// real run does, and logs where it ran and what was there.
	vitest := `echo "npx $*" >> "$(dirname "$0")/calls.log"
[ -e node_modules/.keep ] || { echo "no packages beside the parent"; exit 3; }
if [ -e src/lib/greet.test.ts ] && [ ! -e src/lib/greet.ts ]; then echo "Failed to resolve import @/lib/greet"; exit 1; fi
exit "${STUB_PARENT_EXIT:-0}"
`
	newCode := map[string]string{"src/lib/greet.ts": "export const greet = 1;\n", "src/lib/greet.test.ts": "// test\n"}
	cases := []struct {
		name  string
		files map[string]string
		env   []string
		pass  bool
		want  string
	}{
		{"code and a test that needs it", newCode, nil, true, "fail on the commit the task started from"},
		{"code with no test", map[string]string{"src/lib/greet.ts": "export const greet = 1;\n"}, nil, false, "changed without a new or changed unit test"},
		{"a test that passes before the code", map[string]string{"src/b.test.ts": "// test\n"}, nil, false, "test nothing new"},
		{"only a generated type file", map[string]string{"src/api/schema.d.ts": "export {};\n"}, nil, true, "no app code changed"},
		{"only a document", map[string]string{"README.md": "x\n"}, nil, true, "no app code changed"},
		{"only a Playwright spec", map[string]string{"e2e/home.spec.ts": "// spec\n"}, nil, true, "cannot be shown to fail"},
	}
	for _, c := range cases {
		dir := repo(t, start)
		work(t, dir, c.files)
		bin := t.TempDir()
		stub(t, bin, "npx", vitest)
		code, out := script(t, dir, bin, "web-tdd.sh", c.env...)
		if (code == 0) != c.pass || !strings.Contains(out, c.want) {
			t.Errorf("%s: exit %d, want pass=%v and %q\n%s", c.name, code, c.pass, c.want, out)
		}
	}
	dir := repo(t, start)
	git(t, dir, "switch", "-q", "-c", "work")
	bin := t.TempDir()
	stub(t, bin, "npx", vitest)
	if code, out := script(t, dir, bin, "web-tdd.sh"); code != 0 || !strings.Contains(out, "no app code changed") {
		t.Errorf("an unchanged branch: exit %d\n%s", code, out)
	}
	// The leaf is left as it was, and vitest was given the new test files only.
	dir = repo(t, start)
	work(t, dir, newCode)
	bin = t.TempDir()
	stub(t, bin, "npx", vitest)
	script(t, dir, bin, "web-tdd.sh")
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("the check left the leaf changed:\n%s", st)
	}
	if got := calls(bin); got != "npx vitest run src/lib/greet.test.ts\n" {
		t.Errorf("vitest was run as %q, want only the new test file", got)
	}
}
