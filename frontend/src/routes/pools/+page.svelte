<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { page } from '$app/state';
	import UsageBar from '#lib/components/UsageBar.svelte';
	import CreatePool from '#lib/components/CreatePool.svelte';
	import { approx, kindLabel, num, pct, policyLabel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let showForm = $state(page.url.searchParams.has('new'));
</script>

<div class="page-head">
	<div>
		<h1>Havuzlar</h1>
		<p>Her havuz bir kaynak tipinin bir veya daha fazla sayısal aralığını yönetir.</p>
	</div>
	{#if !showForm}
		<button class="primary" onclick={() => (showForm = true)}>+ Yeni havuz</button>
	{/if}
</div>

{#if showForm}
	<div class="card" style="margin-bottom: 16px">
		<CreatePool kinds={data.kinds} initialKind={page.url.searchParams.get('kind')} oncancel={() => (showForm = false)} />
	</div>
{/if}

<div class="card">
	{#if data.pools.length === 0}
		<div class="empty">Henüz havuz yok.</div>
	{:else}
		<div class="table-wrap">
			<table>
				<thead>
					<tr>
						<th>Havuz</th>
						<th>Tip</th>
						<th>Politika</th>
						<th style="width: 28%">Kullanım</th>
						<th class="num">Atanmış</th>
						<th class="num">Rezerve</th>
						<th class="num">Boşta</th>
						<th class="num">Kapasite</th>
					</tr>
				</thead>
				<tbody>
					{#each data.pools as p (p.id)}
						<tr class:inactive={!p.active}>
							<td>
								<a href="/pools/{p.id}"><strong>{p.name}</strong></a>
								<div class="small muted mono">{p.code}</div>
							</td>
							<td>
								<span class="tag accent">{kindLabel(p)}</span>
								{#if p.prefix}<span class="small mono muted">{p.prefix}</span>{/if}
							</td>
							<td class="small">
								{policyLabel[p.reuse_policy]}{p.reuse_policy === 'cooldown' ? ` (${p.cooldown_days} gün)` : ''}
							</td>
							<td>
								<UsageBar stats={p.stats} height={8} />
								<div class="small muted" style="margin-top: 2px">
									{pct(p.stats.capacity - p.stats.available, p.stats.capacity)} dolu
									{#if !p.active}· <span class="tag">pasif</span>{/if}
									{#if p.stats.low}· <span class="tag warn">azalıyor</span>{/if}
								</div>
							</td>
							<td class="num">{num(p.stats.assigned)}</td>
							<td class="num">{num(p.stats.reserved)}</td>
							<td class="num"><strong>{approx(p.stats.available, p.stats.capacity_exact)}</strong></td>
							<td class="num muted">{approx(p.stats.capacity, p.stats.capacity_exact)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	tr.inactive td {
		opacity: 0.6;
	}
</style>
