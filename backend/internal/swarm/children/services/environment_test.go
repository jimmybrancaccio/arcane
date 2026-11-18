package services

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedEnvironmentRestoresOnlyMaskedSecretValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.MarshalWrite(w, swarm.Service{Spec: swarm.ServiceSpec{TaskTemplate: swarm.TaskSpec{ContainerSpec: &swarm.ContainerSpec{
			Env:    []string{"PASSWORD=sensitive-value", "PUBLIC=public-value"},
			Labels: map[string]string{"getarcane.app/secret-keys": `["PASSWORD"]`},
		}}}}))
	}))
	t.Cleanup(server.Close)
	svc := NewService(newSettingsDockerService(t, newSwarmTestDockerClient(t, server)), func(context.Context) error { return nil }, nil, testSecretHooks())
	spec := swarm.ServiceSpec{TaskTemplate: swarm.TaskSpec{ContainerSpec: &swarm.ContainerSpec{Env: []string{"PASSWORD=[REDACTED]", "PUBLIC=edited-value"}}}}
	require.NoError(t, svc.restoreManagedEnvironment(t.Context(), "service-id", &spec))
	require.Equal(t, []string{"PASSWORD=sensitive-value", "PUBLIC=edited-value"}, spec.TaskTemplate.ContainerSpec.Env)
	spec.TaskTemplate.ContainerSpec.Env = []string{"PASSWORD=explicit-replacement"}
	require.NoError(t, svc.restoreManagedEnvironment(t.Context(), "service-id", &spec))
	require.Equal(t, []string{"PASSWORD=explicit-replacement"}, spec.TaskTemplate.ContainerSpec.Env)
}
