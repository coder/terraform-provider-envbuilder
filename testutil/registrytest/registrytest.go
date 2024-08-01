package registrytest

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

// New starts a new Docker registry listening on localhost.
// It will automatically shut down when the test finishes.
// It will store data in dir.
func New(t testing.TB, dir string) string {
	t.Helper()
	tempDir := t.TempDir()
	regHandler := registry.New(registry.WithBlobHandler(registry.NewDiskBlobHandler(tempDir)))
	regSrv := httptest.NewServer(regHandler)
	t.Cleanup(func() { regSrv.Close() })
	regSrvURL, err := url.Parse(regSrv.URL)
	require.NoError(t, err)
	return fmt.Sprintf("localhost:%s", regSrvURL.Port())
}
