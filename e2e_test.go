package midgard

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

func TestE2EStartsAMockAndTheAppRunsPlaywrightAndStopsThem(t *testing.T) {
	needPython(t)
	mockPort, appPort := freePort(t), freePort(t)
	env := []string{"YGG_MOCK_PORT=" + strconv.Itoa(mockPort), "YGG_APP_PORT=" + strconv.Itoa(appPort), "YGG_E2E_WAIT=5"}
	mk := func() string {
		bin := t.TempDir()
		stub(t, bin, "prism", serve("--port"))
		stub(t, bin, "npm", `echo "npm $*" >> "$(dirname "$0")/calls.log"`+"\n")
		stub(t, bin, "npx", `d="$(dirname "$0")"
case "$1" in
  playwright) echo "playwright E2E_URL=$E2E_URL" >> "$d/calls.log"; exit "${STUB_PLAYWRIGHT_EXIT:-0}" ;;
esac
`)
		// The app is vite from the leaf's own node_modules, run directly so that its pid is the server's.
		stub(t, bin, "vite", `echo "app API_URL=$API_URL" >> "`+bin+`/calls.log"`+"\n"+serve("--port"))
		return bin
	}
	// leafWith makes the leaf, with the stub vite where the script looks for it.
	leafWith := func(bin string, files map[string]string) string {
		dir := leaf(t, files)
		if err := os.MkdirAll(filepath.Join(dir, "node_modules", ".bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(bin, "vite"), filepath.Join(dir, "node_modules", ".bin", "vite")); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	bin := mk()
	dir := leafWith(bin, map[string]string{"contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
	code, out := script(t, dir, bin, "web-e2e.sh", env...)
	got := calls(bin)
	for _, want := range []string{"prism mock --host 127.0.0.1 --port " + strconv.Itoa(mockPort) + " contracts/api.openapi.yaml", "npm run build",
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
		stub(t, bin, "npm", `echo "npm $*" >> "$(dirname "$0")/calls.log"`+"\n")
		stub(t, bin, "npx", `d="$(dirname "$0")"
case "$1" in
  playwright) echo "playwright E2E_URL=$E2E_URL" >> "$d/calls.log"; exit "${STUB_PLAYWRIGHT_EXIT:-0}" ;;
esac
`)
		stub(t, bin, "vite", `echo "app API_URL=$API_URL" >> "`+bin+`/calls.log"`+"\n"+serve("--port"))
		return bin
	}
	// project makes the snapshot, with the stub vite where the script looks for the web leaf's own.
	project := func(bin string) string {
		dir := leaf(t, map[string]string{"repos/api/openapi.yaml": "openapi: 3.1.0\n", "repos/api/cmd/api/main.go": "package main\n", "repos/web/contracts/api.openapi.yaml": "openapi: 3.1.0\n"})
		if err := os.MkdirAll(filepath.Join(dir, "repos/web/node_modules/.bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(bin, "vite"), filepath.Join(dir, "repos/web/node_modules/.bin/vite")); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	bin := mk()
	code, out := script(t, project(bin), bin, "accept-e2e.sh", env...)
	got := calls(bin)
	for _, want := range []string{"oasdiff breaking repos/web/contracts/api.openapi.yaml repos/api/openapi.yaml --fail-on ERR", "api PORT=" + strconv.Itoa(apiPort) + " DATABASE_URL=postgres://postgres@127.0.0.1:", "npm ci --no-audit --no-fund", "npm run build",
		"app API_URL=http://127.0.0.1:" + strconv.Itoa(apiPort), "playwright E2E_URL=http://127.0.0.1:" + strconv.Itoa(appPort)} {
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
	if code, out := script(t, leaf(t, map[string]string{"repos/web/contracts/api.openapi.yaml": "x"}), mk(), "accept-e2e.sh", env...); code == 0 || !strings.Contains(out, "has no repos/api/openapi.yaml") {
		t.Errorf("no API leaf: exit %d\n%s", code, out)
	}
}
