package runtime

import (
	"context"
	"io"
)

// Runtime executes containers from OCI image tarballs
type Runtime interface {
	// Run executes a container from the given image tarball
	Run(ctx context.Context, opts RunOptions) error
}

// RunOptions configures how to run the container
type RunOptions struct {
	// Path to the OCI image tarball
	ImagePath string

	// Script to execute (will be mounted read-only)
	ScriptPath string

	// Arguments to pass to the script
	ScriptArgs []string

	// Working directory to bind mount
	WorkDir string

	// Interactive mode (attach stdin/stdout/stderr)
	Interactive bool

	// Environment variables
	Env map[string]string

	// Stdin/stdout/stderr (optional, defaults to os.Std*)
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}
