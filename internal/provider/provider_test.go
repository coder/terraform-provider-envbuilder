// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bufio"
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/docker/docker/api/types/container"
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

// envs turns a map[string]string of envs to a slice of string in the form k1=v1,k2=v2,...
func envs(m map[string]string) (ss []string) {
	var sb strings.Builder
	for k, v := range m {
		_, _ = sb.WriteString(k)
		_, _ = sb.WriteRune('=')
		_, _ = sb.WriteString(v)
		ss = append(ss, sb.String())
		sb.Reset()
	}
	return ss
}

type testDependencies struct {
	BuilderImage string
	RepoDir      string
	CacheRepo    string
}

func setup(ctx context.Context, t testing.TB, files map[string]string) testDependencies {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "init docker client")
	t.Cleanup(func() { _ = cli.Close() })

	// TODO: envbuilder creates /.envbuilder/bin/envbuilder owned by root:root which we are unable to clean up.
	// This causes tests to fail.
	repoDir := t.TempDir()
	cacheRepo := runLocalRegistry(t)
	writeFiles(t, files, repoDir)

	envbuilderImage := getEnvOrDefault("ENVBUILDER_IMAGE", "ghcr.io/coder/envbuilder-preview")
	envbuilderVersion := getEnvOrDefault("ENVBUILDER_VERSION", "latest")
	refStr := envbuilderImage + ":" + envbuilderVersion
	// Run envbuilder using this dir as a local layer cache
	ctr, err := cli.ContainerCreate(ctx, &container.Config{
		Image: refStr,
		Env: []string{
			"ENVBUILDER_CACHE_REPO=" + cacheRepo,
			"ENVBUILDER_DEVCONTAINER_DIR=" + repoDir,
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
		Binds:       []string{repoDir + ":" + repoDir},
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

	return testDependencies{
		BuilderImage: refStr,
		CacheRepo:    cacheRepo,
		RepoDir:      repoDir,
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

func runLocalRegistry(t testing.TB) string {
	t.Helper()
	tempDir := t.TempDir()
	regHandler := registry.New(registry.WithBlobHandler(registry.NewDiskBlobHandler(tempDir)))
	regSrv := httptest.NewServer(regHandler)
	t.Cleanup(func() { regSrv.Close() })
	regSrvURL, err := url.Parse(regSrv.URL)
	require.NoError(t, err)
	return fmt.Sprintf("localhost:%s/test", regSrvURL.Port())
}
