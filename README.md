> [!NOTE]
> The `main` branch is the v3 version of Arcane See the [`release/2.x.x`](https://github.com/getarcaneapp/arcane/tree/release/2.x.x) branch for the last 2.x release.

> [!IMPORTANT]
> Feature requests have moved to [Discussions](https://github.com/getarcaneapp/arcane/discussions/categories/feature-requests). Open new requests and upvote existing ones there; issues are only used for Bug reports now to help maintain priority on those issues.

<div align="center">

  <img src=".github/assets/img/arcane-full-trace-fill.svg" alt="Arcane Logo" width="500" />
  <p>Modern Docker Management, Designed for Everyone.</p>

<a title="Crowdin" target="_blank" href="https://crowdin.com/project/arcane-docker-management"><img src="https://badges.crowdin.net/arcane-docker-management/localized.svg"></a>
<a href="https://pkg.go.dev/getarcane.dev/arcane/v3"><img src="https://pkg.go.dev/badge/getarcane.dev/arcane/v3.svg" alt="Go Reference"></a>
<a href="https://github.com/getarcaneapp/arcane/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-BSD--3--Clause-blue.svg" alt="License"></a>
<a href="https://snyk.io/test/github/getarcaneapp/arcane"><img src="https://snyk.io/test/github/getarcaneapp/arcane/badge.svg" alt="Known Vulnerabilities"></a>
[![Greptile: The War on Bugs](https://www.greptile.com/badge.svg)](https://www.greptile.com/?utm_source=oss_badge&utm_medium=readme&utm_campaign=greptile_for_open_source)

<br />

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/assets/arcane-dash-v3-dark.jpg" />
  <source media="(prefers-color-scheme: light)" srcset=".github/assets/arcane-dash-v3-light.jpg" />
  <img width="1685" alt="Arcane v3 dashboard" src=".github/assets/arcane-dash-v3-dark.jpg" />
</picture>

## Documentation

For setup instructions, configuration details, and development guides, visit the **[official documentation site](https://getarcane.app)**.

## Infisical in this fork

Configure Universal Auth under **System → Secret Sources**. A source can supply default Infisical project, environment, and folder settings; each workload binding can select another target. Stored credentials remain hidden when editing, and connection tests verify target access. Optional guided setup uses a separate identity to move selected Compose project variables into Infisical and verify them using the deploy identity.

Use **Secret Sources** on a project, container overview, Swarm stack, or Swarm service to preview key names and configure delivery. New Compose projects also offer a source picker. Bindings are explicit: choosing a default source does not inject secrets into every workload. Required bindings block deployment when secrets cannot be fetched. Optional bindings display fetch failures and allow the operation to continue.

Swarm bindings support environment variables and native secret files under `/run/secrets`, including per-service mappings and file ownership/permissions. Native secrets use versioned names; previous versions are retained for Docker rollback. Applying changes updates service specifications and replaces tasks according to the service's update policy. Environment-variable values remain visible to users with Docker inspection access.

The manager holds Infisical credentials and resolves values for local Docker and connected agents. Upgrade agents to this fork before configuring remote bindings. Both the manager and its authenticated tunnel must remain available for remote secret resolution. Secret values are excluded from saved Compose sources, workflow journals, and browser previews; Docker retains the delivered environment or native secret. Opt-in local environment backups contain the original values and use mode `0600`.

Drift checks run every five minutes. Automatic rollout is disabled by default and skips stopped workloads; otherwise, use **Apply secret changes**. A failed automatic attempt requires manual retry or a subsequent secret change. Review service rollout status after applying changes.

This implementation adapts the Infisical work from [goodelyfe/arcane](https://github.com/goodelyfe/arcane/tree/30229db65f9a51d0f5675acb21e829154a73d6c3). Dockhand's workflow informed the interface behavior; its implementation was not copied.

## Sponsors

This project is supported by the following amazing people:

<p align="center">
  <a href="https://github.com/sponsors/kmendell">
    <img src='https://github.com/kmendell/static/blob/main/sponsors.svg?raw=true' alt="Logos" />
  </a>
</p>

## Security & Transparency

View the Software Bill of Materials (SBOM) for Arcane at **[getarcane.app/sbom](https://getarcane.app/sbom)**.

## Translating

Help translate Arcane on Crowdin: https://crowdin.com/project/arcane-docker-management

Thank you for checking out Arcane! Your feedback and contributions are always welcome.

</div>
