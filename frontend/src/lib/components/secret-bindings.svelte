<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';

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
	import { tryCatch } from '#lib/utils/try-catch.js';

	let {
		environmentId,
		owner,
		onDraft
	}: { environmentId: string; owner: SecretOwner; onDraft?: (value: SecretBindingWrite) => void } = $props();
	const client = useQueryClient();
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
	async function preview() {
		previewError = '';
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.keys(sourceId, target)),
			message: m.secret_operation_failed(),
			onSuccess: (result) => {
				keys = result.keys;
				mappings = keys.map((key) => ({ key, name: key, delivery: 'environment' }));
			},
			onError: (error) => {
				keys = [];
				mappings = [];
				previewError = extractApiErrorMessage(error);
			}
		});
	}
	function edit(binding: SecretBinding) {
		editingId = binding.id;
		sourceId = binding.sourceId;
		target = { ...binding.target };
		mappings = binding.mappings.map((mapping) => ({ ...mapping }));
		keys = binding.keys;
		required = binding.required;
		autoRollout = binding.autoRollout;
	}
	async function save() {
		busy = true;
		const value: SecretBindingWrite = {
			owner,
			sourceId,
			target,
			mappings,
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
			result: await tryCatch(secretSourceService.saveBinding(environmentId, editingId, value)),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
			onSuccess: async () => {
				editingId = '';
				await client.invalidateQueries({ queryKey: queryKeys.secretSources.bindings(environmentId, owner.kind, owner.id) });
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
	async function apply() {
		busy = true;
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.apply(environmentId, owner)),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
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
							<p class="text-sm text-muted-foreground">{binding.keys.join(', ')}</p>
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
				<Button variant="outline" disabled={!sourceId} onclick={preview}>{m.preview_secret_keys()}</Button>
				{#if previewError}<p role="alert" class="text-destructive">{previewError}</p>{/if}
				{#if keys.length}<p class="text-sm text-muted-foreground">{m.select_secret_keys()}</p>{/if}
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
				<div class="flex flex-wrap gap-2">
					<Button onclick={save} disabled={busy || !sourceId || mappings.length === 0 || Boolean(previewError)}
						>{m.bind_secrets()}</Button
					><Button variant="outline" onclick={check} disabled={busy}>{m.check_secrets()}</Button><Button
						variant="outline"
						onclick={apply}
						disabled={busy || !bindings.data?.length}>{m.apply_secret_changes()}</Button
					>
				</div>
			</div></Card.Content
		>
	</Card.Root>
{/if}
