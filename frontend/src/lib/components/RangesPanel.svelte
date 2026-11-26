<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { approx, num, parseBody, pct, prefixRange } from '../format';
	import type { KindInfo, Pool } from '../types';

	let { pool, kind }: { pool: Pool; kind: KindInfo } = $props();

	let fixed = $derived(kind.base_digits > 0);
	let adding = $state(false);
	let mode = $state<'prefix' | 'explicit'>('prefix');
	let prefix = $state('');
	let start = $state('');
	let end = $state('');
	let busy = $state(false);

	let effMode = $derived(fixed ? mode : 'explicit');
	let preview = $derived.by(() => {
		if (effMode === 'prefix') {
			const r = prefix ? prefixRange(prefix, kind) : null;
			return r ? r.end - r.start + 1 : null;
		}
		const s = parseBody(start, kind);
		const e = parseBody(end, kind);
		return s !== null && e !== null && s <= e ? e - s + 1 : null;
	});

	async function add(ev: SubmitEvent) {
		ev.preventDefault();
		const s = parseBody(start, kind);
		const e = parseBody(end, kind);
		if (effMode === 'explicit' && (s === null || e === null)) {
			notify.error('Başlangıç/bitiş değerleri geçersiz');
			return;
		}
		busy = true;
		try {
			await api.addRange(pool.id, effMode === 'prefix' ? { prefix } : { start: s!, end: e! });
			notify.ok('Aralık eklendi');
			prefix = start = end = '';
			adding = false;
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}

	async function remove(id: number) {
		if (!confirm('Bu aralık silinsin mi?')) return;
		try {
			await api.deleteRange(pool.id, id);
			notify.ok('Aralık silindi');
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		}
	}

	// Blokla
	let blockValue = $state('');
	let blockNote = $state('');
	async function block(ev: SubmitEvent) {
		ev.preventDefault();
		try {
			const a = await api.block(pool.id, blockValue, blockNote);
			notify.ok(`${a.value} bloklandı`);
			blockValue = blockNote = '';
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		}
	}
</script>

<div class="head">
	<h2>Aralıklar</h2>
	{#if !adding}
		<button class="small" onclick={() => (adding = true)}>+ Aralık ekle</button>
	{/if}
</div>

{#if pool.ranges?.length}
	<table>
		<thead>
			<tr>
				<th>İlk – son değer</th>
				<th class="num">Boyut</th>
				<th class="num">İmleç</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#each pool.ranges as r (r.id)}
				<tr>
					<td class="mono small">{r.first_value}<br />{r.last_value}</td>
					<td class="num">
						{approx(r.valid, r.valid_exact)}
						{#if r.valid !== r.size}<div class="small muted">{num(r.size)} gövdeden</div>{/if}
					</td>
					<td class="num small" title="İmlecin geçtiği değer sayısı">
						{pct(r.size - r.untouched, r.size)}
					</td>
					<td class="actions">
						{#if r.next === r.start}
							<button class="small danger" onclick={() => remove(r.id)}>Sil</button>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
	<div class="alert warn">Havuzda aralık yok; değer dağıtmak için bir aralık ekleyin.</div>
{/if}

{#if adding}
	<form class="add" onsubmit={add}>
		{#if fixed}
			<div class="segmented" style="margin-bottom: 8px">
				<button type="button" class:on={mode === 'prefix'} onclick={() => (mode = 'prefix')}>Önekten</button>
				<button type="button" class:on={mode === 'explicit'} onclick={() => (mode = 'explicit')}>
					Başlangıç – bitiş
				</button>
			</div>
		{/if}
		{#if effMode === 'prefix'}
			<div class="field">
				<input
					class="mono"
					placeholder={kind.radix === 16 ? 'önek, ör. 02:4B:5A (MA-L)' : 'önek'}
					bind:value={prefix}
					required
				/>
			</div>
		{:else}
			{@const hex = kind.radix === 16}
			<div class="row">
				<div class="field">
					<input
						class="mono"
						placeholder={hex ? 'başlangıç adresi' : 'başlangıç (gövde)'}
						inputmode={hex ? 'text' : 'numeric'}
						bind:value={start}
						required
					/>
				</div>
				<div class="field">
					<input
						class="mono"
						placeholder={hex ? 'bitiş adresi' : 'bitiş (gövde)'}
						inputmode={hex ? 'text' : 'numeric'}
						bind:value={end}
						required
					/>
				</div>
			</div>
		{/if}
		<div class="row" style="align-items: center">
			<span class="small muted">
				{#if preview && kind.density}
					{num(preview)} gövde → ≈ {num(Math.round(preview * kind.density))} geçerli kod
				{:else if preview}
					{num(preview)} değer eklenecek
				{/if}
			</span>
			<span class="spacer"></span>
			<button type="button" class="small" onclick={() => (adding = false)}>Vazgeç</button>
			<button class="small primary" disabled={busy}>Ekle</button>
		</div>
	</form>
{/if}

<details class="block">
	<summary>Değer blokla</summary>
	<p class="small muted">
		Başka bir sistemde zaten kullanılmış, hasarlı basılmış ya da özel amaçla ayrılmış değerleri dağıtım dışı bırakın.
	</p>
	<form onsubmit={block}>
		<div class="row">
			<div class="field">
				<input class="mono" placeholder="tam değer" bind:value={blockValue} required />
			</div>
			<div class="field">
				<input placeholder="sebep" bind:value={blockNote} />
			</div>
		</div>
		<button class="small danger">Blokla</button>
	</form>
</details>

<style>
	.head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 8px;
	}
	.head h2 {
		margin: 0;
	}
	.add {
		margin-top: 12px;
		padding-top: 12px;
		border-top: 1px solid var(--border);
	}
	.add .field {
		margin-bottom: 8px;
	}
	.block {
		margin-top: 14px;
		padding-top: 12px;
		border-top: 1px solid var(--border);
	}
	.block p {
		margin: 6px 0 10px;
	}
</style>
