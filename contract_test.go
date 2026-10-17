package midgard

import (
	"strings"
	"testing"
)

func TestContractChecksTheTypesAreWhatTheContractSaysAndListsChanges(t *testing.T) {
	files := map[string]string{"node_modules/.keep": "", ".gitignore": "node_modules/\n", "contracts/api.openapi.yaml": "openapi: 3.1.0\n", "src/api/schema.d.ts": "types\n"}
	dir := repo(t, files)
	work(t, dir, map[string]string{"contracts/api.openapi.yaml": "openapi: 3.1.0\ninfo: {}\n"})
	mk := func(generates string) string {
		bin := stubs(t, "vacuum", "oasdiff")
		stub(t, bin, "npx", `echo "npx $*" >> "$(dirname "$0")/calls.log"
out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
printf '%s\n' '`+generates+`' > "$out"
`)
		return bin
	}
	bin := mk("types")
	code, out := script(t, dir, bin, "web-contract.sh", "STUB_OASDIFF_OUT=[]")
	if code != 0 || !strings.Contains(out, "none") || !strings.Contains(calls(bin), "vacuum lint --no-style --details --fail-severity error contracts/api.openapi.yaml") {
		t.Errorf("types that match: exit %d, calls %q\n%s", code, calls(bin), out)
	}
	if code, out := script(t, dir, mk("types"), "web-contract.sh", `STUB_OASDIFF_OUT=[{"id":"x"}]`); code != 0 || !strings.Contains(out, "BREAKING") {
		t.Errorf("a breaking change is listed and not refused: exit %d\n%s", code, out)
	}
	if code, out := script(t, dir, mk("other types"), "web-contract.sh", "STUB_OASDIFF_OUT=[]"); code == 0 || !strings.Contains(out, "npm run gen:api") {
		t.Errorf("types that are out of date: exit %d\n%s", code, out)
	}
	if code, _ := script(t, dir, mk("types"), "web-contract.sh", "STUB_VACUUM_EXIT=1", "STUB_OASDIFF_OUT=[]"); code == 0 {
		t.Error("a lint error passed")
	}
	if code, out := script(t, repo(t, map[string]string{"node_modules/.keep": "", "a": "x"}), mk("types"), "web-contract.sh"); code == 0 || !strings.Contains(out, "has no contracts/api.openapi.yaml") {
		t.Errorf("no contract: exit %d\n%s", code, out)
	}
}

func TestSecurityStopsOnAVulnerabilityFromEitherSource(t *testing.T) {
	dir := leaf(t, map[string]string{"node_modules/.keep": ""})
	bin := stubs(t, "npm", "osv-scanner")
	if code, out := script(t, dir, bin, "web-security.sh"); code != 0 || calls(bin) != "npm audit --audit-level=high\nosv-scanner scan source -L package-lock.json\n" {
		t.Fatalf("exit %d, calls %q\n%s", code, calls(bin), out)
	}
	for _, env := range []string{"STUB_NPM_EXIT=1", "STUB_OSV_SCANNER_EXIT=1"} {
		if code, _ := script(t, dir, stubs(t, "npm", "osv-scanner"), "web-security.sh", env); code == 0 {
			t.Errorf("%s passed", env)
		}
	}
}
