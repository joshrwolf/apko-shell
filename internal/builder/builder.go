package builder

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"chainguard.dev/apko/pkg/apk/apk"
	"chainguard.dev/apko/pkg/build"
	"chainguard.dev/apko/pkg/build/oci"
	"chainguard.dev/apko/pkg/build/types"
	"chainguard.dev/apko/pkg/tarfs"
	"github.com/chainguard-dev/clog"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// Builder builds OCI images from apko configurations
type Builder struct {
	cacheDir string
	tmpDir   string
}

// New creates a new Builder
func New(cacheDir, tmpDir string) *Builder {
	return &Builder{
		cacheDir: cacheDir,
		tmpDir:   tmpDir,
	}
}

// Build builds an OCI image from the given configuration and returns the path to the tarball
func (b *Builder) Build(ctx context.Context, config *types.ImageConfiguration, tag string) (string, error) {
	log := clog.FromContext(ctx)

	// Default to host architecture
	arch := types.ParseArchitecture(runtime.GOARCH)

	// Create build options
	opts := []build.Option{
		build.WithImageConfiguration(*config),
		build.WithArch(arch),
		build.WithCache(b.cacheDir, false, apk.NewCache(true)),
		build.WithTempDir(b.tmpDir),
	}

	// Create build context
	bc, err := build.New(ctx, tarfs.New(), opts...)
	if err != nil {
		return "", fmt.Errorf("creating build context: %w", err)
	}

	// Build the image filesystem
	log.Info("building image filesystem")
	if err := bc.BuildImage(ctx); err != nil {
		return "", fmt.Errorf("building image: %w", err)
	}

	// Create layers
	log.Info("creating image layers")
	layers, err := bc.BuildLayers(ctx)
	if err != nil {
		return "", fmt.Errorf("building layers: %w", err)
	}

	// Build OCI image from layers
	log.Info("building OCI image")
	img, err := oci.BuildImageFromLayers(
		ctx,
		empty.Image,
		layers,
		bc.ImageConfiguration(),
		time.Now(),
		arch,
	)
	if err != nil {
		return "", fmt.Errorf("building image from layers: %w", err)
	}

	// Generate output path
	outputPath := filepath.Join(b.tmpDir, fmt.Sprintf("apko-shell-%d.tar", time.Now().Unix()))

	// Write image to tarball
	log.Info("writing image to tarball", "path", outputPath)
	if err := b.writeImageTarball(img, tag, outputPath); err != nil {
		return "", fmt.Errorf("writing tarball: %w", err)
	}

	return outputPath, nil
}

// writeImageTarball writes an OCI image to a tarball file
func (b *Builder) writeImageTarball(img v1.Image, tag, outputPath string) error {
	// Parse the tag
	ref, err := name.NewTag(tag)
	if err != nil {
		return fmt.Errorf("parsing tag %q: %w", tag, err)
	}

	// Write to file
	if err := tarball.WriteToFile(outputPath, ref, img); err != nil {
		return fmt.Errorf("writing tarball: %w", err)
	}

	return nil
}
