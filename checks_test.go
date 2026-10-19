package midgard

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	want := "npm ci --ignore-scripts --no-audit --no-fund\neslint --no-config-lookup -c .ygg-eslint.config.js .\ntsc --noEmit -p .ygg-tsconfig.json\ntsc --noEmit -p .ygg-tsconfig.json\nvite build --config .ygg-vite.config.ts\n"
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
	if code == 0 || strings.Contains(out, "TAMPERED") || !strings.HasPrefix(calls(bin), "npm ci --ignore-scripts --no-audit --no-fund\n") {
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

func TestStaticRefusesAChangeToWhatIsNotAToolConfigButTheChecksRelyOn(t *testing.T) {
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
		{"a script of package.json", map[string]string{"package.json": pkg(`"lint": "eslint . || true"`, `"react": "1.0.0"`)}, "package.json"},
		{"a new script", map[string]string{"package.json": pkg(`"lint": "eslint .", "test": "true"`, `"react": "1.0.0"`)}, "package.json"},
		{"a leaf .npmrc", map[string]string{".npmrc": "script-shell=./sh\n"}, ".npmrc"},
		{"the dependencies of package.json", map[string]string{"package.json": pkg(`"lint": "eslint ."`, `"react": "1.0.0", "zod": "3.0.0"`)}, ""},
		{"app code only", map[string]string{"src/a.ts": "export const a = 2;\n"}, ""},
		// The tool configs are not on the list any more: the rungs run the configs baked into the image and
		// never read these, so a change to them changes nothing (see TestRealRungOne...).
		{"eslint config", map[string]string{"eslint.config.js": "export default [{ignores: ['**']}];\n"}, ""},
		{"a stray eslint config", map[string]string{"src/eslint.config.js": "export default [{ignores: ['**']}];\n"}, ""},
		{"tsconfig", map[string]string{"tsconfig.json": "{\"include\": []}\n"}, ""},
		{"vite config", map[string]string{"vite.config.ts": "export default {test: {passWithNoTests: true}};\n"}, ""},
		{"playwright config", map[string]string{"playwright.config.ts": "export default {retries: 9};\n"}, ""},
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
	// A change not committed yet counts, an ignored file too, and so does a changed script after a deleted one.
	dir := repo(t, start)
	put(t, dir, map[string]string{"package.json": pkg(`"lint": "true"`, `"react": "1.0.0"`)})
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"eslint": "", "tsc": "", "vite": ""})
	if code, out := script(t, dir, bin, "web-static.sh"); code == 0 || !strings.Contains(out, "package.json") {
		t.Errorf("an uncommitted change: exit %d\n%s", code, out)
	}
	dir = repo(t, start)
	git(t, dir, "switch", "-q", "-c", "work")
	put(t, dir, map[string]string{".gitignore": "node_modules/\n.npmrc\n", ".npmrc": "script-shell=./sh\n"})
	if code, out := script(t, dir, bin, "web-static.sh"); code == 0 || !strings.Contains(out, ".npmrc") {
		t.Errorf("an ignored .npmrc: exit %d\n%s", code, out)
	}
	// With no main to compare with the check cannot tell, and refuses.
	if code, out := script(t, leaf(t, start), bin, "web-static.sh"); code == 0 || !strings.Contains(out, "no main") {
		t.Errorf("no main: exit %d\n%s", code, out)
	}
}

func TestUnitInstallsCleanAndRunsVitestByPath(t *testing.T) {
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	dir := repo(t, map[string]string{"package.json": "{}"})
	tamper(t, dir, "vitest")
	code, out := script(t, dir, bin, "web-unit.sh", "STUB_VITEST_EXIT=1")
	if code == 0 || strings.Contains(out, "TAMPERED") {
		t.Errorf("a planted vitest was trusted: exit %d\n%s", code, out)
	}
	bin = t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	if code, _ := script(t, repo(t, map[string]string{"package.json": "{}"}), bin, "web-unit.sh"); code != 0 || calls(bin) != "npm ci --ignore-scripts --no-audit --no-fund\nvitest run --config .ygg-vite.config.ts\n" {
		t.Errorf("calls = %q", calls(bin))
	}
}

