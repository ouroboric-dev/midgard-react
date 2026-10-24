package midgard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ouroboric-dev/gungnir/schema"
)

// Every check of the web ladder writes a typed result, ygg.check/v1, at $YGG_CHECK_RESULT (change typed-boundaries,
// T27). These tests run each check script on a small leaf, a passing one and a failing one (testdata/checks/<check>/
// <case>/leaf, with an optional work/ laid over it on a branch), with the real converter, the work image's
// ygg-check-result, read from gleipnir at the version below. The tools of the image are stand-ins that print what the
// real tool printed (testdata/checks/<check>/<case>/rec: the machine output gleipnir recorded from the real tools for
// the converter's own tests) and exit as it did. Only git, node (the converter, the dependency policy) and docs.sh
// are real.

// gleipnirPin is the gleipnir that holds the converter (gleipnir#22), read from the module cache; the realm does not
// require gleipnir.
const gleipnirPin = "github.com/ouroboric-dev/gleipnir@v0.0.0-20261024192006-9da064f0b1ee"

// converterBin returns a folder with the command ygg-check-result, as the work image has it.
func converterBin(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Fatal("node is needed: the converter runs under it")
	}
	cmd := exec.Command("go", "mod", "download", "-json", gleipnirPin)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go mod download %s: %v", gleipnirPin, err)
	}
	var dir string
	for _, l := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), `"Dir": "`); ok && k == "" {
			dir = strings.TrimSuffix(v, `",`)
		}
	}
	mjs := filepath.Join(dir, "sandbox", "assets", "ygg-check-result.mjs")
	if _, err := os.Stat(mjs); err != nil {
		t.Fatalf("the converter is not at %s: %v", mjs, err)
	}
	bin := t.TempDir()
	stub(t, bin, "ygg-check-result", "exec node '"+mjs+"' \"$@\"\n")
	return bin
}

// fakeTool is every tool of the image the checks call, as one script that each tool's name is a link to. It answers
// from the recordings in $YGG_REC: <tool>.json is the machine output (written to the file the call names, or to
// stdout when the call asks for json), <tool>.txt what it prints otherwise, <tool>.exit its exit code.
const fakeTool = `t=$(basename "$0")
rec="$YGG_REC"
if [ "$t" = npm ] && [ "$1" = ci ]; then
  rm -rf node_modules; mkdir -p node_modules/.bin
  for x in eslint tsc vite vitest playwright openapi-typescript; do ln -s "$YGG_FAKE" node_modules/.bin/$x; done
  exit 0
fi
port=""; out=""; machine=""; prev=""
for a in "$@"; do
  case "$a" in
    --outputFile.json=*) out="${a#*=}"; machine=1 ;;
    --json) machine=1 ;;
    --reporter=*json*) out="$PLAYWRIGHT_JSON_OUTPUT_NAME"; machine=1 ;;
  esac
  case "$prev" in
    -o) out="$a"; [ "$t" = openapi-typescript ] || machine=1 ;;
    --format) [ "$a" = json ] && machine=1 ;;
    --port) port="$a" ;;
  esac
  prev="$a"
done
echo "$t $*" >> "$YGG_REC_CALLS"
if [ "$t" = prism ] || { [ "$t" = vite ] && [ "$1" = preview ]; }; then
  exec python3 -m http.server "$port" --bind 127.0.0.1 >/dev/null 2>&1
fi
if [ "$t" = openapi-typescript ]; then cp "$rec/schema.d.ts" "$out"; exit 0; fi
if [ -n "$machine" ]; then
  if [ -z "$out" ]; then cat "$rec/$t.json" 2>/dev/null; else cat "$rec/$t.json" > "$out" 2>/dev/null; fi
else
  cat "$rec/$t.txt" 2>/dev/null
fi
exit "$(cat "$rec/$t.exit" 2>/dev/null || echo 0)"
`

