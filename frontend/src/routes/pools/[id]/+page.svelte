<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import AllocatePanel from '#lib/components/AllocatePanel.svelte';
	import AllocationsTable from '#lib/components/AllocationsTable.svelte';
	import EventList from '#lib/components/EventList.svelte';
	import PoolSettings from '#lib/components/PoolSettings.svelte';
	import RangesPanel from '#lib/components/RangesPanel.svelte';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import { approx, duration, kindLabel, num, pct, policyLabel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let p = $derived(data.pool);
	let s = $derived(p.stats);

	// Kurallı tiplerde büyük aralıkların kapasitesi örneklem tahminidir.
	let cards = $derived([
		{ label: 'Kapasite', n: s.capacity, color: '', exact: s.capacity_exact },
		{ label: 'Atanmış', n: s.assigned, color: 'var(--ok)', exact: true },
		{ label: 'Rezerve', n: s.reserved, color: 'var(--warn)', exact: true },
		{ label: 'Karantinada', n: s.released, color: 'var(--gray)', exact: true },
		{ label: 'Bloklu', n: s.blocked, color: 'var(--danger)', exact: true },
		{ label: 'Boşta', n: s.available, color: 'var(--free)', exact: s.capacity_exact }
	]);
</script>

<div class="page-head">
	<div>
		<div class="crumbs small"><a href="/pools">Havuzlar</a> /</div>
		<h1>
			{p.name}
			<span class="tag accent">{kindLabel(p)}</span>
			{#if !p.active}<span class="tag">pasif</span>{/if}
		</h1>
		<p>
			<span class="mono">{p.code}</span>
			{#if p.prefix}· önek <span class="mono">{p.prefix}</span>{/if}
			· {policyLabel[p.reuse_policy]}{p.reuse_policy === 'cooldown' ? ` (${p.cooldown_days} gün)` : ''}
			· rezervasyon {duration(p.reservation_ttl_seconds)}
			{#if p.description}<br />{p.description}{/if}
			{#if p.kind === 'custom'}<br /><span class="small">Kod kuralları: {data.kind.description}</span>{/if}
		</p>
	</div>
</div>

<div class="stats">
	{#each cards as c (c.label)}
		<div class="stat">
			<div class="label">
				{#if c.color}<span class="dot" style:background={c.color}></span>{/if}{c.label}
			</div>
			<div class="value">{approx(c.n, c.exact)}</div>
		</div>
	{/each}
</div>
<div class="usage">
	<UsageBar stats={s} height={12} />
	<div class="small muted">
		{pct(s.capacity - s.available, s.capacity)} kullanımda
		{#if s.reusable}· {num(s.reusable)} bırakılmış değer tekrar dağıtılabilir{/if}
		{#if p.low_watermark}· uyarı eşiği {num(p.low_watermark)}{/if}
	</div>
</div>

{#if s.low}
	<div class="alert warn" style="margin-bottom: 16px">
		Boşta {num(s.available)} değer kaldı (eşik {num(p.low_watermark)}). Yeni aralık eklemeyi planlayın.
	</div>
{/if}

{#key p.id}
	<div class="grid two">
		<section class="card"><AllocatePanel pool={p} /></section>
		<section class="card">
			<RangesPanel pool={p} kind={data.kind} />
		</section>
	</div>

	<section class="card" style="margin-top: 16px">
		<h2>Tahsisler</h2>
		<AllocationsTable data={data.allocations} filter={data.filter} />
	</section>

	<div class="grid two" style="margin-top: 16px">
		<section class="card">
			<h2>Son hareketler</h2>
			<EventList events={data.events} />
		</section>
		<section class="card">
			<details>
				<summary>Havuz ayarları</summary>
				<div style="margin-top: 12px"><PoolSettings pool={p} /></div>
			</details>
		</section>
	</div>
{/key}

<style>
	h1 {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.crumbs {
		margin-bottom: 2px;
	}
	.usage {
		margin: 12px 0 20px;
	}
	.usage .small {
		margin-top: 4px;
	}
</style>
