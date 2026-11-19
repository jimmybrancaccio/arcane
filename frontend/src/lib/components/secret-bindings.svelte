<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';

	import SelectWithLabel from '#lib/components/form/select-with-label.svelte';
	import SecretSetup from '#lib/components/secret-setup.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { m } from '#lib/paraglide/messages.js';
	import { queryKeys } from '#lib/query/query-keys.js';
	import { secretSourceService } from '#lib/services/secret-source-service.js';
	import type {
		SecretBrowseProject,
		SecretBinding,
		SecretBindingWrite,
		SecretMapping,
		SecretOwner,
		SecretTarget
	} from '#lib/types/secret-source.js';
	import { extractApiErrorMessage, handleApiResultWithCallbacks } from '#lib/utils/api.js';
	import { hasPermission } from '#lib/utils/auth.js';
	import { suggestSecretMappings } from '#lib/utils/secret-mappings.js';
	import { tryCatch } from '#lib/utils/try-catch.js';

	let {
		environmentId,
		owner,
		composeContents = [],
		composeIncomplete = false,
		onDraft
	}: {
		environmentId: string;
		owner: SecretOwner;
		composeContents?: string[];
		composeIncomplete?: boolean;
		onDraft?: (value: SecretBindingWrite) => void;
	} = $props();
	const canUse = $derived(hasPermission('secret-sources:use', environmentId));
	const sources = createQuery(() => ({
		queryKey: queryKeys.secretSources.all,
		queryFn: () => secretSourceService.list(),
		enabled: canUse
	}));
	const bindings = createQuery(() => ({
		queryKey: queryKeys.secretSources.bindings(environmentId, owner.kind, owner.id),
		queryFn: () => secretSourceService.bindings(environmentId, owner),
		enabled: canUse && Boolean(owner.id)
	}));
	let sourceId = $state('');
	let editingId = $state('');
	let target = $state<SecretTarget>({ projectId: '', environment: '', path: '/' });
	let mappings = $state<SecretMapping[]>([]);
	let keys = $state<string[]>([]);
	let required = $state(true);
	let autoRollout = $state(false);
	let busy = $state(false);
	let previewError = $state('');
	let browseProjects = $state<SecretBrowseProject[]>([]);
	let folders = $state<string[]>([]);
	let browseError = $state('');
	let manualKey = $state('');
	let manualService = $state('');
	let reviewedSuggestions = $state(false);
	let previewSelection = $state('');
	let savedDraft = $state('');
	let redeployPending = $state(false);
	let redeployError = $state('');
	const draft = $derived(JSON.stringify({ environmentId, owner, sourceId, target, mappings, required, autoRollout }));
	const unsavedMappings = $derived((Boolean(savedDraft) || mappings.length > 0) && draft !== savedDraft);
	const selection = $derived(JSON.stringify({ environmentId, sourceId, target, owner }));
	const stalePreview = $derived(Boolean(keys.length) && selection !== previewSelection);
	const canSubmit = $derived.by(() => {
		if (busy || stalePreview) return false;
		if (unsavedMappings && (!sourceId || mappings.length === 0 || previewError)) return false;
		if (onDraft) return unsavedMappings && mappings.length > 0;
		return unsavedMappings || Boolean(bindings.data?.length);
	});
	const composeWorkload = $derived(owner.kind === 'project' || owner.kind === 'stack');
	const suggestions = $derived(suggestSecretMappings(composeContents, keys));
	const unmatchedKeys = $derived(keys.filter((key) => !mappings.some((mapping) => mapping.key === key)));
	const nativeSupported = $derived(owner.kind === 'stack' || owner.kind === 'service');
	const prefix = $derived(`infisical-${owner.kind}-${owner.id}`);

	function selectSource(id: string) {
		sourceId = id;
		const source = sources.data?.find((entry) => entry.id === id);
		target = {
			projectId: source?.settings.defaults.projectId ?? '',
			environment: source?.settings.defaults.environment ?? '',
			path: source?.settings.defaults.path ?? '/'
		};
		keys = [];
		mappings = [];
		browseProjects = [];
		folders = [];
		browseError = '';
		previewError = '';
		reviewedSuggestions = false;
		savedDraft = '';
		redeployPending = false;
		redeployError = '';
		manualKey = '';
		manualService = '';
	}
	$effect(() => {
		if (!sourceId && sources.data?.length)
			selectSource(sources.data.find((source) => source.default)?.id ?? sources.data[0]?.id ?? '');
	});
	async function browse(kind: 'projects' | 'folders') {
		browseError = '';
		const browseTarget = kind === 'projects' ? { projectId: '', environment: '', path: '/' } : target;
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.browse(sourceId, browseTarget)),
			message: m.secret_operation_failed(),
			onSuccess: (result) => {
				if (kind === 'projects') browseProjects = result.projects;
				else folders = result.folders;
			},
			onError: (error) => {
				browseError = extractApiErrorMessage(error);
			}
		});
	}
	function reviewSuggestions() {
		mappings = suggestions.mappings.map((mapping) => ({ ...mapping }));
		reviewedSuggestions = true;
	}
	async function preview() {
		previewError = '';
		busy = true;
		const requestedSelection = selection;
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.keys(sourceId, target)),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
			onSuccess: (result) => {
				if (requestedSelection !== selection) return;
				previewSelection = requestedSelection;
				keys = result.keys;
				manualKey = keys[0] ?? '';
				if (!editingId) {
					if (composeWorkload) {
						mappings = suggestSecretMappings(composeContents, result.keys).mappings;
						reviewedSuggestions = true;
					} else mappings = keys.map((key) => ({ key, name: key, delivery: 'environment' }));
				}
			},
			onError: (error) => {
				if (requestedSelection !== selection) return;
				previewError = extractApiErrorMessage(error);
			}
		});
	}
	function edit(binding: SecretBinding) {
		reviewedSuggestions = false;
		redeployPending = false;
		redeployError = '';
		manualKey = binding.keys[0] ?? '';
		editingId = binding.id;
		sourceId = binding.sourceId;
		target = { ...binding.target };
		mappings = binding.mappings.map((mapping) => ({ ...mapping }));
		keys = binding.keys;
		required = binding.required;
		autoRollout = binding.autoRollout;
		previewSelection = JSON.stringify({ environmentId, sourceId: binding.sourceId, target: binding.target, owner });
		savedDraft = JSON.stringify({
			environmentId,
			owner,
			sourceId: binding.sourceId,
			target: binding.target,
			mappings: binding.mappings,
			required: binding.required,
			autoRollout: binding.autoRollout
		});
	}
	async function save() {
		if (!canSubmit) return;
		busy = true;
		redeployError = '';
		const savedSelection = draft;
		const requestedSelection = selection;
		const deploymentEnvironment = environmentId;
		const deploymentOwner = { ...owner };
		const value: SecretBindingWrite = {
			owner: deploymentOwner,
			sourceId,
			target: { ...target },
			mappings: mappings.map((mapping) => ({ ...mapping })),
			required,
			autoRollout,
			enabled: true,
			position: bindings.data?.find((entry) => entry.id === editingId)?.position ?? bindings.data?.length ?? 0
		};
		if (onDraft) {
			onDraft(value);
			busy = false;
			return;
		}
		await handleApiResultWithCallbacks({
			result: await tryCatch(
				secretSourceService.saveAndRedeploy(deploymentEnvironment, {
					owner: deploymentOwner,
					binding: unsavedMappings ? { id: editingId, value } : undefined,
					onSaved: (binding) => {
						if (requestedSelection !== selection) return;
						editingId = binding.id;
						savedDraft = savedSelection;
						redeployPending = true;
					}
				})
			),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
			onSuccess: async () => {
				if (requestedSelection !== selection) return;
				redeployPending = false;
				await bindings.refetch();
			},
			onError: async (error) => {
				if (requestedSelection !== selection) return;
				redeployError = extractApiErrorMessage(error);
				await bindings.refetch();
			}
		});
	}
	async function remove(id: string) {
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.deleteBinding(environmentId, id)),
			message: m.secret_operation_failed(),
			onSuccess: async () => {
				await bindings.refetch();
			}
		});
	}
	async function check() {
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.check(environmentId, owner)),
			message: m.secret_operation_failed(),
			onSuccess: async () => {
				await bindings.refetch();
			}
		});
	}
