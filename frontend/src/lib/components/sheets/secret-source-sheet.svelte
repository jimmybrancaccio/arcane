<script lang="ts">
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as ResponsiveDialog from '#lib/components/ui/responsive-dialog/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { m } from '#lib/paraglide/messages.js';
	import type { SecretSource, SecretSourceWrite } from '#lib/types/secret-source.js';

	let {
		open = $bindable(false),
		source,
		busy = false,
		onSave
	}: {
		open: boolean;
		source: SecretSource | null;
		busy?: boolean;
		onSave: (value: SecretSourceWrite) => Promise<void>;
	} = $props();

	function initial(): SecretSourceWrite {
		return {
			name: source?.name ?? '',
			settings: {
				host: source?.settings.host ?? 'https://app.infisical.com',
				clientId: source?.settings.clientId ?? '',
				setupClientId: source?.settings.setupClientId ?? '',
				defaults: {
					projectId: source?.settings.defaults.projectId ?? '',
					environment: source?.settings.defaults.environment ?? 'prod',
					path: source?.settings.defaults.path ?? '/'
				}
			},
			credential: '',
			setupCredential: '',
			default: source?.default ?? false
		};
	}
	let form = $state<SecretSourceWrite>(initial());
	$effect(() => {
		if (open) form = initial();
	});
</script>

<ResponsiveDialog.Root
	bind:open
	title={source ? m.edit_secret_source() : m.add_secret_source()}
	variant="sheet"
	contentClass="sm:max-w-2xl"
>
	<form
		id="secret-source-form"
		class="space-y-5 py-5"
		onsubmit={async (event) => {
			event.preventDefault();
			await onSave(form);
		}}
	>
		<div class="flex flex-col gap-2">
			<Label for="source-name">{m.common_name()}</Label><Input id="source-name" bind:value={form.name} required maxlength={128} />
		</div>
		<div class="flex flex-col gap-2">
			<Label for="source-host">{m.api_host()}</Label><Input
				id="source-host"
				bind:value={form.settings.host}
				required
				type="url"
			/>
		</div>
		<div class="grid gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-2">
				<Label for="source-client">{m.universal_auth_client_id()}</Label><Input
					id="source-client"
					bind:value={form.settings.clientId}
					required
				/>
			</div>
			<div class="flex flex-col gap-2">
				<Label for="source-credential">{m.universal_auth_client_secret()}</Label><Input
					id="source-credential"
					bind:value={form.credential}
					type="password"
					autocomplete="new-password"
					required={!source}
					placeholder={source ? m.keep_stored_credential() : ''}
				/>
			</div>
			<div class="flex flex-col gap-2">
				<Label for="source-project">{m.infisical_project_id()}</Label><Input
					id="source-project"
					bind:value={form.settings.defaults.projectId}
				/>
			</div>
			<div class="flex flex-col gap-2">
				<Label for="source-environment">{m.environment()}</Label><Input
					id="source-environment"
					bind:value={form.settings.defaults.environment}
				/>
			</div>
			<div class="flex flex-col gap-2">
				<Label for="source-path">{m.secret_path()}</Label><Input id="source-path" bind:value={form.settings.defaults.path} />
			</div>
		</div>
		<p class="text-sm text-muted-foreground">{m.source_defaults_description()}</p>
		<div class="grid gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-2">
				<Label for="source-setup-client">{m.setup_client_id()}</Label><Input
					id="source-setup-client"
					bind:value={form.settings.setupClientId}
				/>
			</div>
			<div class="flex flex-col gap-2">
				<Label for="source-setup-secret">{m.setup_client_secret()}</Label><Input
					id="source-setup-secret"
					bind:value={form.setupCredential}
					type="password"
					autocomplete="new-password"
					placeholder={source ? m.keep_stored_credential() : ''}
				/>
			</div>
		</div>
		<div class="flex items-center gap-3">
			<Switch id="source-default" bind:checked={form.default} /><Label for="source-default">{m.default_secret_source()}</Label>
		</div>
	</form>
	{#snippet footer()}
		<Button
			variant="outline"
			onclick={() => {
				open = false;
			}}
			disabled={busy}>{m.common_cancel()}</Button
		>
		<Button type="submit" form="secret-source-form" disabled={busy}>{m.common_save()}</Button>
	{/snippet}
</ResponsiveDialog.Root>
