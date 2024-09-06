package registrytest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
)

// New starts a new Docker registry listening on localhost.
// It will automatically shut down when the test finishes.
// It will store data in dir.
func New(t testing.TB, dir string, mws ...func(http.Handler) http.Handler) string {
	t.Helper()
	regHandler := registry.New(registry.WithBlobHandler(registry.NewDiskBlobHandler(dir)))
	for _, mw := range mws {
		regHandler = mw(regHandler)
	}
	regSrv := httptest.NewServer(regHandler)
	t.Cleanup(func() { regSrv.Close() })
	regSrvURL, err := url.Parse(regSrv.URL)
	require.NoError(t, err)
	return fmt.Sprintf("localhost:%s", regSrvURL.Port())
}

func BasicAuthMW(t testing.TB, username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if username != "" || password != "" {
				authUser, authPass, ok := r.BasicAuth()
				if !ok || username != authUser || password != authPass {
					t.Logf("basic auth failed: got user %q, pass %q", authUser, authPass)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
