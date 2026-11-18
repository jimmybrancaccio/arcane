package secretsource

import (
	"testing"

	"github.com/stretchr/testify/require"

	"getarcane.dev/types/v3"
)

func TestBindingDeliveryMappings(t *testing.T) {
	binding := Binding{Required: true, Mappings: []types.SecretMapping{
		{Key: "PASSWORD", Name: "database-password", Delivery: "file", Service: "database"},
		{Key: "TOKEN", Name: "API_TOKEN", Delivery: "environment", Service: "api"},
	}}
	result, err := mappedValues(binding, map[string]string{"PASSWORD": "password-value", "TOKEN": "token-value"})
	require.NoError(t, err)
	require.Len(t, result.Files, 1)
	require.Equal(t, uint32(0o400), result.Files[0].Mode)
	require.Equal(t, "database", result.Files[0].Service)
	require.Equal(t, "token-value", result.ServiceEnvironment["api"]["API_TOKEN"])
	version := result.Files[0].Version
	require.NotEmpty(t, version)
	unchanged, err := mappedValues(binding, map[string]string{"PASSWORD": "password-value", "TOKEN": "changed-token"})
	require.NoError(t, err)
	require.Equal(t, version, unchanged.Files[0].Version)
	changed, err := mappedValues(binding, map[string]string{"PASSWORD": "changed-password", "TOKEN": "token-value"})
	require.NoError(t, err)
	require.NotEqual(t, version, changed.Files[0].Version)
	require.Empty(t, result.Environment)
	_, err = mappedValues(binding, map[string]string{"PASSWORD": "password-value"})
	require.ErrorContains(t, err, "required key")
}

func TestBindingValidationAndStableHash(t *testing.T) {
	req := types.SecretBindingWrite{Owner: types.SecretOwner{Kind: "project", ID: "app"}, Mappings: []types.SecretMapping{{Key: "KEY", Name: "file", Delivery: "file"}}}
	require.ErrorContains(t, validateBinding(req), "Swarm workload")
	req.Owner = types.SecretOwner{Kind: "service", ID: "app", ClusterID: "cluster"}
	req.Mappings[0].Name = "../password"
	require.ErrorContains(t, validateBinding(req), "filename")
	req.Mappings[0].Name = "password"
	require.NoError(t, validateBinding(req))
	first := hashValues("salt", map[string]string{"A": "one", "B": "two"})
	require.Equal(t, first, hashValues("salt", map[string]string{"B": "two", "A": "one"}))
	require.NotEqual(t, first, hashValues("other-salt", map[string]string{"A": "one", "B": "two"}))
}
