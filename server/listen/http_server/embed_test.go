package http_server

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var assetRef = regexp.MustCompile(`(?:src|href)="(/[^"]+)"`)

// The web client is embedded at build time. If the embed pattern misses a file
// (Vite emits helper chunks named with a leading underscore), the binary still
// builds and starts, but the browser gets 404s for the bundle. Check every asset
// that index.html points at instead of trusting the pattern.
func TestEmbeddedFrontendAssetsExist(t *testing.T) {
	index, err := fs.ReadFile(local, "dist/index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}

	checked := 0
	for _, match := range assetRef.FindAllStringSubmatch(string(index), -1) {
		ref := match[1]
		if strings.HasPrefix(ref, "/api/") || strings.HasPrefix(ref, "/.well-known/") {
			continue
		}
		if _, err := fs.Stat(local, "dist"+ref); err != nil {
			t.Errorf("index.html references %s but the embedded bundle does not contain it", ref)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("index.html references no assets; the bundle layout changed")
	}
}
