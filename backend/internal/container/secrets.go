package container

import (
	"context"
	"encoding/json/v2"
	"maps"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"kit.getarcane.app"

	"getarcane.dev/arcane/v3/internal/event"
	"getarcane.dev/types/v3"
)

const (
	secretKeysLabel = "getarcane.app/secret-keys"
)

func applySecretEnvironment(env []string, values map[string]string) []string {
	result := slices.DeleteFunc(slices.Clone(env), func(entry string) bool { key, _, _ := strings.Cut(entry, "="); _, exists := values[key]; return exists })
	for _, key := range slices.Sorted(maps.Keys(values)) {
		result = append(result, key+"="+values[key])
	}
	return result
}

func secretEnvironmentLabels(labels, values map[string]string) map[string]string {
	result := make(map[string]string)
	maps.Copy(result, labels)
	if len(values) > 0 {
		encoded, _ := json.Marshal(slices.Sorted(maps.Keys(values)))
		result[secretKeysLabel] = string(encoded)
	}
	return result
}

func maskSecretEnvironment(env []string, labels map[string]string) []string {
	var keys []string
	if err := json.Unmarshal([]byte(labels[secretKeysLabel]), &keys); err != nil {
		return env
	}
	masked := slices.Clone(env)
	for i, entry := range masked {
		key, _, _ := strings.Cut(entry, "=")
		if slices.Contains(keys, key) {
			masked[i] = key + "=[REDACTED]"
		}
	}
	return masked
}

func (s *ContainerService) recreateContainer(
	ctx context.Context,
	containerInfo container.InspectResponse,
	action, newName string,
	newConfig *container.Config,
	newHostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	apiVersion string,
	eventType event.EventType,
	user types.Actor,
) (string, error) {
	containerName := strings.TrimPrefix(containerInfo.Name, "/")
	owner := types.SecretOwner{Kind: "container", ID: containerName}
	resolved, secretErr := s.secrets.ResolveSecrets(ctx, owner)
	if secretErr != nil {
		return "", secretErr
	}
	// Resolve before stopping the original; secret values never enter the recovery journal.
	newConfig.Env = restoreSecretEnvironment(newConfig.Env, kit.FromPtr(containerInfo.Config).Env, kit.FromPtr(containerInfo.Config).Labels)
	newConfig.Env = applySecretEnvironment(newConfig.Env, resolved.Environment)
	newConfig.Labels = secretEnvironmentLabels(newConfig.Labels, resolved.Environment)
	id, err := s.recreateContainerRuntime(ctx, containerInfo, action, newName, newConfig, newHostConfig, networkingConfig, apiVersion, eventType, user)
	if err != nil {
		return "", err
	}
	if recordErr := s.secrets.RecordSecretDeployment(ctx, owner, resolved); recordErr != nil {
		return "", recordErr
	}
	return id, nil
}

func restoreSecretEnvironment(submitted, existing []string, labels map[string]string) []string {
	var keys []string
	if err := json.Unmarshal([]byte(labels[secretKeysLabel]), &keys); err != nil {
		return submitted
	}
	values := make(map[string]string)
	for _, entry := range existing {
		key, value, _ := strings.Cut(entry, "=")
		if slices.Contains(keys, key) {
			values[key] = value
		}
	}
	restored := slices.Clone(submitted)
	for index, entry := range restored {
		key, value, _ := strings.Cut(entry, "=")
		if prior, ok := values[key]; ok && value == "[REDACTED]" {
			restored[index] = key + "=" + prior
		}
	}
	return restored
}
