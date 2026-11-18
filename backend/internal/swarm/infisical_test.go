package swarm

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/libtnb/sqlite"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"kit.getarcane.app/docker/streams/bus"

	"getarcane.dev/arcane/v3/internal/config"
	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/arcane/v3/internal/docker"
	"getarcane.dev/arcane/v3/internal/settings"
	"getarcane.dev/types/v3"
)

type (
	infisicalTestRuntime struct {
		resolution types.SecretResolution
		recorded   bool
	}
)

func (r *infisicalTestRuntime) ResolveSecrets(context.Context, types.SecretOwner) (types.SecretResolution, error) {
	return r.resolution, nil
}

func (r *infisicalTestRuntime) RecordSecretDeployment(context.Context, types.SecretOwner, types.SecretResolution) error {
	r.recorded = true
	return nil
}

func (*infisicalTestRuntime) SecretVariableNames(context.Context, types.SecretOwner) ([]string, error) {
	return nil, nil
}

func TestInfisicalEnvironmentPreservesUnrelatedServiceConfiguration(t *testing.T) {
	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{Name: "example_api", Labels: map[string]string{types.SwarmStackNamespaceLabel: "example"}},
		TaskTemplate: swarm.TaskSpec{ContainerSpec: &swarm.ContainerSpec{
			Image: "example:latest", Env: []string{"KEEP=unchanged", "PASSWORD=old", "REMOVED=old"}, Labels: map[string]string{"getarcane.app/secret-keys": `["PASSWORD","REMOVED"]`},
		}},
	}
	var updated swarm.ServiceSpec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			assert.NoError(
				t,
				json.MarshalWrite(w, map[string]any{"Swarm": map[string]any{"LocalNodeState": "active", "ControlAvailable": true, "Cluster": map[string]string{"ID": "cluster"}}}),
			)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services"):
			assert.NoError(t, json.MarshalWrite(w, []swarm.Service{{ID: "service", Spec: spec}}))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/services/service/update"):
			if r.URL.Query().Get("registryAuthFrom") != "spec" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"service does not have a previous spec"}`))
				return
			}
			assert.NoError(t, json.UnmarshalRead(r.Body, &updated))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	dockerClient, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, dockerClient.Close()) })
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&settings.SettingVariable{}))
	db := &database.DB{DB: gormDB}
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, err := settings.NewSettingsService(settings.SettingsServiceParams{Ctx: t.Context(), DB: db, Config: &config.Config{}, Env: settings.Env{}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(t.Context()))) })
	runtime := &infisicalTestRuntime{resolution: types.SecretResolution{
		Environment: map[string]string{"PASSWORD": "new"}, ServiceEnvironment: map[string]map[string]string{"api": {"TOKEN": "scoped"}}, Hashes: map[string]string{"binding": "version"},
	}}
	service := NewSwarmService(SwarmServiceParams{
		Secrets:         runtime,
		DockerService:   docker.NewDockerClientService(t.Context(), db, settingsService, bus.Config{}).WithClient(dockerClient),
		SettingsService: settingsService,
	})
	require.NoError(t, service.ApplySecretBindings(t.Context(), types.SecretOwner{Kind: "stack", ID: "example"}))
	require.ElementsMatch(t, []string{"KEEP=unchanged", "PASSWORD=new", "TOKEN=scoped"}, updated.TaskTemplate.ContainerSpec.Env)
	require.Equal(t, "example:latest", updated.TaskTemplate.ContainerSpec.Image)
	require.True(t, runtime.recorded)
}
