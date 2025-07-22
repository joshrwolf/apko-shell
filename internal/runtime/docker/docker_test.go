package docker

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"chainguard.dev/apko/pkg/build/types"
	"github.com/joshrwolf/apko-shell/internal/builder"
	"github.com/joshrwolf/apko-shell/internal/runtime"
)

func TestDockerRuntime(t *testing.T) {
	ctx := context.Background()

	// Check if Docker is available
	d := New()
	if !d.Available(ctx) {
		t.Logf("Docker path: %s", d.dockerPath)
		// Try running the command manually to see the error
		cmd := exec.Command(d.dockerPath, "version", "--format", "json")
		output, err := cmd.CombinedOutput()
		t.Logf("Docker check error: %v, output: %s", err, output)
		t.Skip("Docker not available")
	}

	// Create a test script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	scriptContent := `#!/bin/sh
echo "Hello from apko-shell!"
echo "Args: $@"
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatal(err)
	}

	// Build a test image
	b := builder.New(tmpDir, tmpDir)
	config := &types.ImageConfiguration{
		Contents: types.ImageContents{
			RuntimeRepositories: []string{
				"https://packages.wolfi.dev/os",
			},
			Keyring: []string{
				"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
			},
			Packages: []string{
				"wolfi-base",
			},
		},
		Cmd: "/bin/sh",
	}

	tarPath, err := b.Build(ctx, config, "apko-shell-test:latest")
	if err != nil {
		t.Fatalf("failed to build image: %v", err)
	}

	// Test running the script
	var stdout, stderr bytes.Buffer
	opts := runtime.RunOptions{
		ImagePath:  tarPath,
		ScriptPath: scriptPath,
		ScriptArgs: []string{"arg1", "arg2"},
		WorkDir:    tmpDir,
		Stdout:     &stdout,
		Stderr:     &stderr,
	}

	if err := d.Run(ctx, opts); err != nil {
		t.Errorf("Run() error = %v, stderr = %s", err, stderr.String())
	}

	// Check output
	output := stdout.String()
	if output == "" {
		t.Error("expected output, got none")
	}
	t.Logf("Script output:\n%s", output)

	// Verify output contains expected text
	if !bytes.Contains(stdout.Bytes(), []byte("Hello from apko-shell!")) {
		t.Errorf("output missing expected text, got: %s", output)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Args: arg1 arg2")) {
		t.Errorf("output missing args, got: %s", output)
	}
}
