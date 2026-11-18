<script lang="ts">
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { m } from '#lib/paraglide/messages.js';
	import { secretSourceService } from '#lib/services/secret-source-service.js';
	import type { SecretOwner, SecretSetupRequest, SecretSetupResult, SecretTarget } from '#lib/types/secret-source.js';
	import { handleApiResultWithCallbacks } from '#lib/utils/api.js';
	import { tryCatch } from '#lib/utils/try-catch.js';

	let {
		environmentId,
		owner,
		sourceId,
		target,
		onTargetChange,
		onComplete
	}: {
		environmentId: string;
		owner: SecretOwner;
		sourceId: string;
		target: SecretTarget;
		onTargetChange: (target: SecretTarget) => void;
		onComplete: () => Promise<void>;
	} = $props();
	let result = $state<SecretSetupResult | null>(null);
	let keys = $state<string[]>([]);
	let newProjectName = $state('');
	let replaceExisting = $state(false);
	let removeLocal = $state(false);
	let backup = $state(true);
	let busy = $state(false);
	const stepLabels: Record<string, () => string> = {
		create_project: m.setup_step_project,
		create_folders: m.setup_step_folders,
		read_target: m.setup_step_read,
		create_secrets: m.setup_step_create,
		update_secrets: m.setup_step_update,
		approval: m.setup_step_approval,
		find_organization: m.setup_step_identity,
		find_deploy_identity: m.setup_step_identity,
		grant_read_access: m.setup_step_access,
		verify_deploy_identity: m.setup_step_verify,
		verify_values: m.setup_step_verify,
		bind_project: m.setup_step_bind,
		sync_agent: m.setup_step_sync,
		remove_local_values: m.setup_step_cleanup
	};
	async function run(operation: 'plan' | 'apply') {
		busy = true;
		const request: SecretSetupRequest = { owner, sourceId, target, keys, newProjectName, replaceExisting, removeLocal, backup };
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.setup(environmentId, request, operation)),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
			onSuccess: async (value) => {
				result = value;
				if (operation === 'apply' && value.steps.some((step) => step.name === 'create_project' && step.status === 'completed')) {
					onTargetChange(value.target);
					newProjectName = '';
				}
				if (operation === 'plan') keys = value.variables.filter((item) => item.hasLocalValue).map((item) => item.key);
				if (value.verified) await onComplete();
			}
		});
	}
</script>

<details class="rounded-xl border p-4">
	<summary class="cursor-pointer font-medium">{m.guided_infisical_setup()}</summary>
	<div class="mt-4 flex flex-col gap-4">
		<Label for="setup-project-name">{m.new_infisical_project_name()}</Label><Input
			id="setup-project-name"
			bind:value={newProjectName}
		/>
		<Button variant="outline" onclick={() => run('plan')} disabled={busy || !sourceId}>{m.plan_secret_setup()}</Button>
		{#each result?.variables ?? [] as variable (variable.key)}
			<label class="flex items-center gap-3"
				><input type="checkbox" value={variable.key} bind:group={keys} disabled={!variable.hasLocalValue} /><span
					class="font-mono text-sm">{variable.key}</span
				>{#if !variable.hasLocalValue}<span class="text-sm text-muted-foreground">{m.setup_missing_local_value()}</span
					>{:else if variable.differs}<span class="text-sm text-muted-foreground">{m.setup_differing_secret()}</span
					>{:else if variable.existsInInfisical}<span class="text-sm text-muted-foreground">{m.setup_existing_secret()}</span
					>{/if}</label
			>
		{/each}
		<div class="flex items-center gap-3">
			<Switch id="setup-replace" bind:checked={replaceExisting} /><Label for="setup-replace">{m.replace_existing_secrets()}</Label
			>
		</div>
		<div class="flex items-center gap-3">
			<Switch id="setup-remove" bind:checked={removeLocal} /><Label for="setup-remove">{m.remove_verified_local_values()}</Label>
		</div>
		<div class="flex items-center gap-3">
			<Switch id="setup-backup" bind:checked={backup} /><Label for="setup-backup">{m.backup_local_environment()}</Label>
		</div>
		<Button onclick={() => run('apply')} disabled={busy || keys.length === 0}>{m.apply_secret_setup()}</Button>
		{#if result?.steps.length}<p role="status">{result.verified ? m.setup_verified() : m.setup_incomplete()}</p>{/if}
		{#each result?.steps ?? [] as step, index (`${step.name}-${index}`)}
			<p class="text-sm">
				{(stepLabels[step.name] ?? m.guided_infisical_setup)()}: {#if step.status === 'completed'}{m.setup_completed()}{:else if step.status === 'pending'}{m.setup_pending()}{:else}{m.setup_failed()}{/if}
			</p>
		{/each}
	</div>
</details>
