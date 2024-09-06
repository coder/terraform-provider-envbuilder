package imgutil

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GetRemoteImage fetches the image manifest of the image.
func GetRemoteImage(imgRef string) (v1.Image, error) {
	ref, err := name.ParseReference(imgRef)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %w", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("check remote image: %w", err)
	}

	return img, nil
}

// ExtractEnvbuilderFromImage reads the image located at imgRef and extracts
// MagicBinaryLocation to destPath.
func ExtractEnvbuilderFromImage(ctx context.Context, imgRef, destPath string) error {
	needle := ".envbuilder/bin/envbuilder"
	img, err := GetRemoteImage(imgRef)
	if err != nil {
		return fmt.Errorf("check remote image: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("get image layers: %w", err)
	}

	// Check the layers in reverse order. The last layers are more likely to
	// include the binary.
	for i := len(layers) - 1; i >= 0; i-- {
		ul, err := layers[i].Uncompressed()
		if err != nil {
			return fmt.Errorf("get uncompressed layer: %w", err)
		}

		tr := tar.NewReader(ul)
		for {
			th, err := tr.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return fmt.Errorf("read tar header: %w", err)
			}

			name := filepath.Clean(th.Name)
			if th.Typeflag != tar.TypeReg {
				tflog.Debug(ctx, "skip non-regular file", map[string]any{"name": name, "layer_idx": i + 1})
				continue
			}

			if name != needle {
				tflog.Debug(ctx, "skip file", map[string]any{"name": name, "layer_idx": i + 1})
				continue
			}

			tflog.Debug(ctx, "found file", map[string]any{"name": name, "layer_idx": i + 1})
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return fmt.Errorf("create parent directories: %w", err)
			}
			destF, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("create dest file for writing: %w", err)
			}
			defer destF.Close()
			_, err = io.Copy(destF, tr)
			if err != nil {
				return fmt.Errorf("copy dest file from image: %w", err)
			}
			if err := destF.Close(); err != nil {
				return fmt.Errorf("close dest file: %w", err)
			}

			if err := os.Chmod(destPath, 0o755); err != nil {
				return fmt.Errorf("chmod file: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("extract envbuilder binary from image %q: %w", imgRef, os.ErrNotExist)
}
