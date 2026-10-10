package web

import (
	"net/http"
	"strings"
	"testing"
)

// TestStatic checks the embedded page and its ES modules are served: "/" is
// HTML containing a canvas, and every JS module referenced from main.js's
// import graph answers 200 with a JavaScript content type.
func TestStatic(t *testing.T) {
	resp := get(t, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: content-type %q, want text/html", ct)
	}
	body := getBody(t, "/")
	if !strings.Contains(string(body), "<canvas") {
		t.Fatalf("GET /: body doesn't contain <canvas: %s", body)
	}

	for _, js := range []string{
		"/js/main.js", "/js/tree.js", "/js/camera.js", "/js/mesh.js",
		"/js/renderer.js", "/js/treeview.js", "/js/modelpanel.js",
		"/js/api.js", "/js/mat4.js",
	} {
		resp := get(t, js)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d", js, resp.StatusCode)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
			t.Errorf("GET %s: content-type %q, want text/javascript", js, ct)
		}
	}
}
