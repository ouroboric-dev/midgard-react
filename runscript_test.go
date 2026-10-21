package midgard

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startRunScript starts the copy's run.sh in its own process group, with the given extra environment, and
// returns the command and its output. The caller stops it with stopGroup.
func startRunScript(t *testing.T, dir string, env ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command("./run.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("run.sh: %v", err)
	}
	return cmd, &out
}

// stopGroup stops run.sh and everything it started (npm and node are its children).
func stopGroup(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}

// The leaf run contract (change proposal 2026-10-21-leaf-run-contract): run.sh, in the leaf with PORT set,
// serves the leaf on PORT and accepts a connection within 60 seconds.
func TestRunScriptStarts(t *testing.T) {
	dir := copyTemplate(t, "web")
	needPackages(t, dir)
	info, err := os.Stat(filepath.Join(dir, "run.sh"))
	if err != nil {
		t.Fatalf("the template lacks run.sh: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("run.sh is not executable")
	}
	port := strconv.Itoa(freePort(t))
	cmd, out := startRunScript(t, dir, "PORT="+port)
	defer stopGroup(cmd)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(60 * time.Second)
	for {
		if c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second); err == nil {
			c.Close()
			return
		}
		select {
		case err := <-exited:
			t.Fatalf("run.sh exited before the port answered: %v\n%s", err, out)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("nothing accepted a connection on port %s within 60 s\n%s", port, out)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func TestRunScriptWithoutPortFails(t *testing.T) {
	dir := copyTemplate(t, "web")
	cmd := exec.Command("./run.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PORT=")
	out, err := cmd.CombinedOutput()
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("run.sh without PORT: want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PORT") {
		t.Errorf("the message does not name PORT:\n%s", out)
	}
}

func TestRunScriptWithoutPackagesSaysRunInit(t *testing.T) {
	dir := copyTemplate(t, "web")
	cmd := exec.Command("./run.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PORT="+strconv.Itoa(freePort(t)))
	out, err := cmd.CombinedOutput()
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("run.sh without node_modules: want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "init.sh") {
		t.Errorf("the message does not say to run init.sh:\n%s", out)
	}
}
