<script lang="ts">
	import { api } from '#lib/api.ts';
	import StatusBadge from '#lib/components/StatusBadge.svelte';
	import { notify } from '#lib/toast.svelte.ts';
	import { kindKey } from '#lib/format.ts';
	import type { LookupMatch, ValidationResult } from '#lib/types.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	// Doğrulama
	let selected = $state('ean13');
	let value = $state('');
	let prefix = $state('');
	let pad = $state(0);
	let vres = $state<ValidationResult | null>(null);
	let info = $derived(data.kinds.find((k) => kindKey(k) === selected)!);
	let kind = $derived(info.code);

	async function validate(ev: SubmitEvent) {
		ev.preventDefault();
		try {
			const seq = kind === 'sequence';
			vres = await api.validate(kind, value, seq ? prefix : '', seq ? pad : 0, info.custom_kind ?? '');
		} catch (e) {
			notify.error(e);
		}
	}

	// Sorgulama
	let lookupValue = $state('');
	let lres = $state<{ value: string; matches: LookupMatch[] } | null>(null);

	async function lookup(ev: SubmitEvent) {
		ev.preventDefault();
		try {
			lres = await api.lookup(lookupValue);
		} catch (e) {
			notify.error(e);
		}
	}
</script>

<div class="page-head">
	<div>
		<h1>Doğrula & Sorgula</h1>
		<p>Bir değerin biçimini ve kontrol hanesini doğrulayın ya da hangi nesneye ait olduğunu bulun.</p>
	</div>
</div>

<div class="grid two">
	<section class="card">
		<h2>Sorgula: bu değer kimde?</h2>
		<form onsubmit={lookup} class="inline">
			<input class="mono" placeholder="ör. 8691234567890" bind:value={lookupValue} required />
			<button class="primary">Sorgula</button>
		</form>
		{#if lres}
			{#if lres.matches.length === 0}
				<div class="alert warn" style="margin-top: 12px">
					<span class="mono">{lres.value}</span> hiçbir havuzun aralığına düşmüyor (ya da kontrol hanesi hatalı).
				</div>
			{:else}
				{#each lres.matches as m (m.pool_id)}
					<div class="match">
						<div class="top">
							<span class="mono big">{m.value}</span>
							<StatusBadge status={m.state} />
						</div>
						<div class="small">
							Havuz: <a href="/pools/{m.pool_id}">{m.pool_name}</a>
							{#if m.allocation}
								· <a href="/allocations/{m.allocation.id}">tahsis #{m.allocation.id}</a>
							{/if}
						</div>
						{#if m.allocation?.object_ref}
							<div class="small">
								{m.state === 'assigned' || m.state === 'reserved' ? 'Sahibi' : 'Son sahibi'}:
								<strong>{m.allocation.object_type}/{m.allocation.object_ref}</strong>
							</div>
						{:else if m.state === 'available'}
							<div class="small muted">Bu değer henüz kimseye verilmemiş.</div>
						{/if}
					</div>
				{/each}
			{/if}
		{/if}
	</section>

	<section class="card">
		<h2>Doğrula: biçim ve kontrol hanesi</h2>
		<form onsubmit={validate}>
			<div class="row">
				<div class="field" style="flex: 0 1 200px">
					<label for="vk">Tip</label>
					<select id="vk" bind:value={selected} onchange={() => (vres = null)}>
						{#each data.kinds as k (kindKey(k))}
							<option value={kindKey(k)}>{k.name}</option>
						{/each}
					</select>
				</div>
				<div class="field">
					<label for="vv">Değer</label>
					<input id="vv" class="mono" bind:value={value} placeholder={info.example} required />
				</div>
			</div>
			{#if kind === 'sequence'}
				<div class="row">
					<div class="field">
						<label for="vp">Önek</label>
						<input id="vp" class="mono" bind:value={prefix} />
					</div>
					<div class="field">
						<label for="vpad">Dolgu</label>
						<input id="vpad" type="number" min="0" max="18" bind:value={pad} />
					</div>
				</div>
			{/if}
			<button class="primary">Doğrula</button>
		</form>
		{#if vres}
			{#if vres.valid}
				<div class="ok">
					✓ Geçerli: <span class="mono">{vres.normalized}</span>
					<div class="small muted">Gövde: <span class="mono">{vres.base}</span></div>
					{#if vres.details}<div class="small">{vres.details}</div>{/if}
				</div>
			{:else}
				<div class="alert error" style="margin-top: 12px">✗ {vres.error}</div>
			{/if}
		{/if}
		<p class="small muted" style="margin-top: 14px">{info.description}</p>
	</section>
</div>

<style>
	.inline {
		display: flex;
		gap: 8px;
	}
	.match {
		margin-top: 12px;
		padding: 12px;
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.match .top {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 4px;
	}
	.big {
		font-size: 18px;
		font-weight: 650;
	}
	.ok {
		margin-top: 12px;
		padding: 10px 14px;
		border-radius: var(--radius);
		background: var(--ok-soft);
		color: var(--ok);
	}
</style>
