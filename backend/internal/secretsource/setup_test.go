package secretsource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupPlanDescribesVariablesWithoutValues(t *testing.T) {
	variables := setupVariables(
		"services:\n  app:\n    image: app:${VERSION:-latest}\n    environment:\n      - PASSWORD=${PASSWORD:?required}\n",
		map[string]string{"PASSWORD": "private-local-value"},
		map[string]string{"PASSWORD": "private-remote-value"},
	)
	require.Len(t, variables, 2)
	require.Equal(t, "PASSWORD", variables[0].Key)
	require.True(t, variables[0].Differs)
	require.True(t, variables[0].Referenced)
	require.False(t, variables[1].HasLocalValue)
}
