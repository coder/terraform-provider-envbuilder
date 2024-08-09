package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nolint:gosec // Throw-away key for testing. DO NOT REUSE.
const (
	testSSHKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCtxz9h0yXzi/HqZBpSkA2xFo28v5W8O4HimI0ZzNpQkwAAAKhv/+X2b//l
9gAAAAtzc2gtZWQyNTUxOQAAACCtxz9h0yXzi/HqZBpSkA2xFo28v5W8O4HimI0ZzNpQkw
AAAED/G0HuohvSa8q6NzkZ+wRPW0PhPpo9Th8fvcBQDaxCia3HP2HTJfOL8epkGlKQDbEW
jby/lbw7geKYjRnM2lCTAAAAInRlcnJhZm9ybS1wcm92aWRlci1lbnZidWlsZGVyLXRlc3
QBAgM=
-----END OPENSSH PRIVATE KEY-----`
	testSSHPubKey = `ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK3HP2HTJfOL8epkGlKQDbEWjby/lbw7geKYjRnM2lCT terraform-provider-envbuilder-test`
)

func setupGitRepo(t testing.TB, files map[string]string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "repo")

	writeFiles(t, dir, files)

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

	return dir
}

func writeFiles(t testing.TB, destPath string, files map[string]string) {
	t.Helper()

	err := os.MkdirAll(destPath, 0o755)
	require.NoError(t, err, "create dest path")

	for relPath, content := range files {
		absPath := filepath.Join(destPath, relPath)
		d := filepath.Dir(absPath)
		bs := []byte(content)
		require.NoError(t, os.MkdirAll(d, 0o755))
		require.NoError(t, os.WriteFile(absPath, bs, 0o644))
		t.Logf("wrote %d bytes to %s", len(bs), absPath)
	}
}

type testGitRepoSSH struct {
	Dir string
	URL string
	Key string
}

func serveGitRepoSSH(ctx context.Context, t testing.TB, dir string) testGitRepoSSH {
	t.Helper()

	sshDir := filepath.Join(t.TempDir(), "ssh")
	require.NoError(t, os.Mkdir(sshDir, 0o700))

	keyPath := filepath.Join(sshDir, "id_ed25519")
	require.NoError(t, os.WriteFile(keyPath, []byte(testSSHKey), 0o600))

	// Start SSH server
	addr := startSSHServer(ctx, t)

	// Serve git repo
	repoURL := "ssh://" + addr + dir
	return testGitRepoSSH{
		Dir: dir,
		URL: repoURL,
		Key: keyPath,
	}
}

func startSSHServer(ctx context.Context, t testing.TB) string {
	t.Helper()

	s := &ssh.Server{
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true // Allow all keys.
		},
		Handler: func(s ssh.Session) {
			t.Logf("session started: %s", s.RawCommand())

			args := s.Command()
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)

			in, err := cmd.StdinPipe()
			assert.NoError(t, err, "stdin pipe")
			out, err := cmd.StdoutPipe()
			assert.NoError(t, err, "stdout pipe")
			err = cmd.Start()
			if err != nil {
				t.Logf("command failed: %s", err)
				return
			}

			go func() {
				_, _ = io.Copy(in, s)
				_ = in.Close()
			}()
			outDone := make(chan struct{})
			go func() {
				defer close(outDone)
				_, _ = io.Copy(s, out)
				_ = out.Close()
				_ = s.CloseWrite()
			}()
			t.Cleanup(func() {
				_ = in.Close()
				_ = out.Close()
				<-outDone
				_ = cmd.Process.Kill()
			})
			err = cmd.Wait()
			if err != nil {
				t.Logf("command failed: %s", err)
			}

			t.Logf("session ended: %s", s.RawCommand())

			err = s.Exit(cmd.ProcessState.ExitCode())
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Errorf("session exit failed: %s", err)
				}
			}
		},
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "localhost:0")
	require.NoError(t, err, "listen")

	go func() {
		err := s.Serve(ln)
		if !errors.Is(err, ssh.ErrServerClosed) {
			require.NoError(t, err)
		}
	}()
	t.Cleanup(func() {
		_ = s.Close()
		_ = ln.Close()
	})

	return ln.Addr().String()
}
