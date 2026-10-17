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

// webTools makes the npm stand-in of the web scripts. "npm ci" does what the real one does: it deletes
// node_modules and installs the tools again, here the stand-ins named in tools (tool: body; an empty body
// logs like stubs does). Whatever the leaf had in node_modules is gone afterwards. Every call is logged to
// calls.log in bin, as "<tool> <args>".
func webTools(t *testing.T, bin string, tools map[string]string) {
	t.Helper()
	install := "rm -rf node_modules\nmkdir -p node_modules/.bin\ntouch node_modules/.keep\n"
	for tool, body := range tools {
		v := strings.ToUpper(strings.ReplaceAll(tool, "-", "_"))
		if body == "" {
			body = "echo \"" + tool + " $*\" >> \"" + bin + "/calls.log\"\n[ -z \"$STUB_" + v + "_OUT\" ] || echo \"$STUB_" + v + "_OUT\"\nexit \"${STUB_" + v + "_EXIT:-0}\"\n"
		}
		install += "cat > node_modules/.bin/" + tool + " <<'EOT'\n#!/bin/sh\n" + body + "EOT\nchmod +x node_modules/.bin/" + tool + "\n"
	}
	stub(t, bin, "npm", "echo \"npm $*\" >> \""+bin+"/calls.log\"\nif [ \"$1\" = ci ]; then\n"+install+"fi\nexit \"${STUB_NPM_EXIT:-0}\"\n")
}

