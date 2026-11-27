<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { statusLabel } from '../format';
	import type { Status } from '../types';

	let { status }: { status: Status | 'available' | 'reusable' } = $props();
</script>

<span class="badge {status}">{statusLabel[status] ?? status}</span>

<style>
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		font-size: 12px;
		font-weight: 600;
		padding: 1px 8px 1px 7px;
		border-radius: 999px;
		white-space: nowrap;
	}
	.badge::before {
		content: '';
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
	}
	.assigned {
		background: var(--ok-soft);
		color: var(--ok);
	}
	.reserved {
		background: var(--warn-soft);
		color: var(--warn);
	}
	.released {
		background: var(--gray-soft);
		color: var(--gray);
	}
	.blocked {
		background: var(--danger-soft);
		color: var(--danger);
	}
	.available,
	.reusable {
		background: var(--accent-soft);
		color: var(--accent);
	}
</style>
