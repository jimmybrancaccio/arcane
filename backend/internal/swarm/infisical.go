package swarm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"

	"kit.getarcane.app"

	"getarcane.dev/types/v3"
)

func (s *SwarmService) resolveSecrets(ctx context.Context, owner types.SecretOwner) (types.SecretResolution, error) {
	info, err := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.SystemInfoResult, error) {
		return c.Info(ctx, client.InfoOptions{})
	})
	if err != nil {
		return types.SecretResolution{}, err
	}
	owner.ClusterID = kit.FromPtr(info.Info.Swarm.Cluster).ID
	return s.secretRuntime.ResolveSecrets(ctx, owner)
}

func (s *SwarmService) prepareSecretSpec(ctx context.Context, spec swarm.ServiceSpec, resolved types.SecretResolution) (swarm.ServiceSpec, error) {
	if len(resolved.Hashes) == 0 {
		return spec, nil
	}
	container := spec.TaskTemplate.ContainerSpec
	if container == nil {
		return spec, nil
	}
	values := make(map[string]string)
	maps.Copy(values, resolved.Environment)
	serviceName := strings.TrimPrefix(spec.Name, spec.Labels[types.SwarmStackNamespaceLabel]+"_")
	maps.Copy(values, resolved.ServiceEnvironment[serviceName])
	maps.Copy(values, resolved.ServiceEnvironment[spec.Name])
	var previousKeys []string
	_ = json.Unmarshal([]byte(container.Labels["getarcane.app/secret-keys"]), &previousKeys)
	container.Env = slices.DeleteFunc(container.Env, func(entry string) bool {
		key, _, _ := strings.Cut(entry, "=")
		_, found := values[key]
		return found || slices.Contains(previousKeys, key)
	})
	for _, key := range slices.Sorted(maps.Keys(values)) {
		container.Env = append(container.Env, key+"="+values[key])
	}
	labels := make(map[string]string)
	maps.Copy(labels, container.Labels)
	encodedKeys, _ := json.Marshal(slices.Sorted(maps.Keys(values)))
	labels["getarcane.app/secret-keys"] = string(encodedKeys)
	container.Labels = labels
	var previousFiles []string
	_ = json.Unmarshal([]byte(container.Labels["getarcane.app/secret-files"]), &previousFiles)
	container.Secrets = slices.DeleteFunc(container.Secrets, func(reference *swarm.SecretReference) bool {
		return reference.File != nil && slices.Contains(previousFiles, reference.File.Name)
	})
	providedTargets := make(map[string]bool)
	for _, file := range resolved.Files {
		if file.Service != "" && file.Service != spec.Name && file.Service != serviceName {
			continue
		}
		if file.Version == "" {
			return swarm.ServiceSpec{}, errors.New("secret file version is missing")
		}
		if providedTargets[file.Name] {
			continue
		}
		providedTargets[file.Name] = true
		digest := sha256.Sum256([]byte(file.Version + "/" + file.Key + "/" + file.Name))
		name := "arcane-infisical-" + hex.EncodeToString(digest[:16])
		secret, err := s.secrets.Materialize(
			ctx,
			types.SwarmSecret{Spec: swarm.SecretSpec{Annotations: swarm.Annotations{Name: name, Labels: map[string]string{"getarcane.app/infisical": "true"}}, Data: []byte(file.Value)}},
		)
		if err != nil {
			return swarm.ServiceSpec{}, err
		}
		container.Secrets = slices.DeleteFunc(container.Secrets, func(ref *swarm.SecretReference) bool { return ref.File != nil && ref.File.Name == file.Name })
		container.Secrets = append(
			container.Secrets,
			&swarm.SecretReference{
				SecretID:   secret.ID,
				SecretName: name,
				File:       &swarm.SecretReferenceFileTarget{Name: file.Name, UID: file.UID, GID: file.GID, Mode: fs.FileMode(file.Mode)},
			},
		)
	}
	encodedFiles, _ := json.Marshal(slices.Sorted(maps.Keys(providedTargets)))
	container.Labels["getarcane.app/secret-files"] = string(encodedFiles)
	return spec, nil
}

// ApplySecretBindings updates existing service specs, including stacks without saved source files.
func (s *SwarmService) ApplySecretBindings(ctx context.Context, owner types.SecretOwner) error {
	if err := s.ensureSwarmManager(ctx); err != nil {
		return err
	}
	resolved, err := s.resolveSecrets(ctx, owner)
	if err != nil {
		return err
	}
	var services []swarm.Service
	if owner.Kind == "stack" {
		listed, listErr := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.ServiceListResult, error) {
			return c.ServiceList(ctx, client.ServiceListOptions{Filters: make(client.Filters).Add("label", types.SwarmStackNamespaceLabel+"="+owner.ID)})
		})
		if listErr != nil {
			return listErr
		}
		services = listed.Items
	} else {
		inspected, inspectErr := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.ServiceInspectResult, error) {
			return c.ServiceInspect(ctx, owner.ID, client.ServiceInspectOptions{})
		})
		if inspectErr != nil {
			return inspectErr
		}
		services = append(services, inspected.Service)
	}
	for _, service := range services {
		spec, prepareErr := s.prepareSecretSpec(ctx, service.Spec, resolved)
		if prepareErr != nil {
			return prepareErr
		}
		if _, updateErr := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.ServiceUpdateResult, error) {
			return c.ServiceUpdate(ctx, service.ID, client.ServiceUpdateOptions{Spec: spec, Version: service.Version, RegistryAuthFrom: "previous-spec"})
		}); updateErr != nil {
			return updateErr
		}
	}
	return s.secretRuntime.RecordSecretDeployment(ctx, owner, resolved)
}

func (s *SwarmService) SecretWorkloadRunning(ctx context.Context, owner types.SecretOwner) (bool, error) {
	filter := make(client.Filters)
	if owner.Kind == "stack" {
		services, err := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.ServiceListResult, error) {
			return c.ServiceList(ctx, client.ServiceListOptions{Filters: make(client.Filters).Add("label", types.SwarmStackNamespaceLabel+"="+owner.ID)})
		})
		if err != nil {
			return false, err
		}
		if len(services.Items) == 0 {
			return false, nil
		}
		for _, service := range services.Items {
			filter = filter.Add("service", service.ID)
		}
	} else {
		filter = filter.Add("service", owner.ID)
	}
	tasks, err := s.dockerService.Call(ctx, func(ctx context.Context, c *client.Client) (client.TaskListResult, error) {
		return c.TaskList(ctx, client.TaskListOptions{Filters: filter})
	})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(tasks.Items, func(task swarm.Task) bool { return task.Status.State == swarm.TaskStateRunning }), nil
}
