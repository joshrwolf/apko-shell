package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"chainguard.dev/apko/pkg/build/types"
	"github.com/chainguard-dev/clog"
	"github.com/chainguard-dev/clog/slag"
	charmlog "github.com/charmbracelet/log"
	"github.com/joshrwolf/apko-shell/internal/builder"
	"github.com/joshrwolf/apko-shell/internal/runtime"
	"github.com/joshrwolf/apko-shell/internal/runtime/docker"
	"github.com/joshrwolf/apko-shell/internal/script"
	"github.com/spf13/cobra"
)

type options struct {
	logLevel slag.Level

	packages    []string
	interactive bool
	buildOnly   bool
	shell       string
	command     string
}

// setupLogging configures logging for the command
func (o *options) setupLogging(ctx context.Context) context.Context {
	l := charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		Level:           charmlog.Level(o.logLevel),
		ReportTimestamp: true,
	})
	ctx = clog.WithLogger(ctx, clog.New(l))
	slog.SetDefault(slog.New(l))
	return ctx
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		clog.FatalContextf(ctx, "error: %v", err)
	}
}

func run(ctx context.Context) error {
	opts := &options{}

	rootCmd := &cobra.Command{
		Use:   "apko-shell [script]",
		Short: "On-demand development environments using APK packages",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx = opts.setupLogging(ctx)
			cmd.SetContext(ctx)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run(cmd.Context(), args)
		},
	}

	// Define flags
	rootCmd.PersistentFlags().Var(&opts.logLevel, "log-level", "log level (debug, info, warn, error)")
	rootCmd.Flags().StringSliceVarP(&opts.packages, "packages", "p", nil, "APK packages to install")
	rootCmd.Flags().BoolVarP(&opts.interactive, "interactive", "i", false, "Start interactive shell")
	rootCmd.Flags().BoolVar(&opts.buildOnly, "build-only", false, "Build image only")
	rootCmd.Flags().StringVar(&opts.shell, "shell", "/bin/sh", "Shell to use")
	rootCmd.Flags().StringVarP(&opts.command, "command", "c", "", "Command to run (instead of script file)")

	// Merge shebang args if we're executing a script
	if err := mergeShebangArgs(rootCmd); err != nil {
		return fmt.Errorf("failed to parse shebang args: %w", err)
	}

	return rootCmd.ExecuteContext(ctx)
}

func (o *options) run(ctx context.Context, args []string) error {
	log := clog.FromContext(ctx)

	log.Debug("starting apko-shell", "args", args, "packages", o.packages)

	// Get cache directories
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("getting cache dir: %w", err)
	}
	cacheDir = filepath.Join(cacheDir, "apko-shell")

	tmpDir := filepath.Join(os.TempDir(), "apko-shell")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	// Detect runtime
	rt, err := detectRuntime(ctx)
	if err != nil {
		return err
	}
	log.Debug("detected runtime", "runtime", rt)

	// Create builder
	b := builder.New(cacheDir, tmpDir)

	// Build the appropriate image configuration
	var imageConfig *types.ImageConfiguration
	var scriptPath string
	var scriptArgs []string
	workDir := "."

	// Handle inline command mode
	if o.command != "" {
		log.Debug("running inline command", "command", o.command)

		// Write command to a temporary script file
		tmpFile, err := os.CreateTemp("", "apko-shell-inline-*.sh")
		if err != nil {
			return fmt.Errorf("creating temp script: %w", err)
		}
		scriptPath = tmpFile.Name()
		defer os.Remove(scriptPath)

		// Write shebang and command
		if _, err := fmt.Fprintf(tmpFile, "#!%s\n%s\n", o.shell, o.command); err != nil {
			tmpFile.Close()
			return fmt.Errorf("writing inline script: %w", err)
		}
		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("closing temp script: %w", err)
		}

		// Make it executable
		if err := os.Chmod(scriptPath, 0o755); err != nil {
			return fmt.Errorf("chmod temp script: %w", err)
		}

		imageConfig = &types.ImageConfiguration{
			Contents: types.ImageContents{
				Packages: o.packages,
			},
			Cmd: o.shell,
		}

		// Use current directory as working directory
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	} else if len(args) > 0 {
		// When invoked as shebang interpreter: #!/usr/bin/env apko-shell
		// args[0] will be the script path, remaining args are script arguments
		scriptPath = args[0]
		scriptArgs = args[1:]

		log.Debug("running script", "path", scriptPath, "args", scriptArgs)

		// Parse the script to get config
		f, err := os.Open(scriptPath)
		if err != nil {
			return fmt.Errorf("opening script: %w", err)
		}
		defer f.Close()

		cfg, err := script.Parse(f)
		if err != nil {
			return fmt.Errorf("parsing script: %w", err)
		}

		// Build image configuration from script
		if cfg.ImageConfig != nil {
			// Use PEP 723 config as base
			imageConfig = cfg.ImageConfig
			// Merge in CLI packages if any
			if len(o.packages) > 0 {
				imageConfig.Contents.Packages = append(imageConfig.Contents.Packages, o.packages...)
			}
		} else {
			// No PEP 723, build from CLI flags
			imageConfig = &types.ImageConfiguration{
				Contents: types.ImageContents{
					Packages: o.packages,
				},
				Cmd: o.shell,
			}
		}

		// Set working directory to script's directory
		workDir = filepath.Dir(scriptPath)
	} else if len(o.packages) > 0 {
		// Direct invocation: apko-shell -p curl,jq
		log.Debug("direct package invocation", "packages", o.packages)

		imageConfig = &types.ImageConfiguration{
			Contents: types.ImageContents{
				Packages: o.packages,
			},
			Cmd: o.shell,
		}

		// Use current directory as working directory
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

	} else {
		return fmt.Errorf("either provide a script or use -p to specify packages")
	}

	// Add default repositories if none specified
	if len(imageConfig.Contents.RuntimeRepositories) == 0 {
		imageConfig.Contents.RuntimeRepositories = []string{
			"https://packages.wolfi.dev/os",
		}
		imageConfig.Contents.Keyring = []string{
			"https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
		}
	}

	// Add busybox if no packages specified
	if len(imageConfig.Contents.Packages) == 0 {
		imageConfig.Contents.Packages = []string{
			"busybox",
		}
	}

	// Ensure the shell package is installed
	shellPackages := map[string]string{
		"/bin/sh":   "busybox",
		"/bin/bash": "bash",
	}
	if shellPkg, ok := shellPackages[o.shell]; ok {
		// Check if shell package is already in the list
		found := false
		for _, pkg := range imageConfig.Contents.Packages {
			if pkg == shellPkg {
				found = true
				break
			}
		}
		if !found {
			imageConfig.Contents.Packages = append(imageConfig.Contents.Packages, shellPkg)
		}
	}

	// Log the final merged configuration for debugging
	if configJSON, err := json.MarshalIndent(imageConfig, "", "  "); err == nil {
		log.Debug("final image configuration", "config", string(configJSON))
	}

	// Build the image
	log.Info("building image", "packages", imageConfig.Contents.Packages)
	tarPath, err := b.Build(ctx, imageConfig, "apko-shell:latest")
	if err != nil {
		return fmt.Errorf("building image: %w", err)
	}

	// If build-only, we're done
	if o.buildOnly {
		fmt.Println(tarPath)
		return nil
	}

	// If we have a script, create a rendered version with the correct shebang
	var renderedScriptPath string
	if scriptPath != "" {
		renderedScriptPath, err = o.renderScript(ctx, scriptPath, o.shell)
		if err != nil {
			return fmt.Errorf("rendering script: %w", err)
		}
		log.Debug("rendered script", "original", scriptPath, "rendered", renderedScriptPath)
		defer os.Remove(renderedScriptPath) // Clean up temp script
	}

	// Run the container
	runOpts := runtime.RunOptions{
		ImagePath:   tarPath,
		ScriptPath:  renderedScriptPath,
		ScriptArgs:  scriptArgs,
		WorkDir:     workDir,
		Interactive: o.interactive,
	}

	log.Info("running container", "interactive", runOpts.Interactive)
	return rt.Run(ctx, runOpts)
}

