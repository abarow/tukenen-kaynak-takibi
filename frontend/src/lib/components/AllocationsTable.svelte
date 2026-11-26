<script lang="ts">
	import { goto, invalidate } from '$app/navigation';
	import { page as appPage } from '$app/state';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { dateTime, num, relative, statusLabel } from '../format';
	import type { Allocation, AllocationPage, Pool } from '../types';
	import StatusBadge from './StatusBadge.svelte';

	let {
		data,
		filter,
		limit = 25,
		pools = null
	}: {
		data: AllocationPage;
		filter: { status: string; q: string; offset: number; pool_id?: number };
		limit?: number;
		pools?: Pool[] | null; // verilirse havuz sütunu ve filtresi gösterilir
	} = $props();

	let q = $state('');
	// URL başka bir yerden değişirse (ör. "diğer tahsisleri" bağlantısı) arama kutusunu eşitle.
	$effect.pre(() => {
		q = filter.q;
	});
	let busyId = $state<number | null>(null);

	function setFilter(patch: Record<string, string | number>) {
		const sp = new URLSearchParams(appPage.url.search);
		const next: Record<string, string | number> = { offset: 0, ...patch };
		for (const [k, v] of Object.entries(next)) {
			if (v === '' || v === 0 || v === '0') sp.delete(k);
			else sp.set(k, String(v));
		}
		const s = sp.toString();
		goto(appPage.url.pathname + (s ? '?' + s : ''), { reset: false, replace: true });
	}

	async function act(a: Allocation, kind: 'confirm' | 'release') {
		if (kind === 'release') {
			const msg =
				a.status === 'blocked'
					? `${a.value} bloğu kaldırılsın mı? Değer tekrar dağıtılabilir hale gelir.`
					: `${a.value} değeri ${a.object_type}/${a.object_ref} nesnesinden ayrılsın mı?\n` +
						`Havuz politikasına göre bu değer bir daha kullanılamayabilir.`;
			if (!confirm(msg)) return;
		}
		busyId = a.id;
		try {
			const r = kind === 'confirm' ? await api.confirm(a.id) : await api.release(a.id);
			notify.ok(`${r.value}: ${statusLabel[r.status]}`);
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busyId = null;
		}
	}

	let pageNo = $derived(Math.floor(filter.offset / limit) + 1);
	let pageCount = $derived(Math.max(1, Math.ceil(data.total / limit)));
</script>

<div class="toolbar">
	<form
		onsubmit={(e) => {
			e.preventDefault();
			setFilter({ q });
		}}
	>
		<input type="search" placeholder="Değer veya nesne referansı ara…" bind:value={q} />
	</form>
	<select value={filter.status} onchange={(e) => setFilter({ status: e.currentTarget.value })}>
		<option value="">Tüm durumlar</option>
		{#each ['assigned', 'reserved', 'released', 'blocked'] as const as s (s)}
			<option value={s}>{statusLabel[s]}</option>
		{/each}
	</select>
	{#if pools}
		<select
			value={String(filter.pool_id ?? '')}
			onchange={(e) => setFilter({ pool_id: e.currentTarget.value })}
		>
			<option value="">Tüm havuzlar</option>
			{#each pools as p (p.id)}
				<option value={String(p.id)}>{p.name}</option>
			{/each}
		</select>
	{/if}
	<span class="spacer"></span>
	<span class="small muted">{num(data.total)} kayıt</span>
</div>

<div class="table-wrap">
	<table>
		<thead>
			<tr>
				<th>Değer</th>
				<th>Durum</th>
				<th>Nesne</th>
				{#if pools}<th>Havuz</th>{/if}
				<th>Son değişiklik</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#each data.items as a (a.id)}
				<tr>
					<td><a class="mono" href="/allocations/{a.id}">{a.value}</a></td>
					<td>
						<StatusBadge status={a.status} />
						{#if a.status === 'reserved' && a.reserved_until}
							<div class="small muted">bitiş {relative(a.reserved_until)}</div>
						{:else if a.status === 'released'}
							<div class="small muted">
								{a.reusable_at ? `tekrar kullanım ${relative(a.reusable_at)}` : 'tekrar kullanılmaz'}
							</div>
						{/if}
					</td>
					<td>
						{#if a.object_ref}
							<span class="muted small">{a.object_type}/</span><span>{a.object_ref}</span>
						{:else}
							<span class="muted">—</span>
						{/if}
						{#if a.note}<div class="small muted">{a.note}</div>{/if}
					</td>
					{#if pools}<td><a href="/pools/{a.pool_id}">{a.pool_name}</a></td>{/if}
					<td class="small muted" title={dateTime(a.updated_at)}>{relative(a.updated_at)}</td>
					<td class="actions">
						{#if a.status === 'reserved'}
							<button class="small" disabled={busyId === a.id} onclick={() => act(a, 'confirm')}>Onayla</button>
						{/if}
						{#if a.status === 'reserved' || a.status === 'assigned'}
							<button class="small danger" disabled={busyId === a.id} onclick={() => act(a, 'release')}>
								Bırak
							</button>
						{:else if a.status === 'blocked'}
							<button class="small" disabled={busyId === a.id} onclick={() => act(a, 'release')}>
								Bloğu kaldır
							</button>
						{/if}
					</td>
				</tr>
			{:else}
				<tr><td colspan="6" class="empty">Kayıt yok.</td></tr>
			{/each}
		</tbody>
	</table>
</div>

{#if pageCount > 1}
	<div class="toolbar" style="justify-content: flex-end; margin: 12px 0 0">
		<button class="small" disabled={pageNo <= 1} onclick={() => setFilter({ offset: filter.offset - limit })}>
			‹ Önceki
		</button>
		<span class="small muted">{pageNo} / {pageCount}</span>
		<button
			class="small"
			disabled={pageNo >= pageCount}
			onclick={() => setFilter({ offset: filter.offset + limit })}
		>
			Sonraki ›
		</button>
	</div>
{/if}

<style>
	.toolbar form {
		display: contents;
	}
	.toolbar input[type='search'] {
		min-width: 260px;
	}
</style>
