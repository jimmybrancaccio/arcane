package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dockertypes "kit.getarcane.app/docker/types"

	"getarcane.dev/types/v3"
)

type (
	testSecretRuntime struct{ result types.SecretResolution }
)

func (fake testSecretRuntime) ResolveSecrets(context.Context, types.SecretOwner) (types.SecretResolution, error) {
	return fake.result, nil
}

func (testSecretRuntime) RecordSecretDeployment(context.Context, types.SecretOwner, types.SecretResolution) error {
	return nil
}

func TestSecretDeploymentMasksProgressAndErrors(t *testing.T) {
	const secret = "secret-value-sentinel"
	var output bytes.Buffer
	service := &ProjectService{secrets: testSecretRuntime{result: types.SecretResolution{Environment: map[string]string{"PASSWORD": secret}}}}
	ctx := context.WithValue(t.Context(), dockertypes.ProgressWriterKey{}, io.Writer(&output))
	ctx, finish, err := service.beginSecretDeployment(ctx, "app")
	require.NoError(t, err)
	writer := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
	_, err = io.WriteString(writer, "PASSWORD="+secret[:8])
	require.NoError(t, err)
	_, err = io.WriteString(writer, secret[8:]+"\n"+strings.Repeat("x", 70*1024)+secret)
	require.NoError(t, err)
	deployErr := finish(errors.New("failed with " + secret))
	require.Error(t, deployErr)
	require.NotContains(t, deployErr.Error(), secret)
	require.NotContains(t, output.String(), secret)
	require.Contains(t, output.String(), "[REDACTED]")
}

func (testSecretRuntime) SecretVariableNames(context.Context, types.SecretOwner) ([]string, error) {
	return nil, nil
}
