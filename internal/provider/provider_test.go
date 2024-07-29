// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"
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

func seedEnvbuilderCache(t *testing.T, image string, env map[string]string) {
	t.Helper()

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "setup docker test pool")
	require.NoError(t, pool.Client.Ping(), "could not connect to docker daemon")

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
