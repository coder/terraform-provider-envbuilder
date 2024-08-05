// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/coder/terraform-provider-envbuilder/testutil/registrytest"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
)

const (
	testContainerLabel = "terraform-provider-envbuilder-test"
)

// nolint:gosec // Throw-away key for testing. DO NOT REUSE.
const (
	testSSHHostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDBC7DRHALRN3JJrkDQETyL2Vg5O6QsWTE2YWAt9bIZiAAAAKj5O8yU+TvM
lAAAAAtzc2gtZWQyNTUxOQAAACDBC7DRHALRN3JJrkDQETyL2Vg5O6QsWTE2YWAt9bIZiA
AAAED9b0qGgjoDx9YiyCHGBE6ogdnD6IbQsgfaFDI0aE+x3cELsNEcAtE3ckmuQNARPIvZ
WDk7pCxZMTZhYC31shmIAAAAInRlcnJhZm9ybS1wcm92aWRlci1lbnZidWlsZGVyLXRlc3
QBAgM=
-----END OPENSSH PRIVATE KEY-----`
	testSSHHostPubKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMELsNEcAtE3ckmuQNARPIvZWDk7pCxZMTZhYC31shmI terraform-provider-envbuilder-test`
	testSSHUserKey    = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCtxz9h0yXzi/HqZBpSkA2xFo28v5W8O4HimI0ZzNpQkwAAAKhv/+X2b//l
9gAAAAtzc2gtZWQyNTUxOQAAACCtxz9h0yXzi/HqZBpSkA2xFo28v5W8O4HimI0ZzNpQkw
AAAED/G0HuohvSa8q6NzkZ+wRPW0PhPpo9Th8fvcBQDaxCia3HP2HTJfOL8epkGlKQDbEW
jby/lbw7geKYjRnM2lCTAAAAInRlcnJhZm9ybS1wcm92aWRlci1lbnZidWlsZGVyLXRlc3
QBAgM=
-----END OPENSSH PRIVATE KEY-----`
	testSSHUserPubKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK3HP2HTJfOL8epkGlKQDbEWjby/lbw7geKYjRnM2lCT terraform-provider-envbuilder-test`
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
	GitImage     string
	SSHDir       string
}

func setup(t testing.TB, files map[string]string) testDependencies {
	t.Helper()

	envbuilderImage := getEnvOrDefault("ENVBUILDER_IMAGE", "ghcr.io/coder/envbuilder-preview")
	envbuilderVersion := getEnvOrDefault("ENVBUILDER_VERSION", "latest")
	envbuilderImageRef := envbuilderImage + ":" + envbuilderVersion
	gitImageRef := "rockstorm/git-server:2.45"

	// TODO: envbuilder creates /.envbuilder/bin/envbuilder owned by root:root which we are unable to clean up.
	// This causes tests to fail.
	repoDir := t.TempDir()
	regDir := t.TempDir()
	reg := registrytest.New(t, regDir)
	writeFiles(t, files, repoDir)
	initGitRepo(t, repoDir)

	sshDir := t.TempDir()
	writeFiles(t, map[string]string{
		"id_ed25519":      testSSHUserKey,
		"authorized_keys": testSSHUserPubKey,
	}, sshDir)

	return testDependencies{
		BuilderImage: envbuilderImageRef,
		CacheRepo:    reg + "/test",
		RepoDir:      repoDir,
		GitImage:     gitImageRef,
		SSHDir:       sshDir,
	}
}

func seedCache(ctx context.Context, t testing.TB, deps testDependencies) {
	t.Helper()

	t.Logf("seeding cache with %s", deps.CacheRepo)
	defer t.Logf("finished seeding cache with %s", deps.CacheRepo)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "init docker client")
	t.Cleanup(func() { _ = cli.Close() })

	ensureImage(ctx, t, cli, deps.GitImage)
	ensureImage(ctx, t, cli, deps.BuilderImage)

	// TODO(mafredri): Use a dynamic port?
	sshPort := "2222"

	gitCtr, err := cli.ContainerCreate(ctx, &container.Config{
		Image: deps.GitImage,
		Env: []string{
			"SSH_AUTH_METHODS=publickey",
		},
		Labels: map[string]string{
			testContainerLabel: "true",
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"22/tcp": []nat.PortBinding{{HostIP: "localhost", HostPort: sshPort}},
		},
		Binds: []string{
			deps.RepoDir + ":/srv/git/repo.git",
			deps.SSHDir + ":/home/git/.ssh",
		},
	}, nil, nil, "")
	require.NoError(t, err, "failed to run git server")
	t.Cleanup(func() {
		_ = cli.ContainerRemove(ctx, gitCtr.ID, container.RemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
	})
	err = cli.ContainerStart(ctx, gitCtr.ID, container.StartOptions{})
	require.NoError(t, err)

	// Run envbuilder using this dir as a local layer cache
	ctr, err := cli.ContainerCreate(ctx, &container.Config{
		Image: deps.BuilderImage,
		Env: []string{
			"ENVBUILDER_CACHE_REPO=" + deps.CacheRepo,
			"ENVBUILDER_EXIT_ON_BUILD_FAILURE=true",
			"ENVBUILDER_INIT_SCRIPT=exit",
			"ENVBUILDER_PUSH_IMAGE=true",
			"ENVBUILDER_VERBOSE=true",
			fmt.Sprintf("ENVBUILDER_GIT_URL=ssh://git@localhost:%s/srv/git/repo.git", sshPort),
			"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=/tmp/ssh/id_ed25519",
		},
		Labels: map[string]string{
			testContainerLabel: "true",
		},
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode("host"),
		Binds: []string{
			deps.SSHDir + ":/tmp/ssh",
		},
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
	t.Helper()

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
		if strings.HasSuffix(ref, ":latest") {
			t.Logf("always pull latest")
			break
		} else if slices.Contains(img.RepoTags, ref) {
			t.Logf("image %q found locally, not pulling", ref)
			return
		}
	}
	t.Logf("attempting to pull image %q", ref)
	resp, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	require.NoError(t, err)
	_, err = io.ReadAll(resp)
	require.NoError(t, err)
}

func initGitRepo(t testing.TB, dir string) {
	t.Helper()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{
			DefaultBranch: plumbing.ReferenceName("refs/heads/main"),
		},
	})
	require.NoError(t, err, "init git repo")
	wt, err := repo.Worktree()
	require.NoError(t, err, "get worktree")
	_, err = wt.Add(".")
	require.NoError(t, err, "add files")
	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@coder.com",
		},
	})
	require.NoError(t, err, "commit files")
	t.Logf("initialized git repo at %s", dir)
}
