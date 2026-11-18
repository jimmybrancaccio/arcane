package swarm

import (
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/require"

	"getarcane.dev/types/v3"
)

func TestInfisicalEnvironmentPreservesUnrelatedServiceConfiguration(t *testing.T) {
	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{Name: "example_api", Labels: map[string]string{types.SwarmStackNamespaceLabel: "example"}},
		TaskTemplate: swarm.TaskSpec{ContainerSpec: &swarm.ContainerSpec{
			Image: "example:latest", Env: []string{"KEEP=unchanged", "PASSWORD=old", "REMOVED=old"}, Labels: map[string]string{"getarcane.app/secret-keys": `["PASSWORD","REMOVED"]`},
		}},
	}
	result, err := (&SwarmService{}).prepareSecretSpec(t.Context(), spec, types.SecretResolution{
		Environment: map[string]string{"PASSWORD": "new"}, ServiceEnvironment: map[string]map[string]string{"api": {"TOKEN": "scoped"}}, Hashes: map[string]string{"binding": "version"},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"KEEP=unchanged", "PASSWORD=new", "TOKEN=scoped"}, result.TaskTemplate.ContainerSpec.Env)
	require.Equal(t, "example:latest", result.TaskTemplate.ContainerSpec.Image)
}
