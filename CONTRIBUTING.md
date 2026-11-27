# Tükenen Kaynak Takibi'ne katkı

Katkın için teşekkürler. Tükenen Kaynak Takibi bir abarow projesidir; tasarım kararları ve kullanım [README.md](README.md) dosyasında.

## Lisans

Proje [Apache-2.0](LICENSE) ile lisanslıdır. Gönderdiğin katkılar da aynı lisansla projeye dahil olur (Apache-2.0 md. 5).

## Commit imzası (DCO)

Her commit bir `Signed-off-by` satırı taşır. Bu satırla katkıyı gönderme hakkın olduğunu beyan edersin ([Developer Certificate of Origin](https://developercertificate.org)).

```sh
git commit -s -m "store: toplu tahsiste kilit sırası düzeltildi"
```

## Adını ekleme

- **`AUTHORS`:** Katkı yapan herkes adını kendi PR'ında [AUTHORS](AUTHORS) dosyasına ekleyebilir.
- **Dosya başlığı:** Bir dosyada kayda değer değişiklik yaptıysan başlığa kendi telif satırını da ekleyebilirsin.

## Dosya başlıkları

Her kaynak dosya şu başlıkla başlar. Yıl, dosyanın oluşturulduğu yıldır:

```go
// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0
```

Kendi satırını eklerken:

```go
// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// Copyright 2027 Ayşe Yılmaz
// SPDX-License-Identifier: Apache-2.0
```

- Kurucu satırı silinmez. Başlığı olmayan ya da kurucu satırı silinmiş dosyaları `scripts/check-headers.sh` bulur.
- Yorum biçimi dosya türüne göre değişir: SQL'de `--`, Svelte ve HTML'de `<!-- -->`, CSS'te `/* */`, YAML ve Dockerfile'da `#`. HTML'de başlık `<html>` etiketinin altına girer, `<!doctype html>` ilk satırda kalır.

## Göndermeden önce

```sh
cd backend && go vet ./... && go test ./...   # veritabanı testleri için README'deki "Testler" bölümü
cd frontend && npm run check                  # tip ve Svelte denetimi
bash scripts/check-headers.sh                 # telif başlıkları
node scripts/third-party-notices.mjs --check  # bağımlılık lisansları ve THIRD_PARTY_NOTICES güncel mi
```

## Bağımlılık eklerken

Yeni bir Go modülü ya da arayüze giren bir npm paketi eklemeden önce lisansına bak.

- **Karar gerektirmeyenler:** Apache-2.0, MIT, BSD-2-Clause, BSD-3-Clause ve ISC.
- **Karar gerekenler:** GPL, LGPL, AGPL, MPL ya da belirsiz lisans. Bunları proje sahibine sor.
- Ekledikten sonra `node scripts/third-party-notices.mjs` ile THIRD_PARTY_NOTICES dosyasını yenile. Kodu arayüze giren yeni bir npm paketi eklediysen betikteki `UI_PACKAGES` listesine de ekle.
