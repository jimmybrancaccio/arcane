package container

import (
	"context"

	"getarcane.dev/types/v3"
)

type (
	testSecretRuntime struct{}
)

func (testSecretRuntime) ResolveSecrets(context.Context, types.SecretOwner) (types.SecretResolution, error) {
	return types.SecretResolution{}, nil
}

func (testSecretRuntime) RecordSecretDeployment(context.Context, types.SecretOwner, types.SecretResolution) error {
	return nil
}

func (testSecretRuntime) SecretVariableNames(context.Context, types.SecretOwner) ([]string, error) {
	return nil, nil
}
