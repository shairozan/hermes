package docker

import (
	"archive/tar"
	"bytes"
	"sort"
	"strings"
	"testing"
)

// buildTar produces an in-memory tar stream mirroring what Docker's
// CopyFromContainer returns: entries rooted at the working-directory base name.
func buildTar(t *testing.T, files map[string]string) *tar.Reader {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatalf("write tar content: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return tar.NewReader(&buf)
}

// collect runs matchTarArtifacts and returns the emitted relative paths.
func collect(t *testing.T, files map[string]string, stripPrefix string, patterns []string) []string {
	t.Helper()

	var got []string
	n, err := matchTarArtifacts(buildTar(t, files), stripPrefix, patterns, func(relPath string, _ []byte, _ int64) error {
		got = append(got, relPath)
		return nil
	})
	if err != nil {
		t.Fatalf("matchTarArtifacts: %v", err)
	}
	if int(n) != len(got) {
		t.Fatalf("count mismatch: returned %d, emitted %d", n, len(got))
	}
	sort.Strings(got)
	return got
}

// TestMatchTarArtifactsGlobstar validates REQ-FILE-LOC-005 for Docker mode:
// globstar (**) retain patterns are expanded against the container workspace.
func TestMatchTarArtifactsGlobstar(t *testing.T) {
	// Docker roots entries at the working-dir base name ("workspace").
	files := map[string]string{
		"workspace/":                "",
		"workspace/top.json":        "a",
		"workspace/out/":            "",
		"workspace/out/a.json":      "b",
		"workspace/out/c.txt":       "c",
		"workspace/out/deep/b.json": "d",
		"workspace/out/deep/e.log":  "e",
		"workspace/other/f.json":    "f",
	}

	cases := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{
			name:     "globstar recursive json",
			patterns: []string{"out/**/*.json"},
			want:     []string{"out/a.json", "out/deep/b.json"},
		},
		{
			name:     "top level star only",
			patterns: []string{"*.json"},
			want:     []string{"top.json"},
		},
		{
			name:     "double star anywhere",
			patterns: []string{"**/*.json"},
			want:     []string{"other/f.json", "out/a.json", "out/deep/b.json", "top.json"},
		},
		{
			name:     "literal nested path",
			patterns: []string{"out/deep/b.json"},
			want:     []string{"out/deep/b.json"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(t, files, "workspace", tc.patterns)
			if len(got) != len(tc.want) {
				t.Fatalf("pattern %v: got %v, want %v", tc.patterns, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("pattern %v: got %v, want %v", tc.patterns, got, tc.want)
				}
			}
		})
	}
}

// TestMatchTarArtifactsDedup ensures a file matching multiple patterns is
// emitted only once.
func TestMatchTarArtifactsDedup(t *testing.T) {
	files := map[string]string{
		"workspace/out/a.json": "x",
	}
	got := collect(t, files, "workspace", []string{"**/*.json", "out/a.json"})
	if len(got) != 1 || got[0] != "out/a.json" {
		t.Fatalf("expected single deduped match, got %v", got)
	}
}

func TestRelativeArtifactPath(t *testing.T) {
	cases := []struct {
		name, strip, want string
	}{
		{"workspace/out/a.json", "workspace", "out/a.json"},
		{"/workspace/out/a.json", "workspace", "out/a.json"},
		{"workspace/", "workspace", ""},
		{"workspace", "workspace", ""},
		{"out/a.json", "", "out/a.json"},
		{"out/a.json", ".", "out/a.json"},
	}
	for _, tc := range cases {
		if got := relativeArtifactPath(tc.name, tc.strip); got != tc.want {
			t.Errorf("relativeArtifactPath(%q, %q) = %q, want %q", tc.name, tc.strip, got, tc.want)
		}
	}
}
