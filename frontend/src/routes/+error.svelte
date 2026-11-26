<script lang="ts">
	import { page } from '$app/state';
</script>

<div class="card">
	<h1>{page.status === 404 ? 'Bulunamadı' : 'Bir şeyler ters gitti'}</h1>
	<p class="muted">{page.error?.message}</p>
	<a href="/">Panoya dön</a>
</div>
