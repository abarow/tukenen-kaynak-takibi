<script lang="ts">
	import UsageBar from '#lib/components/UsageBar.svelte';
	import EventList from '#lib/components/EventList.svelte';
	import { kindLabel, num, pct } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let active = $derived(data.pools.filter((p) => p.active));
	let low = $derived(active.filter((p) => p.stats.low || (p.stats.capacity > 0 && p.stats.available === 0)));
	let totals = $derived(
		active.reduce(
			(t, p) => ({
				assigned: t.assigned + p.stats.assigned,
				reserved: t.reserved + p.stats.reserved,
				available: t.available + p.stats.available
			}),
			{ assigned: 0, reserved: 0, available: 0 }
		)
	);
</script>

<div class="page-head">
	<div>
		<h1>Pano</h1>
		<p>Kısıtlı kaynak havuzlarının doluluk durumu ve son hareketler</p>
	</div>
	<a class="btn" href="/pools?new=1">+ Yeni havuz</a>
</div>

<div class="stats" style="margin-bottom: 16px">
	<div class="stat">
		<div class="label">Aktif havuz</div>
		<div class="value">{num(active.length)}</div>
	</div>
	<div class="stat">
		<div class="label"><span class="dot" style:background="var(--ok)"></span>Atanmış değer</div>
		<div class="value">{num(totals.assigned)}</div>
	</div>
	<div class="stat">
		<div class="label"><span class="dot" style:background="var(--warn)"></span>Rezerve</div>
		<div class="value">{num(totals.reserved)}</div>
	</div>
	<div class="stat">
		<div class="label"><span class="dot" style:background="var(--free)"></span>Boşta</div>
		<div class="value">{num(totals.available)}</div>
	</div>
</div>

{#if low.length}
	<div class="alert warn" style="margin-bottom: 16px">
		<strong>Azalan kaynak:</strong>
		{#each low as p, i (p.id)}
			{i ? ', ' : ''}<a href="/pools/{p.id}">{p.name}</a> ({num(p.stats.available)} boşta)
		{/each}
		— yeni aralık eklemeyi planlayın.
	</div>
{/if}

<div class="grid two">
	<section class="card">
		<h2>Havuzlar</h2>
		{#if data.pools.length === 0}
			<div class="empty">
				Henüz havuz yok. <a href="/pools?new=1">İlk havuzu oluşturun</a>: bir barkod aralığı, ISBN bloğu
				ya da sıra numarası serisi.
			</div>
		{:else}
			<div class="pools">
				{#each data.pools as p (p.id)}
					<a class="pool" href="/pools/{p.id}" class:inactive={!p.active}>
						<div class="top">
							<span class="name">{p.name}</span>
							<span class="tag accent">{kindLabel(p)}</span>
							{#if !p.active}<span class="tag">pasif</span>{/if}
							{#if p.stats.low}<span class="tag warn">azalıyor</span>{/if}
						</div>
						<UsageBar stats={p.stats} height={8} />
						<div class="meta small muted">
							<span>{num(p.stats.capacity - p.stats.available)} / {num(p.stats.capacity)} kullanımda</span>
							<span>{pct(p.stats.capacity - p.stats.available, p.stats.capacity)}</span>
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</section>

	<section class="card">
		<h2>Son hareketler</h2>
		<EventList events={data.events} showPool />
	</section>
</div>

<style>
	.pools {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.pool {
		display: block;
		padding: 10px;
		margin: 0 -10px;
		border-radius: 6px;
		color: inherit;
		text-decoration: none;
	}
	.pool:hover {
		background: var(--surface-2);
	}
	.pool.inactive {
		opacity: 0.6;
	}
	.top {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-bottom: 6px;
	}
	.name {
		font-weight: 600;
	}
	.meta {
		display: flex;
		justify-content: space-between;
		margin-top: 4px;
	}
</style>
