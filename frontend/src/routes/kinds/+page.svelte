<script lang="ts">
	import { page } from '$app/state';
	import { invalidate } from '$app/navigation';
	import { api } from '#lib/api.ts';
	import CustomKindForm from '#lib/components/CustomKindForm.svelte';
	import { pct } from '#lib/format.ts';
	import { notify } from '#lib/toast.svelte.ts';
	import type { CustomKind } from '#lib/types.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let showForm = $state(page.url.searchParams.has('new'));

	async function remove(k: CustomKind) {
		if (!confirm(`“${k.name}” kod tipi silinsin mi?`)) return;
		try {
			await api.deleteCustomKind(k.id);
			notify.ok('Kod tipi silindi');
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		}
	}
</script>

<div class="page-head">
	<div>
		<h1>Kod tipleri</h1>
		<p>Hazır standartlar ve kendi algoritmanızla tanımladığınız kod tipleri. Havuzlar bu tiplerden oluşturulur.</p>
	</div>
	{#if !showForm}
		<button class="primary" onclick={() => (showForm = true)}>+ Yeni kod tipi</button>
	{/if}
</div>

{#if showForm}
	<div class="card" style="margin-bottom: 16px">
		<CustomKindForm onclose={() => (showForm = false)} />
	</div>
{/if}

<section class="card">
	<h2>Özel kod tipleri</h2>
	{#if data.custom.length === 0}
		<div class="empty">
			Henüz özel tip yok. Kendi kurallarınızla bir ürün kimliği tanımlamak için
			<button class="link" onclick={() => (showForm = true)}>yeni kod tipi</button> oluşturun.
		</div>
	{:else}
		<div class="table-wrap">
			<table>
				<thead>
					<tr>
						<th>Tip</th>
						<th>Yapı</th>
						<th>Kurallar</th>
						<th class="num">Geçerli oran</th>
						<th>Örnek</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each data.custom as k (k.id)}
						<tr>
							<td>
								<strong>{k.name}</strong>
								<div class="small muted mono">{k.code}</div>
								{#if k.description}<div class="small muted">{k.description}</div>{/if}
							</td>
							<td class="small">{k.body_digits} gövde + {k.check_digits} kontrol</td>
							<td class="small">
								<ol class="rules">
									{#each k.rules as r, i (i)}
										<li><span class="mono">{r.expr}</span>{#if r.message}<div class="muted">{r.message}</div>{/if}</li>
									{/each}
									{#if k.check_digits > 0}
										<li class="check">
											<span class="muted">kontrol =</span> <span class="mono">{k.check_expr}</span>
										</li>
									{/if}
								</ol>
							</td>
							<td class="num small">{pct(k.density, 1)}</td>
							<td class="mono">
								{k.example.slice(0, k.body_digits)}<em>{k.example.slice(k.body_digits)}</em>
							</td>
							<td class="actions">
								<a class="btn small" href="/pools?new=1&kind=custom:{k.code}">Havuz oluştur</a>
								{#if k.pools === 0}
									<button class="small danger" onclick={() => remove(k)}>Sil</button>
								{:else}
									<div class="small muted">{k.pools} havuzda</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>

<section class="card">
	<h2>Hazır tipler</h2>
	<div class="table-wrap">
		<table>
			<thead>
				<tr>
					<th>Tip</th>
					<th>Uzunluk</th>
					<th>Algoritma</th>
					<th>Örnek</th>
				</tr>
			</thead>
			<tbody>
				{#each data.builtins as k (k.code)}
					<tr>
						<td><strong>{k.name}</strong></td>
						<td class="small">
							{k.base_digits
								? `${k.base_digits}${k.radix === 16 ? ' onaltılık' : ''} hane` +
									(k.check_digits ? ` + ${k.check_digits} kontrol` : '')
								: 'serbest'}
						</td>
						<td class="small">{k.description}</td>
						<td class="mono">{k.example}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</section>

<style>
	.rules {
		margin: 0;
		padding-left: 18px;
	}
	.rules li + li {
		margin-top: 4px;
	}
	.rules .check {
		list-style: none;
		margin-left: -18px;
	}
	td em {
		font-style: normal;
		color: var(--accent);
		font-weight: 600;
	}
	.btn.small {
		padding: 3px 9px;
		font-size: 12px;
		text-decoration: none;
	}
</style>
