package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chainguard-dev/clog"
	"github.com/joshrwolf/apko-shell/internal/runtime"
)

// Docker runtime implementation
type Docker struct {
	// Path to docker binary (default: "docker")
	dockerPath string
}

// New creates a new Docker runtime
func New() *Docker {
	return &Docker{
		dockerPath: "docker",
	}
}

// Run implements runtime.Runtime
func (d *Docker) Run(ctx context.Context, opts runtime.RunOptions) error {
	log := clog.FromContext(ctx)

	// Load the image from tarball
	imageID, err := d.loadImage(ctx, opts.ImagePath)
	if err != nil {
		return fmt.Errorf("loading image: %w", err)
	}
	log.Debug("loaded image", "id", imageID)

	// Build docker run command
	args := d.buildRunArgs(opts, imageID)

	// Create the command
	cmd := exec.CommandContext(ctx, d.dockerPath, args...)

	// Set up IO
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	// Run the container
	log.Debug("running container", "args", args)
	return cmd.Run()
}

// loadImage loads an OCI tarball and returns the image ID
func (d *Docker) loadImage(ctx context.Context, tarPath string) (string, error) {
	log := clog.FromContext(ctx)

	// docker load -i <tarball>
	cmd := exec.CommandContext(ctx, d.dockerPath, "load", "-i", tarPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker load failed: %w, output: %s", err, string(output))
	}

	// Parse output to find the image reference
	// Output format: "Loaded image: <name:tag>" or "Loaded image ID: sha256:..."
	outputStr := string(output)
	log.Debug("docker load output", "output", outputStr)

	// Look for "Loaded image: " in the output
	const prefix = "Loaded image: "
	idx := strings.Index(outputStr, prefix)
	if idx >= 0 {
		imageRef := strings.TrimSpace(outputStr[idx+len(prefix):])
		// Find end of line
		if nlIdx := strings.IndexAny(imageRef, "\n\r"); nlIdx >= 0 {
			imageRef = imageRef[:nlIdx]
		}
		return imageRef, nil
	}

	// Fallback: look for "Loaded image ID: sha256:..."
	const idPrefix = "Loaded image ID: "
	idx = strings.Index(outputStr, idPrefix)
	if idx >= 0 {
		imageID := strings.TrimSpace(outputStr[idx+len(idPrefix):])
		if nlIdx := strings.IndexAny(imageID, "\n\r"); nlIdx >= 0 {
			imageID = imageID[:nlIdx]
		}
		return imageID, nil
	}

	return "", fmt.Errorf("could not parse image reference from docker load output: %s", outputStr)
}

// buildRunArgs builds the docker run arguments
func (d *Docker) buildRunArgs(opts runtime.RunOptions, imageID string) []string {
	args := []string{"run", "--rm"}

	// Always keep stdin open
	args = append(args, "-i")

	// Add TTY for interactive mode
	if opts.Interactive {
		args = append(args, "-t")
	}

	// User mapping - use current UID:GID
	uid := os.Getuid()
	gid := os.Getgid()
	args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))

	// Working directory mount
	if opts.WorkDir != "" {
		// Convert to absolute path for Docker
		absWorkDir, err := filepath.Abs(opts.WorkDir)
		if err != nil {
			absWorkDir = opts.WorkDir // fallback to original
		}
		args = append(args, "-v", fmt.Sprintf("%s:/workspace:rw", absWorkDir))
		args = append(args, "-w", "/workspace")
	}

	// Script mount (read-only)
	if opts.ScriptPath != "" {
		// Convert to absolute path
		absPath, err := filepath.Abs(opts.ScriptPath)
		if err != nil {
			absPath = opts.ScriptPath // fallback to original
		}
		args = append(args, "-v", fmt.Sprintf("%s:/apko-shell/script:ro", absPath))
	}

	// Environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Image
	args = append(args, imageID)

	// Command to run
	if opts.ScriptPath != "" && !opts.Interactive {
		// Run the script with its arguments
		args = append(args, "/apko-shell/script")
		args = append(args, opts.ScriptArgs...)
	}
	// If interactive, use the default entrypoint from the image

	return args
}

// Available checks if docker is available
func (d *Docker) Available(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, d.dockerPath, "version", "--format", "json")
	return cmd.Run() == nil
}

// String returns the runtime name
func (d *Docker) String() string {
	return "docker"
}