</script>

{#if canUse}
	<Card.Root>
		<Card.Header
			><Card.Title>{m.secret_sources()}</Card.Title><Card.Description>{m.no_secret_values_shown()}</Card.Description></Card.Header
		>
		<Card.Content
			><div class="flex flex-col gap-5">
				{#if sources.error || bindings.error}<p role="alert" class="text-destructive">
						{extractApiErrorMessage(sources.error ?? bindings.error)}
					</p>{/if}
				{#each bindings.data ?? [] as binding (binding.id)}
					<div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4">
						<div>
							<p class="font-medium">
								{sources.data?.find((source) => source.id === binding.sourceId)?.name ?? binding.sourceId} · {binding.target
									.environment} · {binding.target.path}
							</p>
							<p class="text-sm text-muted-foreground">
								{binding.mappings
									.map((mapping) => `${mapping.key} → ${mapping.name} (${mapping.service || m.all_services()})`)
									.join(', ')}
							</p>
							<p class="text-sm">
								{#if binding.lastError}{binding.lastError}{:else if !binding.deployedAt}{m.secrets_not_deployed()}{:else if binding.changed}{m.secrets_changed()}{:else}{m.secrets_current()}{/if}
							</p>
						</div>
						<div class="flex gap-2">
							<Button variant="outline" onclick={() => edit(binding)}>{m.common_edit()}</Button><Button
								variant="outline"
								onclick={() => remove(binding.id)}>{m.remove_binding()}</Button
							>
						</div>
					</div>
				{:else}<p class="text-muted-foreground">{m.no_secret_bindings()}</p>{/each}
				<div class="flex flex-wrap gap-3">
					<Button variant="outline" disabled={!sourceId || busy} onclick={() => browse('projects')}
						>{m.browse_infisical_projects()}</Button
					>
					<Button
						variant="outline"
						disabled={!sourceId || !target.projectId || !target.environment || busy}
						onclick={() => browse('folders')}>{m.browse_infisical_folders()}</Button
					>
				</div>
				{#if browseError}<p role="alert" class="text-destructive">{browseError}</p>{/if}
				{#if browseProjects.length}
					<SelectWithLabel
						id={`${prefix}-browse-project`}
						label={m.select_infisical_project()}
						value={target.projectId}
						options={browseProjects.map((project) => ({ value: project.id, label: project.name }))}
						onValueChange={(id) => {
							target = { ...target, projectId: id };
							folders = [];
						}}
					/>
				{/if}
				{#if browseProjects.find((project) => project.id === target.projectId)?.environments.length}
					<SelectWithLabel
						id={`${prefix}-browse-environment`}
						label={m.environment()}
						value={target.environment}
						options={(browseProjects.find((project) => project.id === target.projectId)?.environments ?? []).map(
							(environment) => ({ value: environment.slug, label: environment.name })
						)}
						onValueChange={(value) => {
							target = { ...target, environment: value };
							folders = [];
						}}
					/>
				{/if}
				{#if folders.length}
					<SelectWithLabel
						id={`${prefix}-browse-folder`}
						label={m.secret_path()}
						value=""
						options={folders.map((folder) => ({ value: folder, label: folder }))}
						onValueChange={(folder) => {
							target = { ...target, path: `${target.path.replace(/\/$/, '')}/${folder}` };
							folders = [];
						}}
					/>
				{/if}
				<div class="grid gap-4 sm:grid-cols-2">
					<div class="flex flex-col gap-2">
						<SelectWithLabel
							id={`${prefix}-source`}
							label={m.select_secret_source()}
							value={sourceId}
							onValueChange={selectSource}
							options={(sources.data ?? []).map((source) => ({ value: source.id, label: source.name }))}
						/>
					</div>
					<div class="flex flex-col gap-2">
						<Label for={`${prefix}-project`}>{m.infisical_project_id()}</Label><Input
							id={`${prefix}-project`}
							bind:value={target.projectId}
						/>
					</div>
					<div class="flex flex-col gap-2">
						<Label for={`${prefix}-environment`}>{m.environment()}</Label><Input
							id={`${prefix}-environment`}
							bind:value={target.environment}
						/>
					</div>
					<div class="flex flex-col gap-2">
						<Label for={`${prefix}-path`}>{m.secret_path()}</Label><Input id={`${prefix}-path`} bind:value={target.path} />
					</div>
				</div>
				<Button variant="outline" disabled={!sourceId || busy} onclick={preview}>{m.preview_secret_keys()}</Button>
				{#if previewError}<p role="alert" class="text-destructive">{previewError}</p>{/if}
				{#if stalePreview}<p role="status">{m.preview_changed_secret_target()}</p>{/if}
				{#if keys.length}<p class="text-sm text-muted-foreground">{m.select_secret_keys()}</p>{/if}
				{#if composeWorkload && keys.length}
					<Button
						variant="outline"
						onclick={reviewSuggestions}
						disabled={busy || stalePreview || suggestions.status !== 'available'}>{m.suggest_secret_mappings()}</Button
					>
					<p class="text-sm text-muted-foreground">{m.review_secret_mapping_suggestions()}</p>
					{#if suggestions.status === 'missing'}<p role="status">
							{m.secret_mapping_source_missing()}
						</p>{:else if suggestions.status === 'invalid'}<p role="alert" class="text-destructive">
							{m.secret_mapping_source_invalid()}
						</p>{/if}
					{#if suggestions.hasExternalInputs || composeIncomplete}<p role="status">{m.secret_mapping_external_inputs()}</p>{/if}
					{#if reviewedSuggestions && mappings.length === 0}<p role="status">{m.no_secret_mapping_suggestions()}</p>{/if}
					{#if unmatchedKeys.length}<p class="text-sm">
							{m.unmapped_secret_keys()}: <span class="font-mono">{unmatchedKeys.join(', ')}</span>
						</p>{/if}
				{/if}
				{#each mappings as mapping, index (`${mapping.key}-${index}`)}
					<div class="grid items-center gap-3 rounded-xl border p-3 sm:grid-cols-4">
						<span class="truncate font-mono text-sm">{mapping.key}</span>
						<Input aria-label={m.destination_name()} bind:value={mapping.name} />
						<SelectWithLabel
							id={`${prefix}-delivery-${index}`}
							label={m.secret_delivery()}
							hideLabel
							value={mapping.delivery}
							onValueChange={(value) => {
								mapping.delivery = value as 'environment' | 'file';
							}}
							options={nativeSupported
								? [
										{ value: 'environment', label: m.environment_variable() },
										{ value: 'file', label: m.native_secret_file() }
									]
								: [{ value: 'environment', label: m.environment_variable() }]}
						/>
						<div class="flex gap-2">
							<Input aria-label={m.service_name_optional()} placeholder={m.all_services()} bind:value={mapping.service} /><Button
								variant="ghost"
								onclick={() => {
									mappings = mappings.filter((_, i) => i !== index);
								}}>{m.common_delete()}</Button
							>
						</div>
						{#if mapping.delivery === 'file'}
							<Input aria-label={m.secret_file_uid()} placeholder="0" bind:value={mapping.uid} />
							<Input aria-label={m.secret_file_gid()} placeholder="0" bind:value={mapping.gid} />
							<Input
								aria-label={m.secret_file_mode()}
								value={(mapping.mode ?? 0o400).toString(8)}
								oninput={(event) => {
									const value = event.currentTarget.value;
									if (/^[0-7]{3,4}$/.test(value)) {
										mapping.mode = Number.parseInt(value, 8);
										previewError = '';
									} else {
										previewError = m.invalid_secret_file_mode();
									}
								}}
							/>
						{/if}
					</div>
				{/each}
				{#if keys.length}
					<div class="grid items-end gap-3 rounded-xl border p-3 sm:grid-cols-3">
						<SelectWithLabel
							id={`${prefix}-manual-key`}
							label={m.secret_keys()}
							value={manualKey}
							onValueChange={(value) => {
								manualKey = value;
							}}
							options={keys.map((key) => ({ value: key, label: key }))}
						/>
						{#if suggestions.services.length && composeWorkload}
							<SelectWithLabel
								id={`${prefix}-manual-service`}
								label={m.recipient_service()}
								value={manualService}
								onValueChange={(value) => {
									manualService = value;
								}}
								options={suggestions.services.map((service) => ({ value: service, label: service }))}
							/>
						{:else}
							<Input aria-label={m.service_name_optional()} placeholder={m.service_name_optional()} bind:value={manualService} />
						{/if}
						<Button
							variant="outline"
							disabled={busy || !manualKey || (composeWorkload && !manualService.trim())}
							onclick={() => {
								mappings = [
									...mappings,
									{ key: manualKey, name: manualKey, delivery: 'environment', service: manualService.trim() }
								];
							}}>{m.add_secret_mapping()}</Button
						>
					</div>
				{/if}
				<div class="flex items-center gap-3">
					<Switch id={`${prefix}-required`} bind:checked={required} /><Label for={`${prefix}-required`}
						>{m.required_secrets()}</Label
					>
				</div>
				<div class="flex items-center gap-3">
					<Switch id={`${prefix}-automatic`} bind:checked={autoRollout} /><Label for={`${prefix}-automatic`}
						>{m.automatic_secret_rollout()}</Label
					>
				</div>
				<p class="text-sm text-muted-foreground">{m.automatic_secret_rollout_description()}</p>
				{#if owner.id && owner.kind === 'project' && hasPermission('secret-sources:manage', environmentId)}
					<SecretSetup
						onTargetChange={(value) => {
							target = value;
						}}
						{environmentId}
						{owner}
						{sourceId}
						{target}
						onComplete={async () => {
							await bindings.refetch();
						}}
					/>
				{/if}
				{#if nativeSupported}<p class="text-sm text-muted-foreground">{m.native_secret_delivery_description()}</p>{/if}
				{#if redeployError}
					<p role="alert" class="text-destructive">
						{#if redeployPending}{m.saved_mappings_redeploy_failed()}
						{/if}{redeployError}
					</p>
				{/if}
				<p class="text-sm text-muted-foreground">
					{onDraft ? m.use_secret_mappings_description() : m.save_and_redeploy_description()}
				</p>
				<div class="flex flex-wrap gap-2">
					<Button onclick={save} disabled={!canSubmit}
						>{#if onDraft}{m.use_secret_mappings()}{:else if busy}{m.saving_and_redeploying()}{:else}{m.projects_save_and_redeploy()}{/if}</Button
					>
					<Button variant="outline" onclick={check} disabled={busy || Boolean(onDraft)}>{m.check_secrets()}</Button>
				</div>
			</div></Card.Content
		>
	</Card.Root>
{/if}
