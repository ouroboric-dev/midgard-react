package midgard

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serve is a stub body that serves HTTP on the port after the option named by flag, until it is killed, and
// logs that it started. It needs python3, as the tests of the e2e scripts do.
func serve(flag string) string {
	return `echo "$0 $*" >> "$(dirname "$0")/calls.log"
port=""; while [ $# -gt 0 ]; do [ "$1" = ` + flag + ` ] && port="$2"; shift; done
exec python3 -m http.server "$port" --bind 127.0.0.1 >/dev/null 2>&1
`
}

func needPython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed to stand in for the servers")
	}
}

func listening(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 200*1e6)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// webE2ETools are the tools npm ci installs for the e2e scripts: vite serves the built app on the port after
// --port and logs the API it was given, tsc and Playwright log (Playwright exits as STUB_PLAYWRIGHT_EXIT says).
func webE2ETools(bin string) map[string]string {
	return map[string]string{
		"tsc":        "",
		"playwright": `echo "playwright E2E_URL=$E2E_URL YGG_E2E_STORE=${YGG_E2E_STORE-unset}" >> "` + bin + `/calls.log"; exit "${STUB_PLAYWRIGHT_EXIT:-0}"` + "\n",
		"vite": `echo "vite $*" >> "` + bin + `/calls.log"` + "\n" +
			`[ "$1" = preview ] || exit 0` + "\n" +
			`echo "app API_URL=$API_URL" >> "` + bin + `/calls.log"` + "\n" + serve("--port"),
	}
}

