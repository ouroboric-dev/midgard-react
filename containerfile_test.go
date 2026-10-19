package midgard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ouroboric-dev/gungnir/schema"
)

var (
	fromLine   = regexp.MustCompile(`(?m)^FROM\s+(\S+)(?:\s+AS\s+(\S+))?`)
	digestRef  = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]*(:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$`)
	fetchCall  = regexp.MustCompile(`fetch (https://\S+) (\S+) ([0-9a-f]{64})`)
	npmGlobal  = regexp.MustCompile(`npm install -g[^\n]*`)
	pinned     = regexp.MustCompile(`@[^@\s]+@\d+\.\d+\.\d+|^[a-z-]+@\d+\.\d+\.\d+$`)
	pipedShell = regexp.MustCompile(`\|\s*(sudo\s+)?(ba|z)?sh\b`)
)

func TestTheStackImagesAreMadeOfPinnedInputsOnly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("devcontainer", "Containerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	stages := map[string]bool{}
	for _, f := range fromLine.FindAllStringSubmatch(text, -1) {
		if !digestRef.MatchString(f[1]) && !stages[f[1]] {
			t.Errorf("FROM %s is neither pinned by digest nor an earlier stage", f[1])
		}
		stages[f[2]] = true
	}
	for _, want := range []string{"golang", "web", "acceptance"} {
		if !stages[want] {
			t.Errorf("the Containerfile has no stage %q", want)
		}
	}
	if n := len(fetchCall.FindAllStringSubmatch(text, -1)); n != 3 {
		t.Errorf("%d downloads are checked against a SHA-256; the three release binaries are", n)
	}
	if n := strings.Count(text, "curl -"); n != 1 {
		t.Errorf("curl is used %d times; the one use is inside fetch, which checks the hash", n)
	}
	if !strings.Contains(text, `echo "$3  $2" | sha256sum -c -`) {
		t.Error("fetch does not check the download against its SHA-256 with sha256sum -c")
	}
	for _, line := range npmGlobal.FindAllString(text, -1) {
		for _, w := range strings.Fields(line)[3:] {
			if !strings.HasPrefix(w, "-") && w != "\\" && !pinned.MatchString(w) {
				t.Errorf("npm installs %q, which is not at an exact version", w)
			}
		}
	}
	if pipedShell.MatchString(text) || strings.Contains(text, "@latest") || strings.Contains(strings.ToLower(text), ":latest") {
		t.Error("the Containerfile pipes a download into a shell or names a floating version")
	}
	for _, want := range []string{"oasdiff", "vacuum", "osv-scanner", "prism-cli", "playwright install --with-deps chromium", "COPY checks/ /opt/ygg/checks/", "chmod -R a+rX,go-w /opt/ygg/checks", "/usr/local/go"} {
		if !strings.Contains(text, want) {
			t.Errorf("the Containerfile lacks %q", want)
		}
	}
	if _, err := schema.HashContext("devcontainer"); err != nil {
		t.Errorf("devcontainer is not a build context: %v", err)
	}
}

func TestPlaywrightsVersionIsTheTemplatesSoItsBrowsersMatch(t *testing.T) {
	pkg, err := os.ReadFile(filepath.Join("templates", "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	cf, _ := os.ReadFile(filepath.Join("devcontainer", "Containerfile"))
	m := regexp.MustCompile(`"@playwright/test": "(\d+\.\d+\.\d+)"`).FindSubmatch(pkg)
	if m == nil {
		t.Fatal("the template pins no @playwright/test")
	}
	if !strings.Contains(string(cf), "playwright@"+string(m[1])) {
		t.Errorf("the template's Playwright is %s and the stack installs another; the browsers would not match", m[1])
	}
}

func TestEveryCheckScriptIsExecutableAndStartsTheWayTheOthersDo(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("devcontainer", "checks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no check scripts")
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "protected-common" { // checks/config holds the baked tool configs and protected-common the list of protected files: data, not scripts
			continue
		}
		p := filepath.Join("devcontainer", "checks", e.Name())
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		if st.Mode()&0o111 == 0 || !strings.HasPrefix(string(b), "#!/bin/bash\n") || !strings.Contains(string(b), "set -euo pipefail") {
			t.Errorf("%s is not an executable bash script that starts with set -euo pipefail", p)
		}
	}
}
