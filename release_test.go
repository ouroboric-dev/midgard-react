package midgard

import (
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The release (gallery-runtime T17): release.sh builds OUT/dist, OUT/serve.mjs and OUT/run.sh from a copy of the
// leaf that has no node_modules (a snapshot of committed files has none), and OUT/run.sh starts with Node alone:
// no npm on PATH, no node_modules anywhere.
func TestReleaseScriptStarts(t *testing.T) {
	dir := copyTemplate(t, "web")
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil {
		t.Fatal("the template copy holds node_modules")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		if os.Getenv("YGG_REQUIRE_TEMPLATES") != "" {
			t.Fatal("npm is not installed")
		}
		t.Skip("npm is not installed (set YGG_REQUIRE_TEMPLATES to fail instead)")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is not installed: %v", err)
	}
	// A relative OUT is the caller's: the script runs from another folder, with OUT a folder of the caller's
	// that already holds symlinks at the names the script writes (they must not be followed).
	caller := t.TempDir()
	out := filepath.Join(caller, "release")
	victim := filepath.Join(caller, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"run.sh", "serve.mjs"} {
		if err := os.Symlink(victim, filepath.Join(out, name)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(filepath.Join(dir, "release.sh"))
	cmd.Dir = caller
	cmd.Env = append(os.Environ(), "OUT=release")
	if b, err := cmd.CombinedOutput(); err != nil {
		if os.Getenv("YGG_REQUIRE_TEMPLATES") == "" && strings.Contains(string(b), "npm ci") && (strings.Contains(string(b), "ENOTFOUND") || strings.Contains(string(b), "EAI_AGAIN")) {
			t.Skipf("the template's packages can't be installed: %v", err)
		}
		t.Fatalf("release.sh: %v\n%s", err, b)
	}

	if b, err := os.ReadFile(victim); err != nil || string(b) != "keep" {
		t.Errorf("release.sh wrote through a symlink: %q, %v", b, err)
	}
	// Only regular files and folders, and no node_modules.
	err = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "node_modules" {
			t.Errorf("the release holds %s", p)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			t.Errorf("%s is not a regular file", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"dist/index.html", "serve.mjs", "run.sh"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("the release lacks %s: %v", f, err)
		}
	}

	// Start it from a folder of its own, with node on PATH and no npm.
	bin := t.TempDir()
	if err := os.Symlink(node, filepath.Join(bin, "node")); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	run := exec.Command(filepath.Join(out, "run.sh"))
	run.Dir = t.TempDir()
	run.Env = []string{"PATH=" + bin, "PORT=" + strconv.Itoa(port), "HOME=" + t.TempDir()}
	run.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmdOut := &strings.Builder{}
	run.Stdout, run.Stderr = cmdOut, cmdOut
	if err := run.Start(); err != nil {
		t.Fatalf("run.sh: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- run.Wait() }()
	t.Cleanup(func() { stopGroup(run, exited) })

	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			b := make([]byte, 512)
			n, _ := resp.Body.Read(b)
			resp.Body.Close()
			if resp.StatusCode != 200 || !strings.Contains(string(b[:n]), "<div id=\"root\"") {
				t.Fatalf("GET /: %d %q", resp.StatusCode, b[:n])
			}
			return
		}
		select {
		case err := <-exited:
			exited <- err // stopGroup waits on the same channel
			t.Fatalf("OUT/run.sh exited before the port answered: %v\n%s", err, cmdOut.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("OUT/run.sh did not answer on port %d within 30 s\n%s", port, cmdOut.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
