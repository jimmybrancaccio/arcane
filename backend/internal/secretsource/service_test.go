package secretsource

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/libtnb/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"kit.getarcane.app/sys/crypto"

	"getarcane.dev/arcane/v3/internal/config"
	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/arcane/v3/internal/event"
	"getarcane.dev/types/v3"
)

type (
	secretFixture struct {
		values map[string]string
		mu     sync.Mutex
		fail   bool
	}
)

var _ types.SecretRuntime = (*Service)(nil)

func newSecretFixture(t *testing.T) (*Service, *secretFixture, types.SecretSource) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	raw, err := db.DB()
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	require.NoError(t, db.AutoMigrate(&Source{}, &Binding{}, &event.Event{}))
	crypto.InitEncryption(crypto.Config{EncryptionKey: "arcane-infisical-testing-key-32-bytes", Environment: "test"})
	fixture := &secretFixture{values: map[string]string{"PASSWORD": "test-sensitive-value"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/login") {
			_, _ = fmt.Fprint(w, `{"accessToken":"test-token","expiresIn":3600}`)
			return
		}
		if fixture.fail {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message":"must-not-leak-sensitive-value"}`)
			return
		}
		if r.URL.Path == "/api/v1/projects" {
			_, _ = fmt.Fprint(w, `{"projects":[{"id":"project","name":"Production","environments":[{"name":"Production","slug":"prod"}]}]}`)
			return
		}
		if r.URL.Path == "/api/v2/folders" {
			_, _ = fmt.Fprint(w, `{"folders":[{"name":"waco"}]}`)
			return
		}
		items := make([]map[string]string, 0, len(fixture.values))
		for key, value := range fixture.values {
			items = append(items, map[string]string{"secretKey": key, "secretValue": value})
		}
		_ = json.MarshalWrite(w, map[string]any{"secrets": items})
	}))
	t.Cleanup(server.Close)
	service := NewService(&database.DB{DB: db}, &config.Config{}, event.NewEventService(&database.DB{DB: db}, &config.Config{}, server.Client()))
	source, err := service.SaveSource(
		t.Context(),
		"",
		types.SecretSourceWrite{
			Name:       "Infisical",
			Credential: "identity-secret-sentinel",
			Settings:   types.SecretSourceSettings{Host: server.URL, ClientID: "deploy-identity", Defaults: types.SecretTarget{ProjectID: "project", Environment: "prod", Path: "/waco"}},
			Default:    true,
		},
	)
	require.NoError(t, err)
	return service, fixture, source
}

func fixtureBinding(source types.SecretSource, owner types.SecretOwner) types.SecretBindingWrite {
	return types.SecretBindingWrite{Owner: owner, SourceID: source.ID, Required: true, Enabled: true}
}

func TestSourceCredentialsAndDefaults(t *testing.T) {
	s, _, source := newSecretFixture(t)
	stored, err := s.loadSource(t.Context(), source.ID)
	require.NoError(t, err)
	require.NotEqual(t, "identity-secret-sentinel", stored.Credential)
	encoded, err := json.Marshal(source)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "identity-secret-sentinel")
	updated, err := s.SaveSource(t.Context(), source.ID, types.SecretSourceWrite{Name: "Renamed", Settings: source.Settings, Default: true})
	require.NoError(t, err)
	require.True(t, updated.HasCredential)
	settings := source.Settings
	settings.Host = "https://different.example"
	_, err = s.SaveSource(t.Context(), source.ID, types.SecretSourceWrite{Name: "Renamed", Settings: settings})
	require.ErrorContains(t, err, "enter the deploy credential again")
	check, err := s.Test(t.Context(), source.ID, types.SecretTarget{})
	require.NoError(t, err)
	require.Equal(t, []string{"PASSWORD"}, check.Keys)
	_, err = s.SaveBinding(t.Context(), "0", "", fixtureBinding(source, types.SecretOwner{Kind: "project", ID: "app"}))
	require.NoError(t, err)
	require.ErrorContains(t, s.DeleteSource(t.Context(), source.ID), "bindings")
}

