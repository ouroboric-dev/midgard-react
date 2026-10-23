package midgard

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// seen is what a stand-in server recorded about its last request.
type seen struct {
	mu     sync.Mutex
	method string
	uri    string
	host   string
	header http.Header
	body   string
}

func (s *seen) handler(answer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.method, s.uri, s.host, s.header, s.body = r.Method, r.RequestURI, r.Host, r.Header.Clone(), string(b)
		s.mu.Unlock()
		_, _ = io.WriteString(w, answer)
	})
}

func (s *seen) get() seen {
	s.mu.Lock()
	defer s.mu.Unlock()
	return seen{method: s.method, uri: s.uri, host: s.host, header: s.header, body: s.body}
}

// startServe starts the template's serve.mjs on a free port and returns the port. It skips without node
// (YGG_REQUIRE_TEMPLATES makes that a failure) and stops the server when the test ends.
func startServe(t *testing.T, dir string, env ...string) int {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		if os.Getenv("YGG_REQUIRE_TEMPLATES") != "" {
			t.Fatal("node is not installed")
		}
		t.Skip("node is not installed (set YGG_REQUIRE_TEMPLATES to fail instead)")
	}
	port := freePort(t)
	cmd := exec.Command("node", "serve.mjs")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"PORT=" + strconv.Itoa(port)}, env...)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("node serve.mjs: %v", err)
	}
	exited := make(chan error, 1)
	done := make(chan struct{})
	go func() { exited <- cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second); err == nil {
			c.Close()
			return port
		}
		select {
		case err := <-exited:
			t.Fatalf("serve.mjs exited before the port answered: %v\n%s", err, out.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve.mjs did not answer on port %d within 10 s\n%s", port, out.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func fetch(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// nonLoopbackAddr returns an address of this machine that is not loopback, or "".
func nonLoopbackAddr() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() {
			return ip.IP.String()
		}
	}
	return ""
}

// The web leaf's own server (delta §9.3, §9.4): it serves the bundle with the index fallback, forwards /api/
// to API_URL with X-Forwarded-Host and X-Forwarded-Proto, and forwards the bucket's path to S3_ENDPOINT
// unchanged, with the request's Host and no X-Forwarded-* header, so a presigned signature still matches.
func TestServeForwards(t *testing.T) {
	dir := copyTemplate(t, "web")
	if err := os.MkdirAll(filepath.Join(dir, "dist/assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist/index.html"), []byte("<p>INDEX</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist/assets/app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	var api, store seen
	apiSrv := httptest.NewServer(api.handler("from-api"))
	defer apiSrv.Close()
	storeSrv := httptest.NewServer(store.handler("from-store"))
	defer storeSrv.Close()

	port := startServe(t, dir, "HOST=127.0.0.1", "API_URL="+apiSrv.URL, "S3_ENDPOINT="+storeSrv.URL, "S3_BUCKET=ygg-store")
	base := "http://127.0.0.1:" + strconv.Itoa(port)

	t.Run("api", func(t *testing.T) {
		req, _ := http.NewRequest("GET", base+"/api/galleries?x=1", nil)
		req.Host = "gallery.test:3000"
		resp, body := fetch(t, req)
		if resp.StatusCode != 200 || body != "from-api" {
			t.Fatalf("got %d %q", resp.StatusCode, body)
		}
		got := api.get()
		if got.uri != "/galleries?x=1" {
			t.Errorf("the API got %q, want /galleries?x=1", got.uri)
		}
		if h := got.header.Get("X-Forwarded-Host"); h != "gallery.test:3000" {
			t.Errorf("X-Forwarded-Host = %q", h)
		}
		if p := got.header.Get("X-Forwarded-Proto"); p != "http" {
			t.Errorf("X-Forwarded-Proto = %q", p)
		}
	})

	t.Run("store", func(t *testing.T) {
		req, _ := http.NewRequest("PUT", base+"/ygg-store/a/b.jpg?X-Amz-Signature=s", strings.NewReader("PHOTO-BYTES"))
		req.Host = "gallery.test:3000"
		resp, body := fetch(t, req)
		if resp.StatusCode != 200 || body != "from-store" {
			t.Fatalf("got %d %q", resp.StatusCode, body)
		}
		got := store.get()
		if got.method != "PUT" || got.uri != "/ygg-store/a/b.jpg?X-Amz-Signature=s" || got.body != "PHOTO-BYTES" {
			t.Errorf("the store got %s %q %q", got.method, got.uri, got.body)
		}
		if got.host != "gallery.test:3000" {
			t.Errorf("the store's Host = %q, want the request's", got.host)
		}
		for _, h := range []string{"X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Proto", "X-Forwarded-For"} {
			if v := got.header.Get(h); v != "" {
				t.Errorf("the store got %s: %q", h, v)
			}
		}
	})

	t.Run("bucket", func(t *testing.T) {
		for _, p := range []string{"/ygg-store", "/ygg-store?list-type=2"} {
			resp, body := fetch(t, mustGet(t, base+p))
			if resp.StatusCode != 200 || body != "from-store" {
				t.Errorf("GET %s: %d %q", p, resp.StatusCode, body)
			}
			if got := store.get(); got.uri != p {
				t.Errorf("GET %s: the store got %q", p, got.uri)
			}
		}
	})

	t.Run("bundle", func(t *testing.T) {
		for _, p := range []string{"/", "/galleries/x"} {
			resp, body := fetch(t, mustGet(t, base+p))
			if resp.StatusCode != 200 || body != "<p>INDEX</p>" {
				t.Errorf("GET %s: %d %q", p, resp.StatusCode, body)
			}
		}
		resp, body := fetch(t, mustGet(t, base+"/assets/app.js"))
		if body != "console.log(1)" || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
			t.Errorf("GET /assets/app.js: %q, Content-Type %q", body, resp.Header.Get("Content-Type"))
		}
	})

	t.Run("dotdot", func(t *testing.T) {
		// Go's client would clean the path, so write the request by hand.
		for _, p := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/assets/../../secret.txt"} {
			c, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(c, "GET "+p+" HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
			raw, _ := io.ReadAll(c)
			c.Close()
			if strings.Contains(string(raw), "SECRET") {
				t.Errorf("GET %s left dist:\n%s", p, raw)
			}
		}
	})
}

func mustGet(t *testing.T, u string) *http.Request {
	t.Helper()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestServeListensOnHost(t *testing.T) {
	other := nonLoopbackAddr()
	if other == "" {
		t.Skip("the machine has no non-loopback address")
	}
	dir := copyTemplate(t, "web")
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dist/index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host string
		want bool
	}{{"127.0.0.1", false}, {"0.0.0.0", true}} {
		t.Run(tc.host, func(t *testing.T) {
			port := startServe(t, dir, "HOST="+tc.host)
			c, err := net.DialTimeout("tcp", net.JoinHostPort(other, strconv.Itoa(port)), time.Second)
			if err == nil {
				c.Close()
			}
			if (err == nil) != tc.want {
				t.Errorf("HOST=%s: connecting to %s: answered=%v, want %v", tc.host, other, err == nil, tc.want)
			}
		})
	}
}
