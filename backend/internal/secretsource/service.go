package secretsource

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"kit.getarcane.app/sys/crypto"

	"getarcane.dev/arcane/v3/internal/common"
	"getarcane.dev/arcane/v3/internal/config"
	"getarcane.dev/arcane/v3/internal/database"
	"getarcane.dev/arcane/v3/internal/event"
	"getarcane.dev/arcane/v3/internal/secretsource/children/infisical"
	"getarcane.dev/types/v3"
)

type (
	Service struct {
		events  *event.EventService
		db      *database.DB
		config  *config.Config
		clients map[string]*infisical.Client
		mu      sync.Mutex
	}
)

func NewService(db *database.DB, cfg *config.Config, events *event.EventService) *Service {
	return &Service{events: events, db: db, config: cfg, clients: make(map[string]*infisical.Client)}
}

func sourceDTO(source Source) types.SecretSource {
	return types.SecretSource{
		ID: source.ID, Name: source.Name, Settings: source.Settings, Default: source.IsDefault,
		HasCredential: source.Credential != "", HasSetupCredential: source.SetupCredential != "",
		LastTestedAt: source.LastTestedAt, LastError: source.LastError,
	}
}

func (s *Service) ListSources(ctx context.Context) ([]types.SecretSource, error) {
	var sources []Source
	if err := s.db.WithContext(ctx).Order("name").Find(&sources).Error; err != nil {
		return nil, err
	}
	result := make([]types.SecretSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, sourceDTO(source))
	}
	return result, nil
}

func (s *Service) loadSource(ctx context.Context, id string) (Source, error) {
	var source Source
	if err := s.db.WithContext(ctx).First(&source, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Source{}, common.Classify(common.ErrNotFound, errors.New("secret source not found"))
		}
		return Source{}, err
	}
	return source, nil
}

func (s *Service) SaveSource(ctx context.Context, id string, req types.SecretSourceWrite) (types.SecretSource, error) {
	settings := req.Settings
	host, err := infisical.ParseSiteURL(settings.Host)
	if err != nil {
		return types.SecretSource{}, common.Classify(common.ErrValidation, err)
	}
	if hostErr := checkSecretHostInternal(host.Hostname()); hostErr != nil {
		return types.SecretSource{}, common.Classify(common.ErrValidation, hostErr)
	}
	settings.Host = host.String()
	settings.ClientID = strings.TrimSpace(settings.ClientID)
	settings.SetupClientID = strings.TrimSpace(settings.SetupClientID)
	if strings.TrimSpace(req.Name) == "" || settings.ClientID == "" {
		return types.SecretSource{}, common.Classify(common.ErrValidation, errors.New("name and Universal Auth client ID are required"))
	}
	if settings.SetupClientID != "" && settings.SetupClientID == settings.ClientID {
		return types.SecretSource{}, common.Classify(common.ErrValidation, errors.New("setup and deploy identities must differ"))
	}
	var previous Source
	if id != "" {
		previous, err = s.loadSource(ctx, id)
		if err != nil {
			return types.SecretSource{}, err
		}
	}
	credential := previous.Credential
	setupCredential := previous.SetupCredential
	if id != "" && (previous.Settings.Host != settings.Host || previous.Settings.ClientID != settings.ClientID) && req.Credential == "" {
		return types.SecretSource{}, common.Classify(common.ErrValidation, errors.New("enter the deploy credential again after changing the host or identity"))
	}
	if previous.Settings.Host != settings.Host || previous.Settings.SetupClientID != settings.SetupClientID {
		setupCredential = ""
	}
	if req.Credential != "" {
		credential, err = crypto.Encrypt(req.Credential)
		if err != nil {
			return types.SecretSource{}, err
		}
	}
	if credential == "" {
		return types.SecretSource{}, common.Classify(common.ErrValidation, infisical.ErrMissingCredentials)
	}
	if req.SetupCredential != "" {
		if settings.SetupClientID == "" {
			return types.SecretSource{}, common.Classify(common.ErrValidation, errors.New("setup client ID is required"))
		}
		setupCredential, err = crypto.Encrypt(req.SetupCredential)
		if err != nil {
			return types.SecretSource{}, err
		}
	}
	settings.Defaults.Path = infisical.NormalizeSecretPath(settings.Defaults.Path)
	source := Source{
		BaseModel: previous.BaseModel, Name: strings.TrimSpace(req.Name), Settings: settings,
		Credential: credential, SetupCredential: setupCredential, IsDefault: req.Default,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.Default {
			if clearErr := tx.Model(&Source{}).Where("is_default = ?", true).Update("is_default", false).Error; clearErr != nil {
				return clearErr
			}
		}
		return tx.Save(&source).Error
	})
	if err != nil {
		return types.SecretSource{}, err
	}
	s.mu.Lock()
	for key := range s.clients {
		if strings.HasPrefix(key, id+"/") {
			delete(s.clients, key)
		}
	}
	s.mu.Unlock()
	return sourceDTO(source), nil
}

