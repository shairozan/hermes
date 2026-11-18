package docker

import (
	"archive/tar"
	"errors"
	"io"
	"testing"

	"github.com/shairozan/hermes/executor"
)

// TestContainerRunDirectory validates REQ-EXE-LOC-005
// Requirement: Run Directory
// Priority: High
// Category: GxP Critical
// Description: Verifies the container working directory for a request
func TestContainerRunDirectory(t *testing.T) {
	cases := []struct {
		workingDir   string
		runDirectory string
		want         string
	}{
		{"/workspace", "", "/workspace"},
		{"/workspace", ".", "/workspace"},
		{"/workspace", "runs/1001", "/workspace/runs/1001"},
		{"/workspace/", "./runs/1001/", "/workspace/runs/1001"},
		{"", "", ""},
	}

	for _, c := range cases {
		got, err := containerRunDirectory(&executor.ExecutionRequest{WorkingDir: c.workingDir, RunDirectory: c.runDirectory})
		if err != nil {
			t.Errorf("containerRunDirectory(%q, %q) returned error: %v", c.workingDir, c.runDirectory, err)
			continue
		}
		if got != c.want {
			t.Errorf("containerRunDirectory(%q, %q) = %q, want %q", c.workingDir, c.runDirectory, got, c.want)
		}
	}
}

// TestContainerRunDirectoryRejected validates REQ-SEC-LOC-003
// Requirement: Run Directory Containment
// Priority: Critical
// Category: GxP Critical
// Description: Verifies that invalid run directories never produce a container path
func TestContainerRunDirectoryRejected(t *testing.T) {
	for _, value := range []string{"/abs", "..", "a/../../b", `a\b`} {
		got, err := containerRunDirectory(&executor.ExecutionRequest{WorkingDir: "/workspace", RunDirectory: value})
		if !errors.Is(err, executor.ErrInvalidRunDirectory) {
			t.Errorf("run_directory %q: error = %v, want ErrInvalidRunDirectory", value, err)
		}
		if got != "" {
			t.Errorf("run_directory %q: returned path %q alongside an error", value, got)
		}
	}
}

// TestContainerRunDirectoryRequiresWorkingDir validates REQ-SEC-LOC-003
// Description: Verifies that a run directory is never resolved against an
// unknown image default directory
func TestContainerRunDirectoryRequiresWorkingDir(t *testing.T) {
	got, err := containerRunDirectory(&executor.ExecutionRequest{RunDirectory: "runs/1001"})
	if !errors.Is(err, executor.ErrInvalidRunDirectory) || got != "" {
		t.Errorf("containerRunDirectory with no working_dir = %q, %v; want ErrInvalidRunDirectory", got, err)
	}
}

// readArchive lists the entries of an injection archive by type.
func readArchive(t *testing.T, r io.Reader) (dirs []string, files map[string]string) {
	t.Helper()

	files = map[string]string{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return dirs, files
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			dirs = append(dirs, hdr.Name)
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read archive entry: %v", err)
		}
		files[hdr.Name] = string(content)
	}
}

// TestInjectionArchiveCreatesRunDirectory validates REQ-EXE-LOC-005
// Description: Verifies that the run directory exists in the container even
// when no injected file is placed in it
func TestInjectionArchiveCreatesRunDirectory(t *testing.T) {
	archive, err := buildInjectionArchive(map[string][]byte{"data/x.csv": []byte("ID,DV\n")}, "runs/1001")
	if err != nil {
		t.Fatalf("buildInjectionArchive: %v", err)
	}

	dirs, files := readArchive(t, archive)
	if len(dirs) != 1 || dirs[0] != "runs/1001/" {
		t.Errorf("directory entries = %v, want [runs/1001/]", dirs)
	}
	if files["data/x.csv"] != "ID,DV\n" {
		t.Errorf("files = %v, want data/x.csv preserved", files)
	}
}

// TestInjectionArchiveWithoutRunDirectory validates REQ-EXE-LOC-005
// Description: Verifies that requests without a run directory are archived as before
func TestInjectionArchiveWithoutRunDirectory(t *testing.T) {
	archive, err := buildInjectionArchive(map[string][]byte{"model.mod": []byte("m")}, "")
	if err != nil {
		t.Fatalf("buildInjectionArchive: %v", err)
	}

	dirs, files := readArchive(t, archive)
	if len(dirs) != 0 {
		t.Errorf("directory entries = %v, want none", dirs)
	}
	if len(files) != 1 || files["model.mod"] != "m" {
		t.Errorf("files = %v, want only model.mod", files)
	}
}

// TestInjectionArchiveRunDirectoryOnly validates REQ-EXE-LOC-005
// Description: Verifies that a run directory is created even with no files to inject
func TestInjectionArchiveRunDirectoryOnly(t *testing.T) {
	archive, err := buildInjectionArchive(nil, "scratch")
	if err != nil {
		t.Fatalf("buildInjectionArchive: %v", err)
	}
	if archive == nil {
		t.Fatal("expected an archive containing the run directory")
	}

	dirs, files := readArchive(t, archive)
	if len(dirs) != 1 || dirs[0] != "scratch/" || len(files) != 0 {
		t.Errorf("dirs = %v, files = %v, want only scratch/", dirs, files)
	}
}

// TestInjectionArchiveEmpty validates REQ-EXE-LOC-005
// Description: Verifies that nothing is copied when there is nothing to inject
func TestInjectionArchiveEmpty(t *testing.T) {
	archive, err := buildInjectionArchive(nil, "")
	if err != nil {
		t.Fatalf("buildInjectionArchive: %v", err)
	}
	if archive != nil {
		t.Error("expected no archive for an empty injection")
	}
}
