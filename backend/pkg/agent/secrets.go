package agent

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"getarcane.dev/types/v3"
)

type (
	SecretCallback func(context.Context, string, types.SecretTunnelRequest) (types.SecretResolution, error)
)

const (
	TunnelCapabilitySecrets = "secret-resolution-v1"
	secretResolutionPath    = "/api/tunnel/secrets"
)

func RequestSecrets(ctx context.Context, req types.SecretTunnelRequest) (types.SecretResolution, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := activeAgentEvents.Load()
	if client == nil {
		return types.SecretResolution{}, ErrNoActiveAgentTunnel
	}
	body, err := json.Marshal(req)
	if err != nil {
		return types.SecretResolution{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, muxManagerBaseURL+secretResolutionPath, bytes.NewReader(body))
	if err != nil {
		return types.SecretResolution{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return types.SecretResolution{}, errors.New("secret resolution manager is unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return types.SecretResolution{}, fmt.Errorf("manager refused secret resolution (HTTP %d)", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (10<<20)+1))
	if err != nil {
		return types.SecretResolution{}, errors.New("unable to read secret resolution")
	}
	if len(raw) > 10<<20 {
		return types.SecretResolution{}, errors.New("secret resolution exceeds the size limit")
	}
	var result types.SecretResolution
	if err = json.Unmarshal(raw, &result); err != nil {
		return types.SecretResolution{}, errors.New("invalid secret resolution response")
	}
	return result, nil
}

func registerSecretCallback(mux *http.ServeMux, callback SecretCallback, environmentID string) {
	if callback == nil {
		return
	}
	cfg := huma.DefaultConfig("Agent secret resolution", "1")
	cfg.OpenAPIPath = ""
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	api := humago.New(mux, cfg)
	huma.Register(api, huma.Operation{OperationID: "agentSecretResolution", Method: http.MethodPost, Path: secretResolutionPath, MaxBodyBytes: 1 << 20},
		func(ctx context.Context, input *struct{ Body types.SecretTunnelRequest }) (*struct{ Body types.SecretResolution }, error) {
			result, err := callback(ctx, environmentID, input.Body)
			if err != nil {
				return nil, huma.Error503ServiceUnavailable("required secret resolution failed")
			}
			return &struct{ Body types.SecretResolution }{Body: result}, nil
		})
}
