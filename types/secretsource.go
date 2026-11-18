package types

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

type (
	SecretBrowseEnvironment struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	SecretBrowseProject struct {
		ID           string                    `json:"id"`
		Name         string                    `json:"name"`
		Environments []SecretBrowseEnvironment `json:"environments"`
	}
	SecretBrowse struct {
		Projects []SecretBrowseProject `json:"projects"`
		Folders  []string              `json:"folders"`
	}

	SecretTarget struct {
		ProjectID   string `json:"projectId,omitempty"`
		Environment string `json:"environment,omitempty"`
		Path        string `json:"path,omitempty"`
	}
	SecretSourceSettings struct {
		Host          string       `json:"host"`
		ClientID      string       `json:"clientId"`
		SetupClientID string       `json:"setupClientId,omitempty"`
		Defaults      SecretTarget `json:"defaults"`
	}
	SecretSource struct {
		LastTestedAt       *time.Time           `json:"lastTestedAt,omitempty"`
		Settings           SecretSourceSettings `json:"settings"`
		ID                 string               `json:"id"`
		Name               string               `json:"name"`
		LastError          string               `json:"lastError,omitempty"`
		Default            bool                 `json:"default"`
		HasCredential      bool                 `json:"hasCredential"`
		HasSetupCredential bool                 `json:"hasSetupCredential"`
	}
	SecretSourceWrite struct {
		Name            string               `json:"name" minLength:"1" maxLength:"128"`
		Settings        SecretSourceSettings `json:"settings"`
		Credential      string               `json:"credential,omitempty"`
		SetupCredential string               `json:"setupCredential,omitempty"`
		Default         bool                 `json:"default"`
	}
	SecretOwner struct {
		Kind      string `json:"kind" enum:"project,container,stack,service"`
		ID        string `json:"id" minLength:"1"`
		ClusterID string `json:"clusterId,omitempty"`
	}
	SecretMapping struct {
		Key      string `json:"key" minLength:"1"`
		Name     string `json:"name" minLength:"1"`
		Delivery string `json:"delivery" enum:"environment,file"`
		Service  string `json:"service,omitempty"`
		UID      string `json:"uid,omitempty"`
		GID      string `json:"gid,omitempty"`
		Mode     uint32 `json:"mode,omitempty" maximum:"511"`
	}
	SecretBindingWrite struct {
		Owner       SecretOwner     `json:"owner"`
		SourceID    string          `json:"sourceId" minLength:"1"`
		Target      SecretTarget    `json:"target"`
		Mappings    []SecretMapping `json:"mappings"`
		Position    int             `json:"position" minimum:"0"`
		Required    bool            `json:"required"`
		Enabled     bool            `json:"enabled"`
		AutoRollout bool            `json:"autoRollout"`
	}
	SecretBinding struct {
		DeployedAt    *time.Time `json:"deployedAt,omitempty"`
		CheckedAt     *time.Time `json:"checkedAt,omitempty"`
		ID            string     `json:"id"`
		EnvironmentID string     `json:"environmentId"`
		LastError     string     `json:"lastError,omitempty"`
		Keys          []string   `json:"keys"`
		SecretBindingWrite
		Changed bool `json:"changed"`
	}
	SecretCheck struct {
		Warning string   `json:"warning,omitempty"`
		Keys    []string `json:"keys"`
		Changed bool     `json:"changed"`
	}
	SecretFileValue struct {
		Version string `json:"version"`
		Key     string `json:"key"`
		Name    string `json:"name"`
		Service string `json:"service,omitempty"`
		UID     string `json:"uid"`
		GID     string `json:"gid"`
		Value   string `json:"value"`
		Mode    uint32 `json:"mode"`
	}
	// SecretResolution is private execution data. Never embed it in activity or public response types.
	SecretResolution struct {
		Environment        map[string]string            `json:"environment"`
		ServiceEnvironment map[string]map[string]string `json:"serviceEnvironment"`
		Hashes             map[string]string            `json:"hashes"`
		Owner              SecretOwner                  `json:"owner"`
		Files              []SecretFileValue            `json:"files"`
	}
	SwarmSecretHooks struct {
		Resolve func(context.Context, SecretOwner) (SecretResolution, error)
		Prepare func(context.Context, swarm.ServiceSpec, SecretResolution) (swarm.ServiceSpec, error)
		Record  func(context.Context, SecretOwner, SecretResolution) error
	}
	SecretRuntime interface {
		SecretVariableNames(ctx context.Context, owner SecretOwner) ([]string, error)
		ResolveSecrets(ctx context.Context, owner SecretOwner) (SecretResolution, error)
		RecordSecretDeployment(ctx context.Context, owner SecretOwner, result SecretResolution) error
	}
	SecretTunnelRequest struct {
		Hashes      map[string]string `json:"hashes,omitempty"`
		Owner       SecretOwner       `json:"owner"`
		Acknowledge bool              `json:"acknowledge"`
	}
)
