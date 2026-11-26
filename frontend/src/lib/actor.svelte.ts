// İşlemi yapan kişinin adı; denetim kaydına X-Actor başlığıyla yazılır.
// Kimlik doğrulama eklendiğinde bu alan oturumdan doldurulmalı.
const KEY = 'kaynak.actor';

export const actor = $state({ name: '' });

if (typeof localStorage !== 'undefined') {
	actor.name = localStorage.getItem(KEY) ?? '';
}

export function saveActor(name: string) {
	actor.name = name.trim();
	localStorage.setItem(KEY, actor.name);
}
