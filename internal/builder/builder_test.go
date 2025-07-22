package builder

import (
	"context"
	"os"
	"testing"

	"chainguard.dev/apko/pkg/build/types"
)

func TestBuilder(t *testing.T) {
	ctx := context.Background()

	// Create temp directories
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create builder
	b := New(cacheDir, tmpDir)

	// Create a minimal config with Wolfi
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

	// Build the image
	tarPath, err := b.Build(ctx, config, "apko-shell-test:latest")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check that the tarball was created
	if _, err := os.Stat(tarPath); err != nil {
		t.Errorf("tarball not created at %s: %v", tarPath, err)
	}

	// Check file size is reasonable
	info, err := os.Stat(tarPath)
	if err == nil && info.Size() < 1000 {
		t.Errorf("tarball seems too small: %d bytes", info.Size())
	}
}
