package secretsource

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"slices"

	"gorm.io/gorm"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/container"
	"getarcane.dev/arcane/v3/internal/environment"
	"getarcane.dev/arcane/v3/internal/project"
	"getarcane.dev/arcane/v3/internal/swarm"
	"getarcane.dev/arcane/v3/pkg/agent"
	"getarcane.dev/types/v3"
)

type (
	Workloads struct {
		service    *Service
		projects   *project.ProjectService
		containers *container.ContainerService
		swarm      *swarm.SwarmService
		transport  environment.RemoteTransport
		tunnels    *agent.TunnelRegistry
	}
)

func (w *Workloads) Running(ctx context.Context, target rotationTarget) (bool, error) {
	if target.Owner.Kind == "stack" || target.Owner.Kind == "service" {
		if target.EnvironmentID == "0" {
			return w.swarm.SecretWorkloadRunning(ctx, target.Owner)
		}
		var result types.ApiResponse[bool]
		body, err := json.Marshal(target.Owner)
		if err != nil {
			return false, err
		}
		err = w.transport.ProxyJSONRequest(ctx, target.EnvironmentID, http.MethodPost, "/api/agents/0/secret-running", body, &result)
		return result.Data, err
	}
	if target.Owner.Kind == "project" {
		if target.EnvironmentID == "0" {
			p, err := w.projects.GetProjectFromDatabaseByID(ctx, target.Owner.ID)
			if err != nil {
				return false, err
			}
			return p.RunningCount > 0 || p.Status == project.ProjectStatusRunning, nil
		}
		var result types.ApiResponse[types.Project]
		err := w.transport.ProxyJSONRequest(ctx, target.EnvironmentID, http.MethodGet, "/api/agents/0/projects/"+url.PathEscape(target.Owner.ID), nil, &result)
		return result.Data.RunningCount > 0 || result.Data.Status == string(project.ProjectStatusRunning), err
	}
	if target.EnvironmentID == "0" {
		c, err := w.containers.GetContainerByReference(ctx, target.Owner.ID)
		if err != nil {
			return false, err
		}
		return c.State != nil && c.State.Running, nil
	}
	var result types.ApiResponse[types.ContainerDetails]
	err := w.transport.ProxyJSONRequest(ctx, target.EnvironmentID, http.MethodGet, "/api/agents/0/containers/"+url.PathEscape(target.Owner.ID), nil, &result)
	return result.Data.State.Running, err
}

func (w *Workloads) CanonicalOwner(ctx context.Context, environmentID string, owner types.SecretOwner) (types.SecretOwner, error) {
	owner.ClusterID = ""
	if owner.Kind != "stack" && owner.Kind != "service" {
		return owner, nil
	}
	var status types.SwarmRuntimeStatus
	if environmentID == "0" {
		result, err := w.swarm.GetSwarmInfo(ctx)
		if err != nil {
			return types.SecretOwner{}, err
		}
		owner.ClusterID = result.ID
		return owner, nil
	}
	var result types.ApiResponse[types.SwarmRuntimeStatus]
	if err := w.transport.ProxyJSONRequest(ctx, environmentID, http.MethodGet, "/api/agents/0/swarm/status", nil, &result); err != nil {
		return types.SecretOwner{}, err
	}
	status = result.Data
	if status.ClusterID == "" {
		return types.SecretOwner{}, common.Classify(common.ErrUnavailable, errors.New("a connected Swarm manager is required"))
	}
	owner.ClusterID = status.ClusterID
	return owner, nil
}

func (w *Workloads) SyncManifest(ctx context.Context, environmentID string) error {
	if environmentID == "0" {
		return nil
	}
	tunnel, ok := w.tunnels.Active(environmentID).Get()
	if !ok || !slices.Contains(tunnel.Capabilities, agent.TunnelCapabilitySecrets) {
		return common.Classify(common.ErrUnavailable, errors.New("connect an agent with Infisical support before changing bindings"))
	}
	bindings, err := w.service.ListBindings(ctx, environmentID, types.SecretOwner{})
	if err != nil {
		return err
	}
	body, err := json.Marshal(bindings)
	if err != nil {
		return err
	}
	return w.transport.ProxyJSONRequest(ctx, environmentID, http.MethodPut, "/api/agents/0/secret-manifest", body, nil)
}

func (w *Workloads) Apply(ctx context.Context, environmentID string, owner types.SecretOwner, actor types.Actor) error {
	if environmentID != "0" {
		if err := w.SyncManifest(ctx, environmentID); err != nil {
			return err
		}
		body, err := json.Marshal(owner)
		if err != nil {
			return err
		}
		return w.transport.ProxyJSONRequest(ctx, environmentID, http.MethodPost, "/api/agents/0/secret-apply", body, nil)
	}
	switch owner.Kind {
	case "project":
		return w.projects.DeployProject(ctx, owner.ID, actor, nil)
	case "container":
		_, err := w.containers.RedeployContainer(ctx, owner.ID, actor)
		return err
	case "stack", "service":
		return w.swarm.ApplySecretBindings(ctx, owner)
	default:
		return common.Classify(common.ErrValidation, errors.New("unsupported secret owner"))
	}
}

func (w *Workloads) ProjectSource(ctx context.Context, environmentID string, owner types.SecretOwner) (types.Project, error) {
	if owner.Kind != "project" {
		return types.Project{}, common.Classify(common.ErrValidation, errors.New("guided setup requires a Compose project"))
	}
	if environmentID == "0" {
		return w.projects.GetProjectDetails(ctx, owner.ID, types.ProjectDetailsOptions{IncludeComposeContent: true, IncludeEnvState: true})
	}
	var result types.ApiResponse[types.Project]
	err := w.transport.ProxyJSONRequest(ctx, environmentID, http.MethodGet, "/api/agents/0/projects/"+url.PathEscape(owner.ID)+"/compose", nil, &result)
	return result.Data, err
}

func (w *Workloads) SaveBinding(ctx context.Context, environmentID, id string, req types.SecretBindingWrite) (types.SecretBinding, error) {
	if environmentID != "0" {
		tunnel, ok := w.tunnels.Active(environmentID).Get()
		if !ok || !slices.Contains(tunnel.Capabilities, agent.TunnelCapabilitySecrets) {
			return types.SecretBinding{}, common.Classify(common.ErrUnavailable, errors.New("connect an agent with Infisical support before saving bindings"))
		}
	}
	var previous Binding
	if id != "" {
		if err := w.service.db.WithContext(ctx).First(&previous, "id = ? AND environment_id = ?", id, environmentID).Error; err != nil {
			return types.SecretBinding{}, err
		}
	}
	result, err := w.service.SaveBinding(ctx, environmentID, id, req)
	if err != nil {
		return types.SecretBinding{}, err
	}
	if err = w.SyncManifest(ctx, environmentID); err != nil {
		rollbackErr := w.service.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
			if id == "" {
				return tx.Delete(&Binding{}, "id = ?", result.ID).Error
			}
			return tx.Save(&previous).Error
		})
		return types.SecretBinding{}, errors.Join(err, rollbackErr)
	}
	return result, nil
}
