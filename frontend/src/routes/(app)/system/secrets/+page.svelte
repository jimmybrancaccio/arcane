<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';

	import SecretSourceSheet from '#lib/components/sheets/secret-source-sheet.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import { SettingsPageLayout } from '#lib/layouts/index.js';
	import { m } from '#lib/paraglide/messages.js';
	import { queryKeys } from '#lib/query/query-keys.js';
	import { secretSourceService } from '#lib/services/secret-source-service.js';
	import type { SecretSource, SecretSourceWrite } from '#lib/types/secret-source.js';
	import { extractApiErrorMessage, handleApiResultWithCallbacks } from '#lib/utils/api.js';
	import { hasShownPermission } from '#lib/utils/environments.js';
	import { tryCatch } from '#lib/utils/try-catch.js';

	const client = useQueryClient();
	const sources = createQuery(() => ({ queryKey: queryKeys.secretSources.all, queryFn: () => secretSourceService.list() }));
	let open = $state(false);
	let selected = $state<SecretSource | null>(null);
	let busy = $state(false);
	const canManage = $derived(hasShownPermission('secret-sources:manage'));

	async function save(value: SecretSourceWrite) {
		busy = true;
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.save(selected?.id ?? '', value)),
			message: m.secret_operation_failed(),
			setLoadingState: (value) => {
				busy = value;
			},
			onSuccess: async () => {
				open = false;
				await client.invalidateQueries({ queryKey: queryKeys.secretSources.all });
				toast.success(m.secret_source_saved());
			}
		});
	}
	async function test(source: SecretSource) {
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.test(source.id, source.settings.defaults)),
			message: m.secret_operation_failed(),
			onSuccess: async () => {
				toast.success(m.infisical_target_accessible());
				await sources.refetch();
			}
		});
	}
	async function remove(source: SecretSource) {
		await handleApiResultWithCallbacks({
			result: await tryCatch(secretSourceService.delete(source.id)),
			message: m.secret_operation_failed(),
			onSuccess: async () => {
				await sources.refetch();
			}
		});
	}
</script>

<SettingsPageLayout title={m.secret_sources()} description={m.infisical_description()} pageType="management">
	{#snippet mainContent()}
		<div class="space-y-5">
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => sources.refetch()}>{m.common_refresh()}</Button>
				{#if canManage}<Button
						onclick={() => {
							selected = null;
							open = true;
						}}>{m.add_secret_source()}</Button
					>{/if}
			</div>
			{#if sources.error}<p role="alert" class="text-destructive">{extractApiErrorMessage(sources.error)}</p>{/if}
			<div class="grid gap-4 lg:grid-cols-2">
				{#each sources.data ?? [] as source (source.id)}
					<Card.Root>
						<Card.Header
							><Card.Title>{source.name}</Card.Title><Card.Description>{source.settings.host}</Card.Description></Card.Header
						>
						<Card.Content
							><div class="flex flex-col gap-3">
								<p class="text-sm text-muted-foreground">
									{source.settings.defaults.environment} · {source.settings.defaults.path}
								</p>
								{#if source.default}<p class="text-sm">{m.default_secret_source()}</p>{/if}
								{#if source.lastError}<p role="alert" class="text-sm text-destructive">{source.lastError}</p>{/if}
								{#if canManage}<div class="flex flex-wrap gap-2">
										<Button variant="outline" onclick={() => test(source)}>{m.test_connection()}</Button><Button
											variant="outline"
											onclick={() => {
												selected = source;
												open = true;
											}}>{m.common_edit()}</Button
										><Button variant="destructive" onclick={() => remove(source)}>{m.common_delete()}</Button>
									</div>{/if}
							</div></Card.Content
						>
					</Card.Root>
				{/each}
			</div>
		</div>
	{/snippet}
	{#snippet additionalContent()}<SecretSourceSheet bind:open source={selected} {busy} onSave={save} />{/snippet}
</SettingsPageLayout>