func TestTDDWantsTheNewTestsToFailOnTheCommitTheTaskStartedFrom(t *testing.T) {
	start := map[string]string{"package.json": "{}", "node_modules/.keep": "", ".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n", "src/a.test.ts": "// test\n"}
	// vitest stands in for the real one, installed by "npm ci" into node_modules/.bin: it fails on the parent
	// when the parent lacks the code the new test needs, as a real run does, and logs where it ran and what
	// was there.
	vitest := func(bin string) string {
		return `echo "vitest $*" >> "` + bin + `/calls.log"
[ -e node_modules/.keep ] || { echo "no packages beside the parent"; exit 3; }
cmp -s .ygg-vite.config.ts "` + bakedPath("vite.config.ts") + `" || { echo "the parent has no fresh baked vite config"; exit 4; }
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
	if got := calls(bin); got != "npm ci --ignore-scripts --no-audit --no-fund\nvitest run --config .ygg-vite.config.ts src/lib/greet.test.ts\n" {
		t.Errorf("vitest was run as %q, want only the new test file", got)
	}
}

func TestTDDDoesNotTrustAVitestTheAgentLeftInNodeModules(t *testing.T) {
	// The planted vitest fails whatever it runs, which would make "the new tests fail on the parent" true.
	// The real one, installed clean, passes a test of nothing new, so the check must refuse.
	start := map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n"}
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
	start := map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n", "src/server.ts": body, "src/server.test.ts": "// test " + body}
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
	if code, out := script(t, dir, bin, "web-tdd.sh", "STUB_VITEST_EXIT=1"); code != 0 || !strings.Contains(calls(bin), "vitest run --config .ygg-vite.config.ts src/routes.test.ts") {
		t.Errorf("a renamed test: exit %d, calls %q\n%s", code, calls(bin), out)
	}
}

// Every rung of the web ladder refuses a change to the scripts of package.json and a leaf .npmrc before it
// installs or runs anything, so a rung run alone is protected too: a leaf .npmrc changes how npm runs the
// scripts of the packages. The tool configs are not here: the rungs run their own baked copies.
func TestEveryWebRungRefusesAChangeToTheScriptsAndNpmrc(t *testing.T) {
	start := map[string]string{".gitignore": "node_modules/\n.npmrc.local\n", "package.json": `{"scripts": {"lint": "eslint ."}}`, "contracts/api.openapi.yaml": "x",
		"eslint.config.js": "export default [];\n", "tsconfig.json": "{}\n", "vite.config.ts": "export default {};\n", "playwright.config.ts": "export default {};\n", "src/a.ts": "export const a = 1;\n"}
	rungs := []string{"web-static.sh", "web-unit.sh", "web-tdd.sh", "web-contract.sh", "web-deps.sh", "web-security.sh", "web-e2e.sh"}
	tools := map[string]string{"eslint": "", "tsc": "", "vite": "", "vitest": "", "prism": "", "playwright": ""}
	for _, rung := range rungs {
		for name, files := range map[string]map[string]string{
			".npmrc":                     {".npmrc": "x\n"},
			"package.json":               {"package.json": `{"scripts": {"lint": "true"}}`},
			"an added script":            {"package.json": `{"scripts": {"lint": "eslint .", "pretest": "true"}}`},
			"package.json (the scripts)": {"package.json": `{"scripts": {}}`},
		} {
			dir := repo(t, start)
			work(t, dir, files)
			bin := t.TempDir()
			webTools(t, bin, tools)
			code, out := script(t, dir, bin, rung, "YGG_E2E_WAIT=1")
			if code == 0 || !strings.Contains(out, "protected") || calls(bin) != "" {
				t.Errorf("%s with %s: exit %d, calls %q\n%s", rung, name, code, calls(bin), out)
			}
		}
		// Added but not committed, and ignored by git, both count.
		dir := repo(t, start)
		git(t, dir, "switch", "-q", "-c", "work")
		put(t, dir, map[string]string{".npmrc": "script-shell=./sh\n"})
		bin := t.TempDir()
		webTools(t, bin, tools)
		if code, out := script(t, dir, bin, rung, "YGG_E2E_WAIT=1"); code == 0 || !strings.Contains(out, ".npmrc") || calls(bin) != "" {
			t.Errorf("%s with an .npmrc not committed: exit %d\n%s", rung, code, out)
		}
		dir = repo(t, start)
		git(t, dir, "switch", "-q", "-c", "work")
		put(t, dir, map[string]string{".gitignore": "node_modules/\n.npmrc\n", ".npmrc": "script-shell=./sh\n"})
		if code, out := script(t, dir, bin, rung, "YGG_E2E_WAIT=1"); code == 0 || !strings.Contains(out, ".npmrc") || calls(bin) != "" {
			t.Errorf("%s with an ignored .npmrc: exit %d\n%s", rung, code, out)
		}
	}
}

// A package may run a script when npm installs it (preinstall, install, postinstall, prepare), and a package
// the agent adds can be its own. Every install of the ladder and of the acceptance run leaves them out; the
// tools the ladder needs ship their binaries in packages and do not need them.
func TestEveryNpmInstallOfTheChecksIgnoresScripts(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("devcontainer", "checks", "*.sh"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scripts: %v", err)
	}
	files = append(files, filepath.Join("templates", "web", "init.sh"))
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if l := strings.TrimSpace(line); strings.HasPrefix(l, "#") || strings.HasPrefix(l, "echo ") {
				continue
			}
			if (strings.Contains(line, "npm ci") || strings.Contains(line, "npm install")) && !strings.Contains(line, "--ignore-scripts") {
				t.Errorf("%s:%d installs packages without --ignore-scripts: %s", f, i+1, line)
			}
		}
	}
}

// bakedPath is where the tool configs the rungs run are kept in the source of the image: the Containerfile
// copies devcontainer/checks to /opt/ygg/checks, where only root can write.
func bakedPath(name string) string {
	abs, err := filepath.Abs(filepath.Join("devcontainer", "checks", "config", name))
	if err != nil {
		panic(err)
	}
	return abs
}

var bakedConfigs = []string{"eslint.config.js", "tsconfig.json", "vite.config.ts", "playwright.config.ts"}

// The baked configs are the one source of truth. The template ships identical copies so a leaf, an editor
// and `npm run` behave as the ladder does; a copy that drifts is a change nobody reviewed.
func TestTheTemplatesToolConfigsAreTheBakedOnes(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("devcontainer", "checks", "config"))
	if err != nil {
		t.Fatal(err)
	}
	// deps-policy.json is data for web-deps.sh, not a tool config (see deps_test.go).
	if len(entries) != len(bakedConfigs)+1 {
		t.Errorf("devcontainer/checks/config has %d files, want the %d tool configs %v and deps-policy.json", len(entries), len(bakedConfigs), bakedConfigs)
	}
	for _, name := range bakedConfigs {
		baked, err := os.ReadFile(bakedPath(name))
		if err != nil {
			t.Errorf("no baked %s: %v", name, err)
			continue
		}
		tpl, err := os.ReadFile(filepath.Join("templates", "web", name))
		if err != nil {
			t.Errorf("the template has no %s: %v", name, err)
			continue
		}
		if !bytes.Equal(baked, tpl) {
			t.Errorf("templates/web/%s differs from devcontainer/checks/config/%s; the baked one is the source, copy it over the template's", name, name)
		}
	}
}

// No rung calls a tool that reads a config without naming the baked copy it installed: a call with no flag
// would look for a config in the leaf, which the agent writes.
func TestEveryToolCallOfTheRungsNamesItsConfig(t *testing.T) {
	flag := map[string]string{
		"eslint":     "--no-config-lookup -c .ygg-eslint.config.js",
		"tsc":        "-p .ygg-tsconfig.json",
		"vite":       "--config .ygg-vite.config.ts",
		"vitest":     "--config .ygg-vite.config.ts",
		"playwright": "--config .ygg-playwright.config.ts",
	}
	files, _ := filepath.Glob(filepath.Join("devcontainer", "checks", "*.sh"))
	call := regexp.MustCompile(`node_modules/\.bin/(eslint|tsc|vite|vitest|playwright)\b`)
	seen := map[string]bool{}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			// A line may chain calls with && or ;.
			for _, part := range regexp.MustCompile(`&&|;|\|\|`).Split(line, -1) {
				for _, m := range call.FindAllStringSubmatch(part, -1) {
					seen[m[1]] = true
					if !strings.Contains(part, flag[m[1]]) {
						t.Errorf("%s:%d calls %s without %q: %s", f, i+1, m[1], flag[m[1]], strings.TrimSpace(part))
					}
				}
			}
		}
	}
	for tool := range flag {
		if !seen[tool] {
			t.Errorf("no rung calls %s by path; the test's list of tools is out of date", tool)
		}
	}
}

// The baked configs go into the leaf after npm ci, as fresh files, whatever the agent left at their names:
// a symlink, a folder, or a file. They are gone again when the rung ends.
func TestRungsInstallTheBakedConfigsFreshAfterTheInstallAndRemoveThem(t *testing.T) {
	probe := func(name string) string { // a tool that logs whether its config is the baked file, a plain one
		return `if cmp -s ` + name + ` "` + bakedPath(strings.TrimPrefix(name, ".ygg-")) + `" && [ -f ` + name + ` ] && [ ! -L ` + name + ` ]; then r=fresh; else r=STALE; fi
echo "$0 ` + name + ` $r" >> "$LOG"
`
	}
	cases := []struct {
		rung  string
		tools map[string]string
		want  []string
	}{
		{"web-static.sh", map[string]string{"eslint": probe(".ygg-eslint.config.js"), "tsc": probe(".ygg-tsconfig.json"), "vite": probe(".ygg-vite.config.ts")},
			[]string{".ygg-eslint.config.js fresh", ".ygg-tsconfig.json fresh", ".ygg-vite.config.ts fresh"}},
		{"web-unit.sh", map[string]string{"vitest": probe(".ygg-vite.config.ts")}, []string{".ygg-vite.config.ts fresh"}},
	}
	for _, c := range cases {
		dir := repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n.ygg-*\n"})
		outside := t.TempDir()
		put(t, outside, map[string]string{"evil.js": "export default [{ignores: ['**']}];\n"})
		for name, how := range map[string]string{".ygg-eslint.config.js": "link", ".ygg-tsconfig.json": "dir", ".ygg-vite.config.ts": "file"} {
			p := filepath.Join(dir, name)
			switch how {
			case "link":
				if err := os.Symlink(filepath.Join(outside, "evil.js"), p); err != nil {
					t.Fatal(err)
				}
			case "dir":
				put(t, dir, map[string]string{name + "/x": "x"})
			default:
				put(t, dir, map[string]string{name: "export default {};\n"})
			}
		}
		bin := t.TempDir()
		webTools(t, bin, c.tools)
		code, out := script(t, dir, bin, c.rung, "LOG="+filepath.Join(bin, "probe.log"))
		log, _ := os.ReadFile(filepath.Join(bin, "probe.log"))
		if code != 0 {
			t.Errorf("%s: exit %d\n%s", c.rung, code, out)
		}
		for _, w := range c.want {
			if !strings.Contains(string(log), w) {
				t.Errorf("%s: want %q in\n%s", c.rung, w, log)
			}
		}
		if strings.Contains(string(log), "STALE") {
			t.Errorf("%s ran a tool with a config that is not the baked one:\n%s", c.rung, log)
		}
		if b, err := os.ReadFile(filepath.Join(outside, "evil.js")); err != nil || !strings.Contains(string(b), "ignores") {
			t.Errorf("%s wrote through the planted symlink: %v %q", c.rung, err, b)
		}
		for _, name := range bakedConfigs {
			if _, err := os.Lstat(filepath.Join(dir, ".ygg-"+name)); err == nil {
				t.Errorf("%s left .ygg-%s in the leaf", c.rung, name)
			}
		}
	}
}

func TestTDDRunsTheParentWithTheBakedViteConfigAndLeavesTheLeafClean(t *testing.T) {
	// web-tdd's vitest stand-in (TestTDDWantsTheNewTests...) fails with exit 4 when the parent lacks the baked
	// config, so a pass there covers it. Here the leaf's own checks: nothing is left behind.
	dir := repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n", "src/a.ts": "export const a = 1;\n"})
	work(t, dir, map[string]string{"src/b.test.ts": "// test\n"})
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": "echo \"$*\" >> \"" + bin + "/calls.log\"\nexit 1\n"})
	if code, out := script(t, dir, bin, "web-tdd.sh"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Errorf("the check left the leaf changed:\n%s", st)
	}
}

// realLeaf copies the web template (without installed packages or a build) into a temp dir, commits it on
// main, and puts files over it. The tests below run the real rung with the real tools, so they need the
// network for npm ci (or a warm npm cache); they skip when it is not there.
func realLeaf(t *testing.T, over map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm is not installed")
	}
	dir := t.TempDir()
	src := filepath.Join("templates", "web")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "start")
	git(t, dir, "switch", "-q", "-c", "work")
	put(t, dir, over)
	return dir
}

func skipOffline(t *testing.T, out string) {
	t.Helper()
	for _, s := range []string{"ENOTFOUND", "EAI_AGAIN", "ECONNREFUSED", "ETIMEDOUT", "ENETUNREACH"} {
		if strings.Contains(out, s) {
			t.Skip("npm ci needs the network: " + s)
		}
	}
}

// The unchanged template passes rung 1 and the unit rung with the baked configs, so the move from
// the leaf's configs to the baked ones changes nothing for a leaf that does not tamper.
func TestRealRungsPassOnTheUnchangedTemplate(t *testing.T) {
	for _, rung := range []string{"web-static.sh", "web-unit.sh"} {
		dir := realLeaf(t, nil)
		code, out := script(t, dir, "", rung)
		skipOffline(t, out)
		if code != 0 {
			t.Errorf("%s on the unchanged template: exit %d\n%s", rung, code, out)
		}
		if st := git(t, dir, "status", "--porcelain"); st != "" {
			t.Errorf("%s left the leaf changed:\n%s", rung, st)
		}
	}
}

// The bypasses of B-049: each tampers with the config the linter, the type-checker or the test runner would
// read, to turn a rung green over code that fails it. The baked config is the one that runs, so each fails.
func TestRealRungsIgnoreAnyConfigTheLeafHas(t *testing.T) {
	silence := "export default [{ignores: ['**']}];\n"
	unused := map[string]string{"src/bad.ts": "const unused = 1;\n"} // a lint error
	merge := func(ms ...map[string]string) map[string]string {
		out := map[string]string{}
		for _, m := range ms {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	cases := []struct {
		name  string
		rung  string
		files map[string]string
		link  [2]string // a symlink to make: name, target
		want  string
	}{
		{"an eslint config added in src/", "web-static.sh", merge(unused, map[string]string{"src/eslint.config.js": silence}), [2]string{}, "no-unused-vars"},
		{"an eslint config added as .mjs", "web-static.sh", merge(unused, map[string]string{"eslint.config.mjs": silence}), [2]string{}, "no-unused-vars"},
		{"the eslint config replaced by a symlink", "web-static.sh", merge(unused, map[string]string{"silence.js": silence}), [2]string{"eslint.config.js", "silence.js"}, "no-unused-vars"},
		{"an eslint config hidden by .gitignore", "web-static.sh", merge(unused, map[string]string{"src/eslint.config.js": silence, ".gitignore": "node_modules/\ndist/\nsrc/eslint.config.js\n"}), [2]string{}, "no-unused-vars"},
		{"the eslint config edited", "web-static.sh", merge(unused, map[string]string{"eslint.config.js": silence}), [2]string{}, "no-unused-vars"},
		{"the eslint config planted where the rung puts its own", "web-static.sh", merge(unused, map[string]string{".ygg-eslint.config.js": silence}), [2]string{}, "no-unused-vars"},
		{"the planted one a symlink", "web-static.sh", merge(unused, map[string]string{"silence.js": silence}), [2]string{".ygg-eslint.config.js", "silence.js"}, "no-unused-vars"},
		{"a tsconfig that includes nothing", "web-static.sh", map[string]string{"tsconfig.json": "{\"include\": []}\n", "src/types.ts": "export const n: number = \"s\";\n"}, [2]string{}, "TS2322"},
		{"a vite config that runs no test", "web-unit.sh", map[string]string{"vite.config.ts": "export default {test: {include: []}};\n", "src/fail.test.ts": "import {expect, it} from 'vitest';\nit('fails', () => expect(1).toBe(2));\n"}, [2]string{}, "fails"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := realLeaf(t, c.files)
			if c.link[0] != "" {
				if err := os.Remove(filepath.Join(dir, c.link[0])); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if err := os.Symlink(c.link[1], filepath.Join(dir, c.link[0])); err != nil {
					t.Fatal(err)
				}
			}
			code, out := script(t, dir, "", c.rung)
			skipOffline(t, out)
			if code == 0 || !strings.Contains(out, c.want) {
				t.Errorf("%s: exit %d, want a failure that says %q\n%s", c.name, code, c.want, out)
			}
		})
	}
}

// Every rung of the web ladder refuses a task that adds, changes or deletes a file of the leaf's dev container
// or of its own CI, before it installs or runs anything: the dev container is what a person's editor runs, and
// the workflow is the product's. Only the product's people change either.
func TestEveryRungRefusesADevContainerOrWorkflowChange(t *testing.T) {
	base := map[string]string{".gitignore": "node_modules/\n", "package.json": `{"scripts": {"lint": "eslint ."}}`, "contracts/api.openapi.yaml": "x",
		"eslint.config.js": "export default [];\n", "tsconfig.json": "{}\n", "vite.config.ts": "export default {};\n", "playwright.config.ts": "export default {};\n", "src/a.ts": "export const a = 1;\n"}
	tools := map[string]string{"eslint": "", "tsc": "", "vite": "", "vitest": "", "prism": "", "playwright": ""}
	for _, p := range []struct{ file, other string }{
		{".devcontainer/devcontainer.json", ".devcontainer/Dockerfile"},
		{".github/workflows/ci.yml", ".github/workflows/deploy.yml"},
	} {
		with := map[string]string{p.file: "{}\n"}
		for k, v := range base {
			with[k] = v
		}
		ignored := map[string]string{".gitignore": "node_modules/\n" + p.file + "\n", p.file: "{}\n"}
		cases := []struct {
			name   string
			start  map[string]string
			change func(dir string)
		}{
			{"added", base, func(dir string) { work(t, dir, map[string]string{p.file: "{}\n"}) }},
			{"changed", with, func(dir string) { work(t, dir, map[string]string{p.file: "{\"x\": 1}\n"}) }},
			{"another file beside it", with, func(dir string) { work(t, dir, map[string]string{p.other: "x\n"}) }},
			{"deleted", with, func(dir string) {
				git(t, dir, "switch", "-q", "-c", "work")
				git(t, dir, "rm", "-q", p.file)
				git(t, dir, "commit", "-q", "-m", "work")
			}},
			{"written, not committed", base, func(dir string) { put(t, dir, map[string]string{p.file: "{}\n"}) }},
			{"deleted, not committed", with, func(dir string) {
				if err := os.Remove(filepath.Join(dir, p.file)); err != nil {
					t.Fatal(err)
				}
			}},
			{"ignored by git", base, func(dir string) { put(t, dir, ignored) }},
		}
		for _, rung := range []string{"web-static.sh", "web-unit.sh", "web-tdd.sh", "web-contract.sh", "web-deps.sh", "web-security.sh", "web-e2e.sh"} {
			for _, c := range cases {
				dir := repo(t, c.start)
				c.change(dir)
				bin := t.TempDir()
				webTools(t, bin, tools)
				want := p.file
				if c.name == "another file beside it" {
					want = p.other
				}
				if code, out := script(t, dir, bin, rung, "YGG_E2E_WAIT=1"); code == 0 || !strings.Contains(out, "protected") || !strings.Contains(out, want) || calls(bin) != "" {
					t.Errorf("%s with %s %s: exit %d, calls %q\n%s", rung, p.file, c.name, code, calls(bin), out)
				}
			}
		}
	}
}