// mergeShebangArgs checks if we're executing a script and merges its shebang args
func mergeShebangArgs(cmd *cobra.Command) error {
	// Check if first arg looks like a script path
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		return nil // Not a script invocation
	}

	scriptPath := os.Args[1]
	info, err := os.Stat(scriptPath)
	if err != nil || info.IsDir() {
		return nil // Not a file
	}

	// Parse script for shebang args
	f, err := os.Open(scriptPath)
	if err != nil {
		return fmt.Errorf("opening script: %w", err)
	}
	defer f.Close()

	cfg, err := script.Parse(f)
	if err != nil {
		return fmt.Errorf("parsing script: %w", err)
	}

	if len(cfg.ShebangArgs) == 0 {
		return nil // No shebang args to merge
	}

	// Build merged args: shebang flags + original args
	var mergedArgs []string

	// Expand shebang args (each may contain multiple flags)
	for _, arg := range cfg.ShebangArgs {
		mergedArgs = append(mergedArgs, strings.Fields(arg)...)
	}

	// Append original args (script path + any script arguments)
	mergedArgs = append(mergedArgs, os.Args[1:]...)

	// Tell Cobra to use the merged args
	cmd.SetArgs(mergedArgs)

	return nil
}

// renderScript creates a temporary script with the correct shebang
func (o *options) renderScript(ctx context.Context, scriptPath, shell string) (string, error) {
	log := clog.FromContext(ctx)
	// Open the original script
	input, err := os.Open(scriptPath)
	if err != nil {
		return "", fmt.Errorf("opening script: %w", err)
	}
	defer input.Close()

	// Create a temporary file for the rendered script
	tmpFile, err := os.CreateTemp("", "apko-shell-script-*.sh")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	log.Debug("created temp script file", "path", tmpPath)

	// Write the correct shebang
	log.Debug("writing shebang", "shell", shell)
	if _, err := fmt.Fprintf(tmpFile, "#!%s\n", shell); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing shebang: %w", err)
	}

	// Copy the rest of the script, skipping apko-shell shebang lines
	scanner := bufio.NewScanner(input)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip apko-shell shebang lines
		if lineNum == 1 && strings.HasPrefix(line, "#!") && strings.Contains(line, "apko-shell") {
			continue
		}
		if strings.HasPrefix(line, "#!apko-shell") {
			continue
		}

		// Write all other lines
		if _, err := fmt.Fprintln(tmpFile, line); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("writing line: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("reading script: %w", err)
	}

	// Make the script executable
	if err := tmpFile.Chmod(0o755); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("closing temp file: %w", err)
	}

	// Debug: log the first few lines of the rendered script
	if content, err := os.ReadFile(tmpPath); err == nil {
		lines := strings.Split(string(content), "\n")
		preview := strings.Join(lines[:min(5, len(lines))], "\n")
		log.Debug("rendered script preview", "content", preview)
	}

	return tmpPath, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// detectRuntime returns an available container runtime
func detectRuntime(ctx context.Context) (runtime.Runtime, error) {
	// Try Docker
	d := docker.New()
	if d.Available(ctx) {
		return d, nil
	}

	// Future: try podman, nerdctl, etc.

	return nil, fmt.Errorf("no container runtime found (docker not available)")
}