func (s *Service) DeleteSource(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Binding{}).Where("source_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return common.Classify(common.ErrConflict, errors.New("remove this source's workload bindings first"))
		}
		if err := tx.Delete(&Source{}, "id = ?", id).Error; err != nil {
			return err
		}
		s.mu.Lock()
		for key := range s.clients {
			if strings.HasPrefix(key, id+"/") {
				delete(s.clients, key)
			}
		}
		s.mu.Unlock()
		return nil
	})
}

func (s *Service) client(ctx context.Context, sourceID string, setup bool) (*infisical.Client, Source, error) {
	source, err := s.loadSource(ctx, sourceID)
	if err != nil {
		return nil, Source{}, err
	}
	key := sourceID
	clientID := source.Settings.ClientID
	ciphertext := source.Credential
	if setup {
		key += "/setup"
		clientID = source.Settings.SetupClientID
		ciphertext = source.SetupCredential
	}
	fingerprint := sha256.Sum256([]byte(source.Settings.Host + "\x00" + clientID + "\x00" + ciphertext))
	key += "/" + hex.EncodeToString(fingerprint[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.clients[key]; ok {
		return cached, source, nil
	}
	credential, err := crypto.Decrypt(ciphertext)
	if err != nil {
		return nil, Source{}, errors.New("unable to decrypt secret source credential")
	}
	client, err := infisical.NewClient(newSecretHTTPClientInternal(30*time.Second), infisical.Config{
		SiteURL: source.Settings.Host, ClientID: clientID, ClientSecret: credential,
	})
	if err != nil {
		return nil, Source{}, err
	}
	s.clients[key] = client
	return client, source, nil
}

func resolvedTarget(target, defaults types.SecretTarget) types.SecretTarget {
	return types.SecretTarget{
		ProjectID:   cmp.Or(target.ProjectID, defaults.ProjectID),
		Environment: cmp.Or(target.Environment, defaults.Environment),
		Path:        infisical.NormalizeSecretPath(cmp.Or(target.Path, defaults.Path)),
	}
}

func (s *Service) Fetch(ctx context.Context, sourceID string, target types.SecretTarget) (map[string]string, error) {
	client, source, err := s.client(ctx, sourceID, false)
	if err != nil {
		return nil, err
	}
	target = resolvedTarget(target, source.Settings.Defaults)
	values, err := client.ListSecrets(
		ctx,
		infisical.SecretsQuery{ProjectID: target.ProjectID, Environment: target.Environment, SecretPath: target.Path, IncludeImports: true, ExpandSecretReferences: true},
	)
	if err != nil {
		return nil, fmt.Errorf("fetch Infisical secrets: %w", err)
	}
	return values, nil
}

func (s *Service) Test(ctx context.Context, id string, target types.SecretTarget) (types.SecretCheck, error) {
	values, err := s.Fetch(ctx, id, target)
	message := ""
	if err != nil {
		message = "Infisical target is unavailable; check the connection, target and identity permissions"
	}
	if saveErr := s.db.WithContext(ctx).Model(&Source{}).Where("id = ?", id).Updates(map[string]any{"last_tested_at": time.Now(), "last_error": message}).Error; saveErr != nil {
		return types.SecretCheck{}, saveErr
	}
	if err != nil {
		return types.SecretCheck{}, common.Classify(common.ErrUnavailable, errors.New(message))
	}
	return types.SecretCheck{Keys: slices.Sorted(maps.Keys(values))}, nil
}

func (s *Service) Browse(ctx context.Context, id string, target types.SecretTarget) (types.SecretBrowse, error) {
	result := types.SecretBrowse{}
	client, _, err := s.client(ctx, id, false)
	if err != nil {
		return result, err
	}
	if target.ProjectID != "" {
		result.Folders, err = client.ListFolders(ctx, target.ProjectID, target.Environment, target.Path)
		return result, err
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return result, err
	}
	for _, project := range projects {
		environments := make([]types.SecretBrowseEnvironment, 0, len(project.Environments))
		for _, environment := range project.Environments {
			environments = append(environments, types.SecretBrowseEnvironment{Name: environment.Name, Slug: environment.Slug})
		}
		result.Projects = append(result.Projects, types.SecretBrowseProject{ID: project.ID, Name: project.Name, Environments: environments})
	}
	return result, nil
}
