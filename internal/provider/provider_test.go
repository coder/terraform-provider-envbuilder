package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/coder/terraform-provider-envbuilder/testutil/registrytest"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

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

// testDependencies contain information about stuff the test depends on.
type testDependencies struct {
	BuilderImage string
	CacheRepo    string
	ExtraEnv     map[string]string
	Repo         testGitRepoSSH
}

// Config generates a valid Terraform config file from the dependencies.
func (d *testDependencies) Config(t testing.TB) string {
	t.Helper()

	tpl := `provider envbuilder {}
resource "envbuilder_cached_image" "test" {
  builder_image            = {{ quote .BuilderImage }}
	cache_repo               = {{ quote .CacheRepo }}
	extra_env                = {
	{{ range $k, $v := .ExtraEnv }}
		{{ quote $k }}: {{ quote $v }}
	{{ end }}
	}
	git_url                  = {{ quote .Repo.URL }}
	git_ssh_private_key_path = {{ quote .Repo.Key }}
	verbose                  = true
	workspace_folder         = {{ quote .Repo.Dir }}
}`

	fm := template.FuncMap{"quote": quote}
	var sb strings.Builder
	tmpl, err := template.New("envbuilder_cached_image").Funcs(fm).Parse(tpl)
	require.NoError(t, err)
	require.NoError(t, tmpl.Execute(&sb, d))
	return sb.String()
}

func quote(s string) string {
	return fmt.Sprintf("%q", s)
}

func setup(ctx context.Context, t testing.TB, files map[string]string) testDependencies {
	t.Helper()

	envbuilderImage := getEnvOrDefault("ENVBUILDER_IMAGE", "ghcr.io/coder/envbuilder-preview")
	envbuilderVersion := getEnvOrDefault("ENVBUILDER_VERSION", "latest")
	envbuilderImageRef := envbuilderImage + ":" + envbuilderVersion

	// TODO: envbuilder creates /.envbuilder/bin/envbuilder owned by root:root which we are unable to clean up.
	// This causes tests to fail.
	regDir := t.TempDir()
	reg := registrytest.New(t, regDir)

	repoDir := setupGitRepo(t, files)
	gitRepo := serveGitRepoSSH(ctx, t, repoDir)

	return testDependencies{
		BuilderImage: envbuilderImageRef,
		CacheRepo:    reg + "/test",
		ExtraEnv:     make(map[string]string),
		Repo:         gitRepo,
	}
}

func seedCache(ctx context.Context, t testing.TB, deps testDependencies) {
	t.Helper()

	t.Logf("seeding cache with %s", deps.CacheRepo)
	defer t.Logf("finished seeding cache with %s", deps.CacheRepo)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "init docker client")
	t.Cleanup(func() { _ = cli.Close() })

	ensureImage(ctx, t, cli, deps.BuilderImage)

	// Run envbuilder using this dir as a local layer cache
	ctr, err := cli.ContainerCreate(ctx, &container.Config{
		Image: deps.BuilderImage,
		Env: []string{
			"ENVBUILDER_CACHE_REPO=" + deps.CacheRepo,
			"ENVBUILDER_EXIT_ON_BUILD_FAILURE=true",
			"ENVBUILDER_INIT_SCRIPT=exit",
			"ENVBUILDER_PUSH_IMAGE=true",
			"ENVBUILDER_VERBOSE=true",
			"ENVBUILDER_GIT_URL=" + deps.Repo.URL,
			"ENVBUILDER_GIT_SSH_PRIVATE_KEY_PATH=/id_ed25519",
		},
		Labels: map[string]string{
			testContainerLabel: "true",
		},
	}, &container.HostConfig{
		NetworkMode: container.NetworkMode("host"),
		Binds: []string{
			deps.Repo.Key + ":/id_ed25519",
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

// quotedPrefix is a helper for asserting quoted strings.
func quotedPrefix(prefix string) func(string) error {
	return func(val string) error {
		trimmed := strings.Trim(val, `"`)
		if !strings.HasPrefix(trimmed, prefix) {
			return fmt.Errorf("expected value %q to have prefix %q", trimmed, prefix)
		}
		return nil
	}
}
