<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import AllocationsTable from '#lib/components/AllocationsTable.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<div class="page-head">
	<div>
		<h1>Tahsisler</h1>
		<p>Tüm havuzlarda rezerve edilmiş, atanmış, bırakılmış ve bloklanmış değerler</p>
	</div>
</div>

<div class="card">
	<AllocationsTable data={data.allocations} filter={data.filter} pools={data.pools} limit={50} />
</div>
