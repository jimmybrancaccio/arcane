package services

import (
	"context"
	"encoding/json/v2"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

func (s *Service) restoreManagedEnvironment(ctx context.Context, serviceID string, spec *swarm.ServiceSpec) error {
	config := spec.TaskTemplate.ContainerSpec
	if config == nil || !slices.ContainsFunc(config.Env, func(entry string) bool { _, value, _ := strings.Cut(entry, "="); return value == "[REDACTED]" }) {
		return nil
	}
	current, err := s.docker.Call(
		ctx,
		func(ctx context.Context, c *client.Client) (client.ServiceInspectResult, error) {
			return c.ServiceInspect(ctx, serviceID, client.ServiceInspectOptions{})
		},
	)
	if err != nil {
		return err
	}
	prior := current.Service.Spec.TaskTemplate.ContainerSpec
	if prior == nil {
		return nil
	}
	var keys []string
	if decodeErr := json.Unmarshal([]byte(prior.Labels["getarcane.app/secret-keys"]), &keys); decodeErr != nil {
		return nil
	}
	values := make(map[string]string)
	for _, entry := range prior.Env {
		key, value, _ := strings.Cut(entry, "=")
		if slices.Contains(keys, key) {
			values[key] = value
		}
	}
	for index, entry := range config.Env {
		key, value, _ := strings.Cut(entry, "=")
		if actual, ok := values[key]; ok && value == "[REDACTED]" {
			config.Env[index] = key + "=" + actual
		}
	}
	return nil
}
