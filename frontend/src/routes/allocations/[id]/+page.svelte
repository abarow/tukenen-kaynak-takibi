<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { api } from '#lib/api.ts';
	import EventList from '#lib/components/EventList.svelte';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import { dateTime, kindLabel, relative, statusLabel } from '#lib/format.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let a = $derived(data.allocation);
	let note = $state('');
	let busy = $state(false);
	let hasMeta = $derived(Object.keys(a.metadata ?? {}).length > 0);

	async function run(kind: 'confirm' | 'release') {
		if (kind === 'release' && !confirm(`${a.value} bırakılsın mı?`)) return;
		busy = true;
		try {
			const r = kind === 'confirm' ? await api.confirm(a.id, note) : await api.release(a.id, note);
			notify.ok(`${r.value}: ${statusLabel[r.status]}`);
			note = '';
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}
</script>

<div class="page-head">
	<div>
		<div class="small"><a href="/pools/{a.pool_id}">{a.pool_name}</a> / tahsis #{a.id}</div>
		<h1 class="value mono">{a.value}</h1>
		<p><span class="tag accent">{kindLabel(a)}</span> <StatusBadge status={a.status} /></p>
	</div>
</div>

<div class="grid two">
	<section class="card">
		<h2>Bilgiler</h2>
		<dl>
			<dt>Nesne</dt>
			<dd>
				{#if a.object_ref}
					<span class="muted">{a.object_type}/</span>{a.object_ref}
					<a class="small" href="/allocations?q={encodeURIComponent(a.object_ref)}">diğer tahsisleri</a>
				{:else}—{/if}
			</dd>
			<dt>Durum</dt>
			<dd>
				{statusLabel[a.status]}
				{#if a.status === 'reserved'}
					<span class="muted">· bitiş {relative(a.reserved_until)} ({dateTime(a.reserved_until)})</span>
				{:else if a.status === 'released'}
					<span class="muted">
						· {a.reusable_at ? `tekrar kullanılabilir: ${dateTime(a.reusable_at)}` : 'bir daha dağıtılmaz'}
					</span>
				{/if}
			</dd>
			<dt>Atanma</dt>
			<dd>{dateTime(a.assigned_at)}</dd>
			<dt>Bırakılma</dt>
			<dd>{dateTime(a.released_at)}</dd>
			<dt>İlk kayıt</dt>
			<dd>{dateTime(a.created_at)}</dd>
			{#if a.note}
				<dt>Not</dt>
				<dd>{a.note}</dd>
			{/if}
		</dl>
		{#if hasMeta}
			<h3 style="margin-top: 14px">Metadata</h3>
			<pre>{JSON.stringify(a.metadata, null, 2)}</pre>
		{/if}

		{#if a.status !== 'released'}
			<div class="actions">
				<input placeholder="not (opsiyonel)" bind:value={note} />
				{#if a.status === 'reserved'}
					<button class="primary" disabled={busy} onclick={() => run('confirm')}>Onayla</button>
				{/if}
				<button class={a.status === 'blocked' ? '' : 'danger'} disabled={busy} onclick={() => run('release')}>
					{a.status === 'blocked' ? 'Bloğu kaldır' : 'Bırak'}
				</button>
			</div>
		{/if}
	</section>

	<section class="card">
		<h2>Geçmiş</h2>
		<EventList events={a.events} showValue={false} />
	</section>
</div>

<style>
	.value {
		font-size: 28px;
		letter-spacing: 0.04em;
	}
	dl {
		display: grid;
		grid-template-columns: 110px 1fr;
		gap: 6px 12px;
		margin: 0;
	}
	dt {
		color: var(--muted);
		font-size: 13px;
	}
	dd {
		margin: 0;
	}
	pre {
		background: var(--surface-2);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 10px;
		font-size: 12px;
		overflow: auto;
		margin: 0;
	}
	.actions {
		display: flex;
		gap: 8px;
		margin-top: 16px;
		padding-top: 14px;
		border-top: 1px solid var(--border);
	}
</style>
