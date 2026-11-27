<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { untrack } from 'svelte';
	import { invalidate } from '$app/navigation';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { downloadCSV, statusLabel } from '../format';
	import type { Allocation, KindCode, Pool } from '../types';
	import StatusBadge from './StatusBadge.svelte';

	let { pool }: { pool: Pool } = $props();

	const defaultType: Record<KindCode, string> = {
		ean13: 'product',
		ean8: 'product',
		isbn13: 'book',
		mac: 'device',
		sequence: 'document',
		custom: 'product'
	};
	const refHint: Record<KindCode, string> = {
		ean13: 'ör. SKU-10045, ERP ürün kodu',
		ean8: 'ör. SKU-10045, ERP ürün kodu',
		isbn13: 'ör. kitap/baskı kodu',
		mac: 'ör. cihaz seri no veya sanal makine adı',
		sequence: 'ör. belge kimliği',
		custom: 'ör. SKU-10045, ERP ürün kodu'
	};

	// Bileşen havuz başına yeniden oluşturulur ({#key}), başlangıç değeri yeterli.
	const typeKey = untrack(() => `kaynak.objtype.${pool.id}`);

	let tab = $state<'single' | 'bulk'>('single');
	let objectType = $state(untrack(() => localStorage.getItem(typeKey) ?? defaultType[pool.kind]));
	let objectRef = $state('');
	let mode = $state<'assign' | 'reserve'>('assign');
	let value = $state('');
	let note = $state('');
	let bulkRefs = $state('');
	let busy = $state(false);

	let result = $state<{ allocation: Allocation; created: boolean } | null>(null);
	let bulkResult = $state<{ allocation: Allocation; created: boolean }[] | null>(null);

	let refs = $derived(
		bulkRefs
			.split(/\r?\n/)
			.map((s) => s.trim())
			.filter(Boolean)
	);

	function rememberType() {
		localStorage.setItem(typeKey, objectType.trim());
	}

	async function single(ev: SubmitEvent) {
		ev.preventDefault();
		busy = true;
		rememberType();
		try {
			result = await api.allocate(pool.id, {
				object_type: objectType,
				object_ref: objectRef,
				mode,
				value: value || undefined,
				note: note || undefined
			});
			notify.ok(
				result.created
					? `${result.allocation.value} → ${objectRef}`
					: `${objectRef} nesnesinin zaten bir değeri var: ${result.allocation.value}`
			);
			objectRef = '';
			value = '';
			note = '';
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}

	async function bulk(ev: SubmitEvent) {
		ev.preventDefault();
		if (refs.length === 0) return;
		busy = true;
		rememberType();
		try {
			const r = await api.allocateBulk(
				pool.id,
				refs.map((ref) => ({ object_type: objectType, object_ref: ref, mode, note: note || undefined }))
			);
			bulkResult = r.items;
			const fresh = r.items.filter((i) => i.created).length;
			notify.ok(`${fresh} yeni değer verildi${fresh < r.items.length ? `, ${r.items.length - fresh} nesnenin zaten değeri vardı` : ''}`);
			bulkRefs = '';
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}

	function exportBulk() {
		if (!bulkResult) return;
		downloadCSV(`${pool.code}-tahsis.csv`, [
			['nesne_tipi', 'nesne_ref', 'deger', 'durum', 'yeni'],
			...bulkResult.map((r) => [
				r.allocation.object_type,
				r.allocation.object_ref,
				r.allocation.value,
				statusLabel[r.allocation.status],
				r.created ? 'evet' : 'hayır'
			])
		]);
	}

	async function copy(text: string) {
		await navigator.clipboard.writeText(text);
		notify.ok('Kopyalandı');
	}
</script>

<div class="head">
	<h2>Değer tahsis et</h2>
	<div class="segmented">
		<button type="button" class:on={tab === 'single'} onclick={() => (tab = 'single')}>Tekil</button>
		<button type="button" class:on={tab === 'bulk'} onclick={() => (tab = 'bulk')}>Toplu</button>
	</div>
</div>

{#if !pool.active}
	<div class="alert warn">Havuz pasif; tahsis yapılamaz.</div>
{:else}
	<form onsubmit={tab === 'single' ? single : bulk}>
		<div class="row">
			<div class="field" style="flex: 0 1 150px">
				<label for="ot">Nesne tipi</label>
				<input id="ot" bind:value={objectType} required maxlength="64" />
			</div>
			{#if tab === 'single'}
				<div class="field">
					<label for="or">Nesne referansı</label>
					<input id="or" bind:value={objectRef} required maxlength="256" placeholder={refHint[pool.kind]} />
				</div>
			{/if}
		</div>

		{#if tab === 'bulk'}
			<div class="field">
				<label for="refs">Nesne referansları <span class="muted">(her satıra bir tane, en fazla 1000)</span></label>
				<textarea id="refs" rows="6" bind:value={bulkRefs} placeholder={'SKU-1001\nSKU-1002\nSKU-1003'}></textarea>
				<div class="hint">
					{refs.length} kalem. İşlem tek parça yapılır: havuz yetmezse hiçbir değer verilmez.
				</div>
			</div>
		{/if}

		<div class="row" style="align-items: flex-end">
			<div class="field" style="flex: 0 0 auto">
				<span class="lbl">İşlem</span>
				<div class="segmented">
					<button type="button" class:on={mode === 'assign'} onclick={() => (mode = 'assign')}>Ata</button>
					<button type="button" class:on={mode === 'reserve'} onclick={() => (mode = 'reserve')}>Rezerve et</button>
				</div>
			</div>
			{#if tab === 'single'}
				<div class="field">
					<label for="val">Belirli değer <span class="muted">(opsiyonel)</span></label>
					<input id="val" class="mono" bind:value={value} placeholder="boş = sıradaki boş değer" />
				</div>
			{/if}
		</div>
		{#if mode === 'reserve'}
			<div class="hint" style="margin: -6px 0 10px">
				Rezervasyon {Math.round(pool.reservation_ttl_seconds / 60)} dk içinde onaylanmazsa değer havuza geri
				döner.
			</div>
		{/if}

		<div class="field">
			<label for="note">Not</label>
			<input id="note" bind:value={note} placeholder="opsiyonel" />
		</div>

		<button class="primary" disabled={busy || (tab === 'bulk' && refs.length === 0)}>
			{tab === 'bulk' ? `${refs.length} nesne için ` : ''}{mode === 'assign' ? 'Ata' : 'Rezerve et'}
		</button>
	</form>

	{#if tab === 'single' && result}
		<div class="result">
			<div class="value mono">{result.allocation.value}</div>
			<button class="small" onclick={() => copy(result!.allocation.value)}>Kopyala</button>
			<div class="small">
				<StatusBadge status={result.allocation.status} />
				<span class="muted">
					{result.allocation.object_type}/{result.allocation.object_ref}
					{result.created ? '' : '· mevcut tahsis döndü'}
				</span>
			</div>
		</div>
	{/if}

	{#if tab === 'bulk' && bulkResult}
		<div class="bulk-head">
			<strong>{bulkResult.length} sonuç</strong>
			<button class="small" onclick={exportBulk}>CSV indir</button>
		</div>
		<div class="bulk-list">
			<table>
				<tbody>
					{#each bulkResult as r (r.allocation.id)}
						<tr>
							<td>{r.allocation.object_ref}</td>
							<td class="mono">{r.allocation.value}</td>
							<td>{r.created ? '' : 'mevcut'}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/if}

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}
	.head h2 {
		margin: 0;
	}
	.lbl {
		display: block;
		font-size: 12px;
		font-weight: 600;
		color: var(--muted);
		margin-bottom: 4px;
	}
	.result {
		margin-top: 14px;
		padding: 12px 14px;
		border-radius: var(--radius);
		background: var(--ok-soft);
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}
	.result .value {
		font-size: 22px;
		font-weight: 650;
		letter-spacing: 0.03em;
	}
	.bulk-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-top: 14px;
	}
	.bulk-list {
		max-height: 240px;
		overflow: auto;
		border: 1px solid var(--border);
		border-radius: 6px;
		margin-top: 6px;
	}
	.bulk-list td {
		padding: 4px 10px;
	}
</style>
