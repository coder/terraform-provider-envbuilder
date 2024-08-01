// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/mafredri/terraform-provider-envbuilder/testutil/registrytest"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

const (
	testContainerLabel = "terraform-provider-envbuilder-test"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"envbuilder": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}

type testDependencies struct {
	BuilderImage string
	RepoDir      string
	CacheRepo    string
}

func setup(t testing.TB, files map[string]string) testDependencies {
	t.Helper()

	envbuilderImage := getEnvOrDefault("ENVBUILDER_IMAGE", "ghcr.io/coder/envbuilder-preview")
	envbuilderVersion := getEnvOrDefault("ENVBUILDER_VERSION", "latest")
	envbuilderImageRef := envbuilderImage + ":" + envbuilderVersion

	// TODO: envbuilder creates /.envbuilder/bin/envbuilder owned by root:root which we are unable to clean up.
	// This causes tests to fail.
	repoDir := t.TempDir()
	regDir := t.TempDir()
	reg := registrytest.New(t, regDir)
	writeFiles(t, files, repoDir)
	return testDependencies{
		BuilderImage: envbuilderImageRef,
		CacheRepo:    reg + "/test",
		RepoDir:      repoDir,
	}
}

func seedCache(ctx context.Context, t testing.TB, deps testDependencies) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "init docker client")
	t.Cleanup(func() { _ = cli.Close() })
	ensureImage(ctx, t, cli, deps.BuilderImage)
	// Run envbuilder using this dir as a local layer cache
	ctr, err := cli.ContainerCreate(ctx, &container.Config{
		Image: deps.BuilderImage,
		Env: []string{
			"ENVBUILDER_CACHE_REPO=" + deps.CacheRepo,
			"ENVBUILDER_DEVCONTAINER_DIR=" + deps.RepoDir,
			"ENVBUILDER_EXIT_ON_BUILD_FAILURE=true",
			"ENVBUILDER_INIT_SCRIPT=exit",
			// FIXME: Enabling this options causes envbuilder to add its binary to the image under the path
			// /.envbuilder/bin/envbuilder. This file will have ownership root:root and permissions 0o755.
			// Because of this, t.Cleanup() will be unable to delete the temp dir, causing the test to fail.
			// "ENVBUILDER_PUSH_IMAGE=true",
		},
		Labels: map[string]string{
			testContainerLabel: "true",
		}}, &container.HostConfig{
		NetworkMode: container.NetworkMode("host"),
		Binds:       []string{deps.RepoDir + ":" + deps.RepoDir},
	}, nil, nil, "")
	require.NoError(t, err, "failed to run envbuilder to seed cache")
	t.Cleanup(func() {
		_ = cli.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
	})
	err = cli.ContainerStart(ctx, ctr.ID, container.StartOptions{})
	require.NoError(t, err)

	rawLogs, err := cli.ContainerLogs(ctx, ctr.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	require.NoError(t, err)
	defer rawLogs.Close()
	scanner := bufio.NewScanner(rawLogs)
SCANLOGS:
	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "envbuilder did not finish running in time")
		default:
			if !scanner.Scan() {
				require.Fail(t, "envbuilder did not run successfully")
			}
			log := scanner.Text()
			t.Logf("envbuilder: %s", log)
			if strings.Contains(log, "=== Running the init command") {
				break SCANLOGS
			}
		}
	}

}

func getEnvOrDefault(env, defVal string) string {
	if val := os.Getenv(env); val != "" {
		return val
	}
	return defVal
}

func writeFiles(t testing.TB, files map[string]string, destPath string) {
	for relPath, content := range files {
		absPath := filepath.Join(destPath, relPath)
		d := filepath.Dir(absPath)
		bs := []byte(content)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(absPath, bs, 0o644))
		t.Logf("wrote %d bytes to %s", len(bs), absPath)
	}
}

func ensureImage(ctx context.Context, t testing.TB, cli *client.Client, ref string) {
	t.Helper()

	t.Logf("ensuring image %q", ref)
	images, err := cli.ImageList(ctx, image.ListOptions{})
	require.NoError(t, err, "list images")
	for _, img := range images {
		if slices.Contains(img.RepoTags, ref) {
			t.Logf("image %q found locally, not pulling", ref)
			return
		}
	}
	t.Logf("image %s not found locally, attempting to pull", ref)
	resp, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	require.NoError(t, err)
	_, err = io.ReadAll(resp)
	require.NoError(t, err)
}