// tamper plants a tool in the leaf's node_modules that passes and says so, as an agent that wants a rung
// green would.
func tamper(t *testing.T, dir string, tools ...string) {
	t.Helper()
	files := map[string]string{"node_modules/planted": "x"}
	for _, tool := range tools {
		files["node_modules/.bin/"+tool] = "#!/bin/sh\necho TAMPERED " + tool + "\nexit 0\n"
	}
	put(t, dir, files)
	for _, tool := range tools {
		if err := os.Chmod(filepath.Join(dir, "node_modules/.bin", tool), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStaticInstallsCleanCallsTheToolsByPathAndStopsAtTheFirstProblem(t *testing.T) {
	tools := map[string]string{"eslint": "", "tsc": "", "vite": ""}
	bin := t.TempDir()
	webTools(t, bin, tools)
	if code, out := script(t, repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n"}), bin, "web-static.sh"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	want := "npm ci --no-audit --no-fund\neslint .\ntsc --noEmit -p tsconfig.json\ntsc --noEmit -p tsconfig.json\nvite build\n"
	if calls(bin) != want {
		t.Errorf("calls = %q, want %q: install, lint, type-check, build", calls(bin), want)
	}
	// A leaf with packages installed gets them installed again, from nothing: a tool the agent planted in
	// node_modules is not what runs, and the check does not trust the folder.
	bin = t.TempDir()
	webTools(t, bin, tools)
	dir := repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n"})
	tamper(t, dir, "eslint", "tsc", "vite")
	code, out := script(t, dir, bin, "web-static.sh", "STUB_ESLINT_EXIT=1")
	if code == 0 || strings.Contains(out, "TAMPERED") || !strings.HasPrefix(calls(bin), "npm ci --no-audit --no-fund\n") {
		t.Errorf("a planted node_modules: exit %d, calls %q\n%s", code, calls(bin), out)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules/planted")); err == nil {
		t.Error("the planted file survived the install")
	}
	// A failing step ends it: the later ones do not run.
	bin = t.TempDir()
	webTools(t, bin, tools)
	code, out = script(t, repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n"}), bin, "web-static.sh", "STUB_TSC_EXIT=2", "STUB_TSC_OUT=src/a.ts(1,1): error TS2322")
	if code == 0 || !strings.Contains(out, "TS2322") || strings.Contains(calls(bin), "vite build") {
		t.Errorf("a type error: exit %d, calls %q\n%s", code, calls(bin), out)
	}
	bin = t.TempDir()
	webTools(t, bin, tools)
	if code, out := script(t, repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n", "docs/Bad Name.md": "x"}), bin, "web-static.sh"); code == 0 || !strings.Contains(out, "docs:") {
		t.Errorf("a bad document: exit %d\n%s", code, out)
	}
}

func TestStaticRefusesAChangeToTheConfigurationItReads(t *testing.T) {
	pkg := func(scripts, deps string) string {
		return `{"scripts": {` + scripts + `}, "dependencies": {` + deps + `}}`
	}
	start := map[string]string{".gitignore": "node_modules/\n", "package.json": pkg(`"lint": "eslint ."`, `"react": "1.0.0"`), "eslint.config.js": "export default [];\n",
		"tsconfig.json": "{}\n", "vite.config.ts": "export default {};\n", "playwright.config.ts": "export default {};\n", "src/a.ts": "export const a = 1;\n"}
	cases := []struct {
		name  string
		files map[string]string
		fails string // the file named in the refusal; empty when the change is fine
	}{
		{"eslint config", map[string]string{"eslint.config.js": "export default [{ignores: ['**']}];\n"}, "eslint.config.js"},
		{"tsconfig", map[string]string{"tsconfig.json": "{\"include\": []}\n"}, "tsconfig.json"},
		{"vite config", map[string]string{"vite.config.ts": "export default {test: {passWithNoTests: true}};\n"}, "vite.config.ts"},
		{"playwright config", map[string]string{"playwright.config.ts": "export default {retries: 9};\n"}, "playwright.config.ts"},
		{"a script of package.json", map[string]string{"package.json": pkg(`"lint": "eslint . || true"`, `"react": "1.0.0"`)}, "package.json"},
		{"a new script", map[string]string{"package.json": pkg(`"lint": "eslint .", "test": "true"`, `"react": "1.0.0"`)}, "package.json"},
		{"the dependencies of package.json", map[string]string{"package.json": pkg(`"lint": "eslint ."`, `"react": "1.0.0", "zod": "3.0.0"`)}, ""},
		{"app code only", map[string]string{"src/a.ts": "export const a = 2;\n"}, ""},
	}
	for _, c := range cases {
		dir := repo(t, start)
		work(t, dir, c.files)
		bin := t.TempDir()
		webTools(t, bin, map[string]string{"eslint": "", "tsc": "", "vite": ""})
		code, out := script(t, dir, bin, "web-static.sh")
		if c.fails == "" {
			if code != 0 {
				t.Errorf("%s: exit %d\n%s", c.name, code, out)
			}
			continue
		}
		if code == 0 || !strings.Contains(out, "protected") || !strings.Contains(out, c.fails) || calls(bin) != "" {
			t.Errorf("%s: exit %d, calls %q, want a refusal naming %s\n%s", c.name, code, calls(bin), c.fails, out)
		}
	}
	// A change not committed yet counts, and so does a deleted file.
	dir := repo(t, start)
	put(t, dir, map[string]string{"eslint.config.js": "export default [{ignores: ['**']}];\n"})
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"eslint": "", "tsc": "", "vite": ""})
	if code, out := script(t, dir, bin, "web-static.sh"); code == 0 || !strings.Contains(out, "eslint.config.js") {
		t.Errorf("an uncommitted change: exit %d\n%s", code, out)
	}
	dir = repo(t, start)
	git(t, dir, "switch", "-q", "-c", "work")
	git(t, dir, "rm", "-q", "tsconfig.json")
	git(t, dir, "commit", "-q", "-m", "gone")
	if code, out := script(t, dir, bin, "web-static.sh"); code == 0 || !strings.Contains(out, "tsconfig.json") {
		t.Errorf("a deleted config: exit %d\n%s", code, out)
	}
	// With no main to compare with the check cannot tell, and refuses.
	if code, out := script(t, leaf(t, start), bin, "web-static.sh"); code == 0 || !strings.Contains(out, "no main") {
		t.Errorf("no main: exit %d\n%s", code, out)
	}
}

func TestUnitInstallsCleanAndRunsVitestByPath(t *testing.T) {
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	dir := leaf(t, map[string]string{"package.json": "{}"})
	tamper(t, dir, "vitest")
	code, out := script(t, dir, bin, "web-unit.sh", "STUB_VITEST_EXIT=1")
	if code == 0 || strings.Contains(out, "TAMPERED") {
		t.Errorf("a planted vitest was trusted: exit %d\n%s", code, out)
	}
	bin = t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	if code, _ := script(t, leaf(t, map[string]string{"package.json": "{}"}), bin, "web-unit.sh"); code != 0 || calls(bin) != "npm ci --no-audit --no-fund\nvitest run\n" {
		t.Errorf("calls = %q", calls(bin))
	}
}

func TestTDDWantsTheNewTestsToFailOnTheCommitTheTaskStartedFrom(t *testing.T) {
	start := map[string]string{"node_modules/.keep": "", ".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n", "src/a.test.ts": "// test\n"}
	// vitest stands in for the real one, installed by "npm ci" into node_modules/.bin: it fails on the parent
	// when the parent lacks the code the new test needs, as a real run does, and logs where it ran and what
	// was there.
	vitest := func(bin string) string {
		return `echo "vitest $*" >> "` + bin + `/calls.log"
[ -e node_modules/.keep ] || { echo "no packages beside the parent"; exit 3; }
if [ -e src/lib/greet.test.ts ] && [ ! -e src/lib/greet.ts ]; then echo "Failed to resolve import @/lib/greet"; exit 1; fi
exit "${STUB_PARENT_EXIT:-0}"
`
	}
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
		webTools(t, bin, map[string]string{"vitest": vitest(bin)})
		code, out := script(t, dir, bin, "web-tdd.sh", c.env...)
		if (code == 0) != c.pass || !strings.Contains(out, c.want) {
			t.Errorf("%s: exit %d, want pass=%v and %q\n%s", c.name, code, c.pass, c.want, out)
		}
	}
	dir := repo(t, start)
	git(t, dir, "switch", "-q", "-c", "work")
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": vitest(bin)})
	if code, out := script(t, dir, bin, "web-tdd.sh"); code != 0 || !strings.Contains(out, "no app code changed") {
		t.Errorf("an unchanged branch: exit %d\n%s", code, out)
	}
	// The leaf is left as it was, and vitest was given the new test files only.
	dir = repo(t, start)
	work(t, dir, newCode)
	bin = t.TempDir()
	webTools(t, bin, map[string]string{"vitest": vitest(bin)})
	script(t, dir, bin, "web-tdd.sh")
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("the check left the leaf changed:\n%s", st)
	}
	if got := calls(bin); got != "npm ci --no-audit --no-fund\nvitest run src/lib/greet.test.ts\n" {
		t.Errorf("vitest was run as %q, want only the new test file", got)
	}
}

func TestTDDDoesNotTrustAVitestTheAgentLeftInNodeModules(t *testing.T) {
	// The planted vitest fails whatever it runs, which would make "the new tests fail on the parent" true.
	// The real one, installed clean, passes a test of nothing new, so the check must refuse.
	start := map[string]string{".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n"}
	dir := repo(t, start)
	work(t, dir, map[string]string{"src/b.test.ts": "// test\n"})
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	put(t, dir, map[string]string{"node_modules/.bin/vitest": "#!/bin/sh\necho TAMPERED\nexit 1\n"})
	if err := os.Chmod(filepath.Join(dir, "node_modules/.bin/vitest"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := script(t, dir, bin, "web-tdd.sh")
	if code == 0 || strings.Contains(out, "TAMPERED") || !strings.Contains(out, "test nothing new") {
		t.Errorf("a planted vitest: exit %d\n%s", code, out)
	}
}

func TestTDDSeesAFileThatWasRenamed(t *testing.T) {
	body := "export const a = 1;\nexport const b = 2;\nexport const c = 3;\nexport const d = 4;\n"
	start := map[string]string{".gitignore": "node_modules/\n", "src/server.ts": body, "src/server.test.ts": "// test " + body}
	dir := repo(t, start)
	// Moved to a new name with a new function in it, git shows it as a rename.
	git(t, dir, "switch", "-q", "-c", "work")
	git(t, dir, "mv", "src/server.ts", "src/routes.ts")
	put(t, dir, map[string]string{"src/routes.ts": body + "export const e = 5;\n"})
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "work")
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	if code, out := script(t, dir, bin, "web-tdd.sh"); code == 0 || !strings.Contains(out, "src/routes.ts") {
		t.Errorf("a renamed file with new code: exit %d\n%s", code, out)
	}
	// A renamed test file is a changed test: it is run on the parent.
	dir = repo(t, start)
	git(t, dir, "switch", "-q", "-c", "work")
	git(t, dir, "mv", "src/server.test.ts", "src/routes.test.ts")
	git(t, dir, "commit", "-q", "-m", "work")
	bin = t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	if code, out := script(t, dir, bin, "web-tdd.sh", "STUB_VITEST_EXIT=1"); code != 0 || !strings.Contains(calls(bin), "vitest run src/routes.test.ts") {
		t.Errorf("a renamed test: exit %d, calls %q\n%s", code, calls(bin), out)
	}
}