func TestSecretResolutionAndDeploymentState(t *testing.T) {
	s, fixture, source := newSecretFixture(t)
	owner := types.SecretOwner{Kind: "project", ID: "app"}
	binding, err := s.SaveBinding(t.Context(), "0", "", fixtureBinding(source, owner))
	require.NoError(t, err)
	result, err := s.ResolveSecrets(t.Context(), owner)
	require.NoError(t, err)
	require.Equal(t, "test-sensitive-value", result.Environment["PASSWORD"])
	require.NoError(t, s.RecordSecretDeployment(t.Context(), owner, result))
	fixture.mu.Lock()
	fixture.values["PASSWORD"] = "rotated-sensitive-value"
	fixture.mu.Unlock()
	targets, err := s.DriftTargets(t.Context())
	require.NoError(t, err)
	require.Empty(t, targets, "automatic rollout is opt-in")
	bindings, err := s.ListBindings(t.Context(), "0", owner)
	require.NoError(t, err)
	require.True(t, bindings[0].Changed)
	encoded, err := json.Marshal(bindings)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "rotated-sensitive-value")
	require.NotContains(t, string(encoded), result.Hashes[binding.ID])
	require.NoError(t, s.db.Model(&Binding{}).Where("id = ?", binding.ID).Update("auto_rollout", true).Error)
	targets, err = s.DriftTargets(t.Context())
	require.NoError(t, err)
	require.Len(t, targets, 1)
}

func TestRequiredAndOptionalSecretFailures(t *testing.T) {
	s, fixture, source := newSecretFixture(t)
	owner := types.SecretOwner{Kind: "project", ID: "app"}
	binding, err := s.SaveBinding(t.Context(), "0", "", fixtureBinding(source, owner))
	require.NoError(t, err)
	fixture.mu.Lock()
	fixture.fail = true
	fixture.mu.Unlock()
	_, err = s.ResolveSecrets(t.Context(), owner)
	require.ErrorContains(t, err, "required Infisical binding is unavailable")
	require.NotContains(t, err.Error(), "must-not-leak")
	require.NoError(t, s.db.Model(&Binding{}).Where("id = ?", binding.ID).Update("required", false).Error)
	result, err := s.ResolveSecrets(t.Context(), owner)
	require.NoError(t, err)
	require.Empty(t, result.Environment)
	fixture.mu.Lock()
	fixture.fail = false
	fixture.values = map[string]string{}
	fixture.mu.Unlock()
	require.NoError(t, s.db.Model(&Binding{}).Where("id = ?", binding.ID).Update("required", true).Error)
	_, err = s.ResolveSecrets(t.Context(), owner)
	require.Error(t, err)
}

func TestAgentSecretResolutionIsEnvironmentScoped(t *testing.T) {
	s, _, source := newSecretFixture(t)
	owner := types.SecretOwner{Kind: "project", ID: "shared-id"}
	_, err := s.SaveBinding(t.Context(), "agent-one", "", fixtureBinding(source, owner))
	require.NoError(t, err)
	result, err := s.AgentRequest(t.Context(), "agent-two", types.SecretTunnelRequest{Owner: owner})
	require.NoError(t, err)
	require.Empty(t, result.Environment)
	result, err = s.AgentRequest(t.Context(), "agent-one", types.SecretTunnelRequest{Owner: owner})
	require.NoError(t, err)
	require.Equal(t, "test-sensitive-value", result.Environment["PASSWORD"])
}

func TestSourceBrowsingReturnsTargetMetadata(t *testing.T) {
	s, _, source := newSecretFixture(t)
	projects, err := s.Browse(t.Context(), source.ID, types.SecretTarget{})
	require.NoError(t, err)
	require.Len(t, projects.Projects, 1)
	require.Equal(t, "prod", projects.Projects[0].Environments[0].Slug)
	require.Empty(t, projects.Folders)
	folders, err := s.Browse(t.Context(), source.ID, types.SecretTarget{ProjectID: "project", Environment: "prod", Path: "/"})
	require.NoError(t, err)
	require.Equal(t, []string{"waco"}, folders.Folders)
	require.Empty(t, folders.Projects)
}

func TestSwarmDeploymentRecordsTheResolvedClusterScope(t *testing.T) {
	s, _, source := newSecretFixture(t)
	owner := types.SecretOwner{Kind: "service", ID: "service", ClusterID: "cluster"}
	_, err := s.SaveBinding(t.Context(), "remote-manager", "", fixtureBinding(source, owner))
	require.NoError(t, err)
	resolved, err := s.ResolveSecrets(t.Context(), owner)
	require.NoError(t, err)
	require.NoError(t, s.RecordSecretDeployment(t.Context(), types.SecretOwner{Kind: "service", ID: "service"}, resolved))
	bindings, err := s.ListBindings(t.Context(), "0", owner)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.NotNil(t, bindings[0].DeployedAt)
}
