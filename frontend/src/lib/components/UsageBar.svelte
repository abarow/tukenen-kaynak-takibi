<script lang="ts">
	import { num, pct } from '../format';
	import type { PoolStats } from '../types';

	let { stats, height = 10 }: { stats: PoolStats; height?: number } = $props();

	// Kullanılmış kısım görünür olsun diye çok küçük dilimlere en az 0,5% genişlik ver.
	const w = (n: number) => (n === 0 || !stats.capacity ? 0 : Math.max(0.5, (n / stats.capacity) * 100));

	let segments = $derived([
		{ key: 'assigned', label: 'Atanmış', n: stats.assigned, color: 'var(--ok)' },
		{ key: 'reserved', label: 'Rezerve', n: stats.reserved, color: 'var(--warn)' },
		{ key: 'released', label: 'Bırakılmış (karantina)', n: stats.released, color: 'var(--gray)' },
		{ key: 'blocked', label: 'Bloklu', n: stats.blocked, color: 'var(--danger)' }
	]);
	let used = $derived(stats.capacity - stats.available);
</script>

<div
	class="bar"
	style:height="{height}px"
	role="img"
	aria-label="Kullanım {pct(used, stats.capacity)}: {num(used)} / {num(stats.capacity)}"
>
	{#each segments as s (s.key)}
		{#if s.n > 0}
			<span style:width="{w(s.n)}%" style:background={s.color} title="{s.label}: {num(s.n)}"></span>
		{/if}
	{/each}
</div>

<style>
	.bar {
		display: flex;
		width: 100%;
		background: var(--free);
		border-radius: 999px;
		overflow: hidden;
	}
	span {
		height: 100%;
	}
	span + span {
		box-shadow: -1px 0 0 var(--surface);
	}
</style>
