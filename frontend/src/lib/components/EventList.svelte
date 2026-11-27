<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { actionLabel, dateTime, relative } from '../format';
	import type { AllocationEvent } from '../types';

	let {
		events,
		showPool = false,
		showValue = true
	}: { events: AllocationEvent[]; showPool?: boolean; showValue?: boolean } = $props();

	const tone: Record<string, string> = {
		assign: 'var(--ok)',
		confirm: 'var(--ok)',
		reserve: 'var(--warn)',
		renew: 'var(--warn)',
		release: 'var(--gray)',
		expire: 'var(--gray)',
		block: 'var(--danger)',
		unblock: 'var(--accent)'
	};
</script>

{#if events.length === 0}
	<div class="empty">Henüz hareket yok.</div>
{:else}
	<ol>
		{#each events as e (e.id)}
			<li>
				<span class="dot" style:background={tone[e.action] ?? 'var(--gray)'}></span>
				<div class="body">
					<div>
						<strong>{actionLabel[e.action] ?? e.action}</strong>
						{#if showValue}
							<a class="mono" href="/allocations/{e.allocation_id}">{e.value}</a>
						{/if}
						{#if e.object_ref}
							<span class="muted">→ {e.object_type}/{e.object_ref}</span>
						{/if}
					</div>
					<div class="small muted">
						<span title={dateTime(e.created_at)}>{relative(e.created_at)}</span>
						· {e.actor || '—'}
						{#if showPool}· <a href="/pools/{e.pool_id}">{e.pool_code}</a>{/if}
						{#if e.note}· “{e.note}”{/if}
					</div>
				</div>
			</li>
		{/each}
	</ol>
{/if}

<style>
	ol {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		gap: 10px;
		padding: 7px 0;
		border-bottom: 1px solid var(--border);
	}
	li:last-child {
		border-bottom: 0;
	}
	.dot {
		margin-top: 7px;
	}
	.body {
		min-width: 0;
		overflow-wrap: anywhere;
	}
</style>