type typedCase struct {
	check, kase string
	pass        bool
	located     bool   // a failure holds the tool's test id or location
	names       string // else: a failure's message holds this, the most specific thing the converter's output has
	env         func(t *testing.T) []string
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		files[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestEveryCheckWritesATypedResult(t *testing.T) {
	needPython(t)
	conv := converterBin(t)
	ports := func(t *testing.T) []string {
		return []string{"YGG_MOCK_PORT=" + strconv.Itoa(freePort(t)), "YGG_APP_PORT=" + strconv.Itoa(freePort(t)), "YGG_API_PORT=" + strconv.Itoa(freePort(t)), "YGG_E2E_WAIT=5"}
	}
	accept := func(t *testing.T) []string { return append(ports(t), "YGG_WEB_DIR=.") }
	apiBreaks := func(t *testing.T) []string { return append(ports(t), "YGG_WEB_DIR=.", "YGG_API_DIR=api") }
	// names is what the converter's output holds when the tool gives no test id or location: osv and npm audit name the
	// package, oasdiff the path, vacuum the contract file, and a tdd refusal the file.
	cases := []typedCase{
		{"web-static", "pass", true, false, "", nil}, {"web-static", "fail", false, true, "", nil},
		{"web-unit", "pass", true, false, "", nil}, {"web-unit", "fail", false, true, "", nil},
		{"web-tdd", "pass", true, false, "", nil}, {"web-tdd", "fail", false, false, "src/b.ts", nil},
		{"web-contract", "pass", true, false, "", nil}, {"web-contract", "fail", false, false, "contracts/api.openapi.yaml", nil},
		{"web-deps", "pass", true, false, "", nil}, {"web-deps", "fail", false, false, "golang.org/x/text", nil}, {"web-deps", "unpinned", false, false, "react", nil},
		{"web-security", "pass", true, false, "", nil}, {"web-security", "fail", false, false, "lodash", nil},
		{"web-e2e", "pass", true, false, "", ports}, {"web-e2e", "fail", false, true, "", ports},
		{"accept-e2e", "pass", true, false, "", accept}, {"accept-e2e", "fail", false, true, "", accept},
		{"accept-e2e", "api-breaks", false, false, "/pets", apiBreaks},
	}
	for _, c := range cases {
		t.Run(c.check+"/"+c.kase, func(t *testing.T) {
			root := filepath.Join("testdata", "checks", c.check, c.kase)
			files := readTree(t, filepath.Join(root, "leaf"))
			if api, err := os.Stat(filepath.Join(root, "api")); err == nil && api.IsDir() {
				for k, v := range readTree(t, filepath.Join(root, "api")) {
					files[filepath.Join("api", k)] = v
				}
			}
			dir := repo(t, files)
			if _, err := os.Stat(filepath.Join(root, "work")); err == nil {
				work(t, dir, readTree(t, filepath.Join(root, "work")))
			}
			bin := t.TempDir()
			fake := filepath.Join(bin, "fake")
			stub(t, bin, "fake", fakeTool)
			for _, tool := range []string{"npm", "vacuum", "oasdiff", "osv-scanner", "prism"} {
				if err := os.Symlink(fake, filepath.Join(bin, tool)); err != nil {
					t.Fatal(err)
				}
			}
			rec, _ := filepath.Abs(filepath.Join(root, "rec"))
			result := filepath.Join(t.TempDir(), "result.json")
			env := []string{"YGG_CHECK_RESULT=" + result, "YGG_REC=" + rec, "YGG_FAKE=" + fake, "YGG_REC_CALLS=" + filepath.Join(bin, "calls.log"),
				"PATH=" + conv + ":" + bin + ":" + os.Getenv("PATH")}
			if c.env != nil {
				env = append(env, c.env(t)...)
			}
			code, out := script(t, dir, "", c.check+".sh", env...)
			if (code == 0) != c.pass {
				t.Fatalf("exit %d, want pass=%v\n%s", code, c.pass, out)
			}
			b, err := os.ReadFile(result)
			if err != nil {
				t.Fatalf("no result at $YGG_CHECK_RESULT: %v\n%s", err, out)
			}
			r, err := schema.Decode[schema.CheckResult](schema.ContractCheck, b)
			if err != nil {
				t.Fatalf("the result is not a valid ygg.check/v1: %v\n%s", err, b)
			}
			if r.Check != c.check || r.Passed != c.pass || (r.ExitCode == 0) != c.pass {
				t.Errorf("result = check %q passed %v exit %d, want %q passed %v with the script's exit %d\n%s", r.Check, r.Passed, r.ExitCode, c.check, c.pass, code, b)
			}
			if r.ExitCode != code {
				t.Errorf("exit_code = %d, the script exited %d", r.ExitCode, code)
			}
			if c.pass {
				return
			}
			if len(r.Failures) == 0 {
				t.Fatalf("a failing check with no failure:\n%s", b)
			}
			located := false
			for _, f := range r.Failures {
				located = located || f.Test != "" || f.Location != ""
			}
			if c.located && !located {
				t.Errorf("no failure holds a test id or a location:\n%s", b)
			}
			named := false
			for _, f := range r.Failures {
				named = named || strings.Contains(f.Message, c.names) || strings.Contains(f.Test, c.names) || strings.Contains(f.Location, c.names)
			}
			if !c.located && !named {
				t.Errorf("no failure names %q:\n%s", c.names, b)
			}
		})
	}
}

// With no YGG_CHECK_RESULT (a local run, a command Odin runs untyped) a check writes nothing and behaves as it did.
func TestACheckWritesNoResultWithoutTheVariable(t *testing.T) {
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	dir := repo(t, map[string]string{"package.json": "{}"})
	env := []string{"YGG_CHECK_RESULT="}
	if code, out := script(t, dir, bin, "web-unit.sh", env...); code != 0 || calls(bin) != "npm ci --ignore-scripts --no-audit --no-fund\nvitest run --config .ygg-vite.config.ts\n" {
		t.Errorf("exit %d, calls %q\n%s", code, calls(bin), out)
	}
}

// A refusal of web-protected.sh is a policy failure, one for each file, whether the script runs on its own or as the
// first step of a rung.
func TestAProtectedFileChangeIsAPolicyFailureOfEachRung(t *testing.T) {
	conv := converterBin(t)
	for _, name := range []string{"web-protected.sh", "web-unit.sh", "web-security.sh", "web-deps.sh"} {
		t.Run(name, func(t *testing.T) {
			dir := repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n"})
			work(t, dir, map[string]string{".npmrc": "ignore-scripts=false\n", "tsconfig.json": "{}"})
			bin := t.TempDir()
			webTools(t, bin, map[string]string{"vitest": ""})
			result := filepath.Join(t.TempDir(), "r.json")
			code, out := script(t, dir, bin, name, "YGG_CHECK_RESULT="+result, "PATH="+conv+":"+bin+":"+os.Getenv("PATH"))
			if code == 0 {
				t.Fatalf("passed\n%s", out)
			}
			b, err := os.ReadFile(result)
			if err != nil {
				t.Fatalf("no result: %v\n%s", err, out)
			}
			r, err := schema.Decode[schema.CheckResult](schema.ContractCheck, b)
			if err != nil {
				t.Fatalf("%v\n%s", err, b)
			}
			var n int
			for _, f := range r.Failures {
				if f.Kind == "policy" && (strings.Contains(f.Message, ".npmrc") || strings.Contains(f.Message, "tsconfig.json")) {
					n++
				}
			}
			if r.Passed || n != 2 {
				t.Errorf("want two policy failures, one for each file:\n%s", b)
			}
		})
	}
}

// A step after npm ci that fails is reported as itself, not as the install that came before it.
func TestAFailedConfigInstallIsNotReportedAsNpmCi(t *testing.T) {
	conv := converterBin(t)
	bin := t.TempDir()
	webTools(t, bin, map[string]string{"vitest": ""})
	stub(t, bin, "install", "echo 'install: cannot create' >&2\nexit 1\n")
	dir := repo(t, map[string]string{"package.json": "{}", ".gitignore": "node_modules/\n"})
	result := filepath.Join(t.TempDir(), "r.json")
	code, out := script(t, dir, bin, "web-unit.sh", "YGG_CHECK_RESULT="+result, "PATH="+conv+":"+bin+":"+os.Getenv("PATH"))
	if code == 0 {
		t.Fatalf("passed\n%s", out)
	}
	b, err := os.ReadFile(result)
	if err != nil {
		t.Fatalf("no result: %v\n%s", err, out)
	}
	r, err := schema.Decode[schema.CheckResult](schema.ContractCheck, b)
	if err != nil || len(r.Failures) == 0 {
		t.Fatalf("%v\n%s", err, b)
	}
	if m := r.Failures[0].Message; strings.Contains(m, "npm ci") || !strings.Contains(m, "tool configs") {
		t.Errorf("the failure says %q, want the config install", m)
	}
}
