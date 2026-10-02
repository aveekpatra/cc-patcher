package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type entry struct {
	name, body string
	typ        byte
	link       string
}

func tarball(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Size: int64(len(e.body)), Linkname: e.link}
		switch typ {
		case tar.TypeXGlobalHeader:
			h = &tar.Header{Typeflag: typ, PAXRecords: map[string]string{"comment": "sha"}}
		case tar.TypeReg:
		default:
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

func serve(t *testing.T, data []byte) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/o/r/tar.gz/main" {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	old := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = old })
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
}

func testRemote(srcs ...Source) *Remote {
	return &Remote{Title: "t", Desc: "d", Repo: "o/r", Ref: "main", License: "MIT", Skills: srcs}
}

func TestRemoteInstall(t *testing.T) {
	serve(t, tarball(t, []entry{
		{name: "pax_global_header", typ: tar.TypeXGlobalHeader},
		{name: "r-main/", typ: tar.TypeDir},
		{name: "r-main/README.md", body: "readme"},
		{name: "r-main/skills/a/SKILL.md", body: "---\nname: a\n---\n"},
		{name: "r-main/skills/a/refs/x.md", body: "x"},
		{name: "r-main/skills/a/link", typ: tar.TypeSymlink, link: "/etc/passwd"},
		{name: "r-main/skills/b/SKILL.md", body: "b"},
	}))
	r := testRemote(Source{Path: "skills/a", Name: "a"}, Source{Path: "skills/b", Name: "bee"})
	if r.Enabled() {
		t.Fatal("enabled before install")
	}
	if !strings.HasSuffix(r.Description(), " (downloads from GitHub, MIT)") {
		t.Fatal(r.Description())
	}
	if err := r.Enable(); err != nil {
		t.Fatal(err)
	}
	if !r.Enabled() {
		t.Fatal("not enabled after install")
	}
	root := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "skills")
	for _, p := range []string{"a/SKILL.md", "a/refs/x.md", "a/" + Marker, "bee/SKILL.md", "bee/" + Marker} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "a/link")); err == nil {
		t.Error("symlink was extracted")
	}
	if _, err := os.Stat(filepath.Join(root, "a/README.md")); err == nil {
		t.Error("file outside subpath was extracted")
	}
	ents, _ := os.ReadDir(root)
	if len(ents) != 2 {
		t.Errorf("stage dir left behind: %v", ents)
	}

	// A user-made skill dir without our marker must survive uninstall.
	os.Remove(filepath.Join(root, "bee", Marker))
	if err := r.Disable(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "a")); err == nil {
		t.Error("a not removed")
	}
	if _, err := os.Stat(filepath.Join(root, "bee/SKILL.md")); err != nil {
		t.Error("unmarked dir was removed")
	}
	if err := r.Enable(); err == nil {
		t.Error("overwrote a dir not installed by cc-patcher")
	}
}

func TestRemoteRootSubpath(t *testing.T) {
	serve(t, tarball(t, []entry{
		{name: "r-main/SKILL.md", body: "root"},
		{name: "r-main/references/a.md", body: "a"},
	}))
	r := testRemote(Source{Path: "", Name: "whole"})
	if err := r.Enable(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "skills/whole/references/a.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteTraversal(t *testing.T) {
	for _, name := range []string{"r-main/skills/a/../../../../evil", "/abs/evil", `r-main/skills/a/..\..\evil`} {
		serve(t, tarball(t, []entry{
			{name: "r-main/skills/a/SKILL.md", body: "a"},
			{name: name, body: "pwned"},
		}))
		r := testRemote(Source{Path: "skills/a", Name: "a"})
		if err := r.Enable(); err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Errorf("%q: want unsafe path error, got %v", name, err)
		}
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if _, err := os.Stat(filepath.Join(dir, "skills/a")); err == nil {
			t.Errorf("%q: partial install left behind", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
			t.Errorf("%q: wrote outside skills dir", name)
		}
	}
}

func TestRemoteMissingSkill(t *testing.T) {
	serve(t, tarball(t, []entry{{name: "r-main/other.md", body: "x"}}))
	if err := testRemote(Source{Path: "skills/a", Name: "a"}).Enable(); err == nil {
		t.Fatal("want error for missing SKILL.md")
	}
}

func TestRemoteCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range RemoteList() {
		if r.Repo == "" || r.Ref == "" || r.License == "" || len(r.Skills) == 0 {
			t.Errorf("incomplete entry %q", r.Title)
		}
		for _, s := range r.Skills {
			if seen[s.Name] {
				t.Errorf("duplicate skill %q", s.Name)
			}
			seen[s.Name] = true
		}
	}
}
