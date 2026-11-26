<script lang="ts">
	import '../app.css';
	import favicon from '#lib/assets/favicon.svg';
	import { page } from '$app/state';
	import { actor, saveActor } from '#lib/actor.svelte.ts';
	import { toasts, dismiss } from '#lib/toast.svelte.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const nav = [
		{ href: '/', label: 'Pano', icon: 'M3 3h7v9H3zM14 3h7v5h-7zM14 12h7v9h-7zM3 16h7v5H3z' },
		{ href: '/pools', label: 'Havuzlar', icon: 'M4 6h16M4 12h16M4 18h10' },
		{ href: '/allocations', label: 'Tahsisler', icon: 'M9 7h11M9 12h11M9 17h11M4 7h.01M4 12h.01M4 17h.01' },
		{ href: '/kinds', label: 'Kod tipleri', icon: 'M8 6l-5 6 5 6M16 6l5 6-5 6M13 4l-2 16' },
		{ href: '/tools', label: 'Doğrula & Sorgula', icon: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM21 21l-4.3-4.3' }
	];

	const active = (href: string) =>
		href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);

	let actorDraft = $state(actor.name);
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Tükenen Kaynak Takibi</title>
</svelte:head>

<div class="shell">
	<aside>
		<a class="brand" href="/">
			<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
				<rect x="2" y="5" width="2" height="14" fill="currentColor" />
				<rect x="6" y="5" width="1" height="14" fill="currentColor" />
				<rect x="9" y="5" width="3" height="14" fill="currentColor" />
				<rect x="14" y="5" width="1" height="14" fill="currentColor" />
				<rect x="17" y="5" width="2" height="14" fill="currentColor" />
				<rect x="21" y="5" width="1" height="14" fill="currentColor" />
			</svg>
			<span>Tükenen Kaynak Takibi</span>
		</a>
		<nav>
			{#each nav as item (item.href)}
				<a href={item.href} class:on={active(item.href)}>
					<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
						<path d={item.icon} fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
					</svg>
					{item.label}
				</a>
			{/each}
		</nav>
		<div class="actor">
			<label for="actor">İşlemi yapan</label>
			<input
				id="actor"
				placeholder="adınız"
				bind:value={actorDraft}
				onchange={() => saveActor(actorDraft)}
			/>
		</div>
	</aside>

	<main>
		{@render children()}
	</main>
</div>

<div class="toasts" aria-live="polite">
	{#each toasts as t (t.id)}
		<button class="toast {t.kind}" onclick={() => dismiss(t.id)}>{t.text}</button>
	{/each}
</div>

<style>
	.shell {
		display: grid;
		grid-template-columns: 220px 1fr;
		min-height: 100vh;
	}
	aside {
		background: var(--surface);
		border-right: 1px solid var(--border);
		padding: 18px 12px;
		display: flex;
		flex-direction: column;
		gap: 18px;
		position: sticky;
		top: 0;
		height: 100vh;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-weight: 700;
		font-size: 16px;
		line-height: 1.2;
		text-wrap: balance;
		color: var(--text);
		padding: 0 8px;
		text-decoration: none;
	}
	.brand svg {
		flex: none;
	}
	nav {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	nav a {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 7px 10px;
		border-radius: 6px;
		color: var(--muted);
		font-weight: 550;
		text-decoration: none;
	}
	nav a:hover {
		background: var(--surface-2);
		color: var(--text);
	}
	nav a.on {
		background: var(--accent-soft);
		color: var(--accent);
	}
	.actor {
		margin-top: auto;
		padding: 0 6px;
	}
	main {
		padding: 28px 32px 60px;
		max-width: 1280px;
		width: 100%;
	}
	.toasts {
		position: fixed;
		right: 20px;
		bottom: 20px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		z-index: 50;
		max-width: 420px;
	}
	.toast {
		text-align: left;
		white-space: normal;
		box-shadow: 0 6px 20px rgb(0 0 0 / 0.15);
		border: 1px solid var(--border);
		padding: 10px 14px;
	}
	.toast.ok {
		border-left: 4px solid var(--ok);
	}
	.toast.error {
		border-left: 4px solid var(--danger);
		color: var(--danger);
	}
	@media (max-width: 820px) {
		.shell {
			grid-template-columns: 1fr;
		}
		aside {
			position: static;
			height: auto;
			flex-direction: row;
			flex-wrap: wrap;
			align-items: center;
		}
		nav {
			flex-direction: row;
			flex-wrap: wrap;
		}
		.actor {
			margin: 0;
		}
		main {
			padding: 20px 16px 60px;
		}
	}
</style>
