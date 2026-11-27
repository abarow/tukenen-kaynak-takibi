<!-- Copyright 2026 Mehmet ERGULCU and the abarow contributors -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<script lang="ts">
	import { untrack } from 'svelte';
	import { goto, invalidate } from '$app/navigation';
	import { api } from '../api';
	import { notify } from '../toast.svelte';
	import { policyLabel } from '../format';
	import type { Pool, ReusePolicy } from '../types';

	let { pool }: { pool: Pool } = $props();

	// Form taslağı: kayıtlı değerlerle bir kez başlatılır, kullanıcı düzenledikçe değişir.
	const init = untrack(() => ({ ...pool }));
	let name = $state(init.name);
	let description = $state(init.description);
	let policy = $state<ReusePolicy>(init.reuse_policy);
	let cooldownDays = $state(init.cooldown_days || 1460);
	let ttlMinutes = $state(init.reservation_ttl_seconds / 60);
	let lowWatermark = $state(init.low_watermark);
	let active = $state(init.active);
	let busy = $state(false);

	async function save(ev: SubmitEvent) {
		ev.preventDefault();
		busy = true;
		try {
			await api.updatePool(pool.id, {
				name,
				description,
				reuse_policy: policy,
				cooldown_days: policy === 'cooldown' ? cooldownDays : 0,
				reservation_ttl_seconds: Math.round(ttlMinutes * 60),
				low_watermark: lowWatermark,
				active
			});
			notify.ok('Ayarlar kaydedildi');
			await invalidate('app:data');
		} catch (e) {
			notify.error(e);
		} finally {
			busy = false;
		}
	}

	async function remove() {
		if (!confirm(`“${pool.name}” havuzu silinsin mi? Bu işlem geri alınamaz.`)) return;
		try {
			await api.deletePool(pool.id);
			notify.ok('Havuz silindi');
			await goto('/pools');
		} catch (e) {
			notify.error(e);
		}
	}
</script>

<form onsubmit={save}>
	<div class="row">
		<div class="field">
			<label for="s-name">Ad</label>
			<input id="s-name" bind:value={name} required />
		</div>
		<div class="field">
			<label for="s-desc">Açıklama</label>
			<input id="s-desc" bind:value={description} />
		</div>
	</div>
	<div class="row">
		<div class="field">
			<label for="s-policy">Yeniden kullanım</label>
			<select id="s-policy" bind:value={policy}>
				{#each Object.entries(policyLabel) as [k, v] (k)}
					<option value={k}>{v}</option>
				{/each}
			</select>
		</div>
		{#if policy === 'cooldown'}
			<div class="field">
				<label for="s-cd">Karantina (gün)</label>
				<input id="s-cd" type="number" min="1" bind:value={cooldownDays} />
			</div>
		{/if}
		<div class="field">
			<label for="s-ttl">Rezervasyon (dk)</label>
			<input id="s-ttl" type="number" min="1" bind:value={ttlMinutes} />
		</div>
		<div class="field">
			<label for="s-low">Uyarı eşiği</label>
			<input id="s-low" type="number" min="0" bind:value={lowWatermark} />
		</div>
	</div>
	<div class="hint small muted" style="margin: -4px 0 12px">
		Politika değişikliği yalnızca bundan sonra bırakılan değerleri etkiler.
	</div>
	<label class="check"><input type="checkbox" bind:checked={active} /> Havuz aktif (tahsis yapılabilir)</label>
	<div class="row" style="margin-top: 12px">
		<button class="primary" disabled={busy}>Kaydet</button>
		<span class="spacer"></span>
		<button type="button" class="danger" onclick={remove}>Havuzu sil</button>
	</div>
</form>

<style>
	.check {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 14px;
		font-weight: 500;
		color: var(--text);
	}
</style>