func TestE2EStartsAMockAndTheAppRunsPlaywrightAndStopsThem(t *testing.T) {
	needPython(t)
	mockPort, appPort := freePort(t), freePort(t)
	env := []string{"YGG_MOCK_PORT=" + strconv.Itoa(mockPort), "YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5"}
	mk := func() string {
		bin := t.TempDir()
		stub(t, bin, "prism", serve("--port"))
		webTools(t, bin, webE2ETools(bin))
		return bin
	}
	leafWith := func(bin string, files map[string]string) string { files["package.json"] = "{}"; return repo(t, files) }
	bin := mk()
	dir := leafWith(bin, map[string]string{"contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	code, out := script(t, dir, bin, "web-e2e.sh", env...)
	got := calls(bin)
	for _, want := range []string{"prism mock --host 127.0.0.1 --port " + strconv.Itoa(mockPort) + " contracts/api.openapi.yaml", "npm ci --ignore-scripts --no-audit --no-fund", "vite build",
		"app API_URL=http://127.0.0.1:" + strconv.Itoa(mockPort), "playwright E2E_URL=http://127.0.0.1:" + strconv.Itoa(appPort)} {
		if !strings.Contains(got, want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
	if code != 0 {
		t.Errorf("exit %d\n%s", code, out)
	}
	time.Sleep(300 * time.Millisecond)
	if listening(mockPort) || listening(appPort) {
		t.Error("a server the script started was still up when it ended")
	}
	// A failing spec fails the check, and the servers are still stopped.
	if code, _ := script(t, dir, mk(), "web-e2e.sh", append(env, "STUB_PLAYWRIGHT_EXIT=1")...); code == 0 {
		t.Error("a failing Playwright run passed")
	}
	time.Sleep(300 * time.Millisecond)
	if listening(mockPort) || listening(appPort) {
		t.Error("a server was left up after a failing run")
	}
	// A mock that never comes up ends the run, with its log.
	broken := mk()
	stub(t, broken, "prism", "echo 'prism: cannot read the contract' >&2\nexit 1\n")
	if code, out := script(t, dir, broken, "web-e2e.sh", append(env, "YGG_E2E_WAIT=2")...); code == 0 || !strings.Contains(out, "mock did not answer") {
		t.Errorf("no mock: exit %d\n%s", code, out)
	}
	// A server of an earlier run still on a port is refused, not mistaken for this run's mock.
	taken, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(mockPort))
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busyBin := mk()
	if code, out := script(t, leafWith(busyBin, map[string]string{"contracts/api.openapi.yaml": "x"}), busyBin, "web-e2e.sh", env...); code == 0 || !strings.Contains(out, "already in use") || calls(busyBin) != "" {
		t.Errorf("a busy port: exit %d, calls %q\n%s", code, calls(busyBin), out)
	}
}

func TestAcceptanceChecksTheAPIKeepsWhatTheWebCopyPromisesThenRunsBothAgainstEachOther(t *testing.T) {
	needPython(t)
	db, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	apiPort, appPort := freePort(t), freePort(t)
	env := []string{"YGG_DB_HOST=127.0.0.1", "YGG_DB_PORT=" + strconv.Itoa(db.Addr().(*net.TCPAddr).Port), "YGG_API_PORT=" + strconv.Itoa(apiPort),
		"YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5"}
	mk := func() string {
		bin := t.TempDir()
		stub(t, bin, "oasdiff", `echo "oasdiff $*" >> "$(dirname "$0")/calls.log"`+"\n"+`[ -z "$STUB_OASDIFF_OUT" ] || echo "$STUB_OASDIFF_OUT"`+"\n"+`exit "${STUB_OASDIFF_EXIT:-0}"`+"\n")
		// go build -o <path>: the "API" it makes serves on $PORT and logs the database it was given.
		stub(t, bin, "go", `out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
cat > "$out" <<EOS
#!/bin/sh
echo "api PORT=\$PORT DATABASE_URL=\$DATABASE_URL" >> "`+bin+`/calls.log"
exec python3 -m http.server "\$PORT" --bind 127.0.0.1 >/dev/null 2>&1
EOS
chmod +x "$out"
`)
		stub(t, bin, "node", `echo "node $1 API_URL=$API_URL" >> "`+bin+`/calls.log"`+"\n"+`exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1`+"\n")
		webTools(t, bin, webE2ETools(bin))
		return bin
	}
	project := func(bin string) string {
		return leaf(t, map[string]string{"repos/api/openapi.yaml": "openapi: 3.1.0\n", "repos/api/cmd/api/main.go": "package main\n", "repos/web/contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	}
	bin := mk()
	code, out := script(t, project(bin), bin, "accept-e2e.sh", env...)
	got := calls(bin)
	for _, want := range []string{"oasdiff breaking repos/web/contracts/api.openapi.yaml repos/api/openapi.yaml --fail-on ERR", "api PORT=" + strconv.Itoa(apiPort) + " DATABASE_URL=postgres://postgres@127.0.0.1:", "npm ci --ignore-scripts --no-audit --no-fund", "npm ci --ignore-scripts --no-audit --no-fund", "vite build",
		"API_URL=http://127.0.0.1:" + strconv.Itoa(apiPort), "playwright E2E_URL=http://127.0.0.1:" + strconv.Itoa(appPort)} {
		if !strings.Contains(got, want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
	if code != 0 {
		t.Errorf("exit %d\n%s", code, out)
	}
	time.Sleep(300 * time.Millisecond)
	if listening(apiPort) || listening(appPort) {
		t.Error("a server the script started was still up when it ended")
	}
	// An API whose contract breaks what the web app's copy promises stops it before anything is built.
	bin = mk()
	code, out = script(t, project(bin), bin, "accept-e2e.sh", append(env, "STUB_OASDIFF_EXIT=1", "STUB_OASDIFF_OUT=error [response-success-status-removed]")...)
	if code == 0 || !strings.Contains(out, "breaks what the web app's copy") || strings.Contains(calls(bin), "go ") || strings.Contains(calls(bin), "npm") {
		t.Errorf("a broken contract: exit %d, calls %q\n%s", code, calls(bin), out)
	}
	failing := mk()
	if code, _ := script(t, project(failing), failing, "accept-e2e.sh", append(env, "STUB_PLAYWRIGHT_EXIT=1")...); code == 0 {
		t.Error("a failing Playwright run passed")
	}
	// A server of an earlier run still on a port is refused, not mistaken for this run's.
	taken, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(appPort))
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busyBin := mk()
	if code, out := script(t, project(busyBin), busyBin, "accept-e2e.sh", env...); code == 0 || !strings.Contains(out, "already in use") || strings.Contains(calls(busyBin), "go ") {
		t.Errorf("a busy port: exit %d, calls %q\n%s", code, calls(busyBin), out)
	}
	if code, out := script(t, leaf(t, map[string]string{"repos/api/cmd/api/main.go": "package main\n", "repos/web/contracts/api.openapi.yaml": "x"}), mk(), "accept-e2e.sh", env...); code == 0 || !strings.Contains(out, "has no repos/api/openapi.yaml") {
		t.Errorf("an API leaf with no contract: exit %d\n%s", code, out)
	}
}

func TestAcceptanceOfAWebOnlyProductRunsTheWebAppAgainstItsOwnMock(t *testing.T) {
	needPython(t)
	// The database is not up and no API is built: a web-only product has neither.
	apiPort, appPort := freePort(t), freePort(t)
	env := []string{"YGG_DB_HOST=127.0.0.1", "YGG_DB_PORT=" + strconv.Itoa(freePort(t)), "YGG_API_PORT=" + strconv.Itoa(apiPort),
		"YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5"}
	mk := func() string {
		bin := t.TempDir()
		stub(t, bin, "oasdiff", `echo "oasdiff $*" >> "$(dirname "$0")/calls.log"`+"\n")
		stub(t, bin, "go", `echo "go $*" >> "$(dirname "$0")/calls.log"`+"\n")
		stub(t, bin, "prism", serve("--port"))
		webTools(t, bin, webE2ETools(bin))
		return bin
	}
	bin := mk()
	dir := leaf(t, map[string]string{"repos/web/contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	code, out := script(t, dir, bin, "accept-e2e.sh", env...)
	got := calls(bin)
	for _, want := range []string{"prism mock --host 127.0.0.1 --port " + strconv.Itoa(apiPort) + " contracts/api.openapi.yaml", "vite build",
		"app API_URL=http://127.0.0.1:" + strconv.Itoa(apiPort), "playwright E2E_URL=http://127.0.0.1:" + strconv.Itoa(appPort)} {
		if !strings.Contains(got, want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
	if code != 0 || strings.Contains(got, "oasdiff") || strings.Contains(got, "go ") {
		t.Errorf("exit %d, calls %q\n%s", code, got, out)
	}
	time.Sleep(300 * time.Millisecond)
	if listening(apiPort) || listening(appPort) {
		t.Error("a server the script started was still up when it ended")
	}
	// With no contract either, the web app runs with no API at all.
	bin = mk()
	code, out = script(t, leaf(t, map[string]string{"repos/web/package.json": "{}"}), bin, "accept-e2e.sh", env...)
	if code != 0 || strings.Contains(calls(bin), "prism") || !strings.Contains(calls(bin), "playwright") {
		t.Errorf("no API and no contract: exit %d, calls %q\n%s", code, calls(bin), out)
	}
}

// acceptRun is the acceptance run of a project with an API leaf and a web leaf that has its own serve.mjs, with
// stand-ins for go (the "API" logs every name it was given and serves on $PORT) and node (it logs the script it
// was given and the web names, and serves on $PORT). It returns the exit code, the output and the calls.
func acceptRun(t *testing.T, extra ...string) (int, string, string) {
	t.Helper()
	needPython(t)
	db, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	apiPort, appPort := freePort(t), freePort(t)
	env := append([]string{"YGG_DB_HOST=127.0.0.1", "YGG_DB_PORT=" + strconv.Itoa(db.Addr().(*net.TCPAddr).Port), "YGG_API_PORT=" + strconv.Itoa(apiPort),
		"YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5"}, extra...)
	bin := t.TempDir()
	stub(t, bin, "oasdiff", "exit 0\n")
	stub(t, bin, "go", `out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
cat > "$out" <<EOS
#!/bin/sh
echo "api PORT=\$PORT HOST=\$HOST DATABASE_URL=\$DATABASE_URL DATA_DIR=\$DATA_DIR S3_ENDPOINT=\$S3_ENDPOINT S3_REGION=\$S3_REGION S3_BUCKET=\$S3_BUCKET S3_ACCESS_KEY_ID=\$S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY=\$S3_SECRET_ACCESS_KEY" >> "`+bin+`/calls.log"
[ -d "\$DATA_DIR" ] && echo "api data dir exists" >> "`+bin+`/calls.log"
exec python3 -m http.server "\$PORT" --bind 127.0.0.1 >/dev/null 2>&1
EOS
chmod +x "$out"
`)
	stub(t, bin, "node", `echo "node $1 PORT=$PORT HOST=$HOST API_URL=$API_URL S3_ENDPOINT=$S3_ENDPOINT S3_BUCKET=$S3_BUCKET DIST=$DIST" >> "`+bin+`/calls.log"
exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1
`)
	webTools(t, bin, webE2ETools(bin))
	dir := leaf(t, map[string]string{"repos/api/openapi.yaml": "openapi: 3.1.0\n", "repos/api/cmd/api/main.go": "package main\n",
		"repos/web/contracts/api.openapi.yaml": "openapi: 3.1.0\n", "repos/web/serve.mjs": "// the leaf's own\n"})
	code, out := script(t, dir, bin, "accept-e2e.sh", env...)
	time.Sleep(300 * time.Millisecond)
	if listening(apiPort) || listening(appPort) {
		t.Error("a server the script started was still up when it ended")
	}
	return code, out, calls(bin)
}

// The acceptance run gives the API every name of delta §9.4 and serves the web app with the baked serve.mjs.
func TestAcceptanceGivesTheRuntime(t *testing.T) {
	code, out, got := acceptRun(t)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	apiLine := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "api PORT=") {
			apiLine = l
		}
	}
	for _, want := range []string{"HOST=127.0.0.1", "DATABASE_URL=postgres://postgres@127.0.0.1:", " DATA_DIR=/", "S3_ENDPOINT=http://s3:8333 ", "S3_REGION=us-east-1 ", "S3_BUCKET=ygg-store ", "S3_ACCESS_KEY_ID=ygg-accept ", "S3_SECRET_ACCESS_KEY=ygg-accept-secret"} {
		if !strings.Contains(apiLine, want) {
			t.Errorf("the API line lacks %q:\n%s", want, apiLine)
		}
	}
	if !strings.Contains(got, "api data dir exists") {
		t.Errorf("DATA_DIR was not made before the API started:\n%s", got)
	}
	api := regexp.MustCompile(` DATA_DIR=(\S+)`).FindStringSubmatch(apiLine)
	if api == nil || strings.HasPrefix(api[1], "/home/") || strings.Contains(api[1], "repos/") {
		t.Errorf("DATA_DIR %v is not in a temporary folder", api)
	}
	nodeLine := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "node ") {
			nodeLine = l
		}
	}
	if !strings.HasPrefix(nodeLine, "node "+bakedPath("serve.mjs")+" ") {
		t.Errorf("the web app is not served by the baked serve.mjs:\n%s", nodeLine)
	}
	for _, want := range []string{"HOST=127.0.0.1", "API_URL=http://127.0.0.1:", "S3_ENDPOINT=http://s3:8333 ", "S3_BUCKET=ygg-store ", "DIST="} {
		if !strings.Contains(nodeLine, want) {
			t.Errorf("the node line lacks %q:\n%s", want, nodeLine)
		}
	}
	if strings.Contains(got, "vite preview") || strings.Contains(got, "app API_URL") {
		t.Errorf("the web app was served by vite preview:\n%s", got)
	}
	// YGG_S3_ENDPOINT moves the store for both.
	code, out, got = acceptRun(t, "YGG_S3_ENDPOINT=http://store.test:9000")
	if code != 0 || strings.Count(got, "S3_ENDPOINT=http://store.test:9000 ") != 2 {
		t.Errorf("YGG_S3_ENDPOINT: exit %d\n%s\n%s", code, out, got)
	}
}

// The baked serve.mjs is the template's, byte for byte.
func TestBakedServeIsTheTemplates(t *testing.T) {
	baked, err := os.ReadFile(bakedPath("serve.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := os.ReadFile(filepath.Join("templates", "web", "serve.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baked, tpl) {
		t.Error("devcontainer/checks/config/serve.mjs differs from templates/web/serve.mjs; the template's is the source, copy it over the baked one")
	}
}

// A spec can tell whether a real store is behind /<bucket>/: the acceptance run sets YGG_E2E_STORE=1, the rung
// leaves it unset even when the caller's environment sets it, and so does the acceptance run of a product with
// no API.
func TestE2EStoreFlag(t *testing.T) {
	code, out, got := acceptRun(t)
	if code != 0 || !strings.Contains(got, "YGG_E2E_STORE=1") {
		t.Errorf("accept: exit %d\n%s\n%s", code, out, got)
	}
	needPython(t)
	mockPort, appPort := freePort(t), freePort(t)
	env := []string{"YGG_MOCK_PORT=" + strconv.Itoa(mockPort), "YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5", "YGG_E2E_STORE=1"}
	bin := t.TempDir()
	stub(t, bin, "prism", serve("--port"))
	webTools(t, bin, webE2ETools(bin))
	dir := repo(t, map[string]string{"package.json": "{}", "contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	code, out = script(t, dir, bin, "web-e2e.sh", env...)
	if got := calls(bin); code != 0 || !strings.Contains(got, "YGG_E2E_STORE=unset") || strings.Contains(got, "YGG_E2E_STORE=1") {
		t.Errorf("rung: exit %d\n%s\n%s", code, out, got)
	}
	// No API leaf: the Prism branch has no store.
	apiPort := freePort(t)
	bin = t.TempDir()
	stub(t, bin, "prism", serve("--port"))
	stub(t, bin, "oasdiff", "exit 0\n")
	stub(t, bin, "go", "exit 0\n")
	webTools(t, bin, webE2ETools(bin))
	dir = leaf(t, map[string]string{"repos/web/contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	code, out = script(t, dir, bin, "accept-e2e.sh", "YGG_API_PORT="+strconv.Itoa(apiPort), "YGG_APP_PORT="+strconv.Itoa(appPort), "YGG_E2E_WAIT=5", "YGG_E2E_STORE=1")
	if got := calls(bin); code != 0 || !strings.Contains(got, "YGG_E2E_STORE=unset") {
		t.Errorf("accept with no API: exit %d\n%s\n%s", code, out, got)
	}
}
