# Kaynak — Kısıtlı Kaynak Yönetimi

Barkod numarası aralığı, ISBN bloğu, MAC adresi bloğu, fatura/seri numarası gibi **sayısı sınırlı ve
tekrar kullanılması riskli** kaynakları nesnelerle (ürün, kitap, cihaz, sanal makine, belge…) eşleştiren uygulama.

- **Backend:** Go (stdlib `net/http` + `pgx`), tek ikili dosya, arayüz içine gömülü
- **Frontend:** SvelteKit 3 / Svelte 5 (SPA, `adapter-static`)
- **Veritabanı:** PostgreSQL 18

## Kavramlar

| Kavram | Açıklama |
|---|---|
| **Havuz** | Bir kaynak tipinden yönetilen değer kümesi. Ör. "Ana ürün barkodları (EAN-13)". |
| **Aralık** | Havuza ait sayısal blok. Önekten (`8691234` → 100.000 barkod) ya da başlangıç–bitişten tanımlanır. Bir havuzun birden çok aralığı olabilir. |
| **Tahsis** | Bir değerin bir nesneye (`object_type` + `object_ref`) bağlanması. |
| **Olay** | Her durum değişikliğinin değiştirilemez denetim kaydı (kim, ne zaman, hangi nesne). |

### Durumlar

```mermaid
stateDiagram-v2
    [*] --> reserved: tahsis (mode=reserve)
    [*] --> assigned: tahsis (mode=assign)
    [*] --> blocked: blokla
    reserved --> assigned: onayla / aynı nesne için assign
    reserved --> released: bırak / süre doldu
    assigned --> released: bırak
    blocked --> released: bloğu kaldır
    released --> reserved: yeniden kullanılabilirse
    released --> assigned: yeniden kullanılabilirse
    released --> blocked: blokla
```

**Bırakılan değer ne zaman tekrar dağıtılır?** Havuzun politikasına bağlı:

| Politika | Atanmış değer bırakılınca | Uygun olduğu yer |
|---|---|---|
| `never` | Bir daha verilmez | ISBN, GS1 barkodları (GS1 GTIN'lerin yeniden kullanılmamasını öneriyor), cihaza basılan MAC adresleri |
| `cooldown` | N gün karantinadan sonra | Sanal makine MAC adresleri (ağ cihazlarının ARP/DHCP önbellekleri boşalsın diye), iç etiketler |
| `immediate` | Hemen | Geçici numaralar |

Hiç kullanılmamış değerler (süresi dolan rezervasyon, kaldırılan blok) politikadan bağımsız olarak hemen havuza döner.

## Kaynak tipleri

| Tip | Gövde | Kontrol hanesi | Not |
|---|---|---|---|
| `ean13` | 12 hane | GS1 mod-10 | Firma öneki + ürün numarası |
| `ean8` | 7 hane | GS1 mod-10 | |
| `isbn13` | 12 hane | GS1 mod-10 | 978/979 ile başlamalı. EAN-13 ile **aynı uzayı** paylaşır |
| `mac` | 12 **onaltılık** hane (48 bit) | yok; bit kuralları var | Fiziksel ağ adresi (EUI-48). Aşağıya bakın |
| `sequence` | serbest | yok | Önek + sıfır dolgu: `FTR-000123` |
| `custom` | siz belirlersiniz | sizin formülünüz | Kendi kurallarınızla tanımlanan tip (aşağıya bakın) |

Aralıklar her zaman **gövde** üzerinde tanımlanır; kontrol haneleri otomatik hesaplanır.
**GS1 mod-10** (EAN, ISBN-13): sağdan başlayarak haneler sırayla 3 ve 1 ile çarpılıp toplanır.

### MAC adresi (fiziksel ağ adresi)

Kontrol hanesi yoktur; geçerliliği ilk oktetteki iki bit ve IEEE blok yapısı belirler:

```
 02 : 4B : 59 : 00 : 00 : 01
 └┬┘
  0000 0010
          │└─ I/G biti: 0 = tekil (cihaza verilebilir), 1 = multicast/broadcast → REDDEDİLİR
          └── U/L biti: 0 = evrensel (IEEE'den alınan OUI), 1 = yerel yönetilen (VM, konteyner)
```

| IEEE bloğu | Önek | Adres sayısı |
|---|---|---|
| MA-L (OUI) | 6 onaltılık hane, ör. `00:00:5E` | 16.777.216 |
| MA-M | 7 hane, ör. `00:00:5E:0` | 1.048.576 |
| MA-S | 9 hane, ör. `00:00:5E:00:5` | 4.096 |

- Girdi yazımı serbest: `00:00:5E:00:53:01`, `00-00-5e-00-53-01` (Windows), `0000.5E00.5301` (Cisco)
  hepsi aynı adrese çözülür ve `00:00:5E:00:53:01` olarak saklanır. Sorgulama da her yazımla çalışır.
- Bir aralık tek bir ilk oktet içinde kalmalı. İki ardışık oktetten biri mutlaka tek sayı (multicast)
  olacağından, örneğin `02:…`–`03:…` aralığı reddedilir.
- Doğrulama aracı adresin OUI'sini ve evrensel/yerel olduğunu da gösterir.
- Örneklerde RFC 7042'nin dokümantasyona ayırdığı `00:00:5E:00:53:00–FF` bloğu kullanılıyor.

### Özel kod tipleri (kendi algoritmanız)

Kod yazmadan, arayüzdeki **Kod tipleri** sayfasından ya da API'den yeni bir tip tanımlanır:

- **Gövde uzunluğu** ve **kontrol kısmı uzunluğu**,
- gövdenin sağlaması gereken **kurallar**: her biri doğru/yanlış dönen bir formül ve kullanıcıya gösterilecek hata mesajı,
- kontrol kısmını hesaplayan **formül**: sonucun son *k* hanesi, sıfır dolgulu olarak kullanılır.

Örnek: 12 haneli ürün kimliği

| | Formül | Anlamı |
|---|---|---|
| Kural 1 | `digitSum(1, 3) % 2 == 0` | ilk 3 hanenin toplamı çift |
| Kural 2 | `digitSum(4, 8) % 3 == 0` | sonraki 5 hanenin toplamı 3'e bölünür |
| Kontrol (4 hane) | `sum(chunks(1, 8, 2))` | ilk 8 hane 2'şerli 4 sayı olarak toplanır, son 4 hanesi |

`12345678` → 1+2+3 = 6 çift ✓, 4+5+6+7+8 = 30 ✓, 12+34+56+78 = 180 → kod **`1234567` `8` `0180`**.

Formüller [Expr](https://expr-lang.org) ifade dilinde yazılır: tek bir ifade, JavaScript/Excel formülüne
benzer. Fonksiyon adları İngilizcedir ve Expr'in kendi fonksiyonlarıyla (`sum`, `filter`, `findIndex`…) aynı
camelCase düzenini izler. İlk sürümdeki Türkçe adlarla (`hane`, `toplam`, `sayi`, `gruplar`, `agirlikli`)
kaydedilmiş tanımlar [005 migration'ı](backend/internal/db/migrations/005_english_function_names.sql) ile
otomatik çevrilir; algoritma değişmez.

| Fonksiyon | Açıklama |
|---|---|
| `digit(i)` | i. hane (konumlar 1'den başlar, gövdeye göre) |
| `digitSum(a, b)` | a..b hanelerinin toplamı |
| `number(a, b)` | a..b hanelerinin oluşturduğu sayı |
| `chunks(a, b, n)` | a..b hanelerini n haneli sayılara böler: `chunks(1, 8, 2)` = `[12, 34, 56, 78]` |
| `weightedSum(a, b, [3, 1])` | ağırlıklı hane toplamı, ağırlıklar soldan döner. EAN-13 kontrolü: `(10 - weightedSum(1, 12, [1, 3]) % 10) % 10` |
| `sum`, `map`, `filter`, `all`, `any`, `count`, `len`, `abs`, `max`, `min` | liste işlemleri |

Nasıl çalışıyor:

- Formüller [expr](https://github.com/expr-lang/expr) ile derlenir. Dosya ve ağ erişimi yoktur, bellek ve
  ifade boyutu sınırlıdır. `now()` gibi her çalıştırmada farklı sonuç verebilecek fonksiyonlar kapalıdır.
- Kayıttan önce tanım 100.000 rastgele gövdeyle sınanır. Çalışırken hata veren (ör. `digit(9)` 8 haneli
  gövdede) ya da hiçbir değeri kabul etmeyen tanım reddedilir. Arayüz tanımı siz yazarken canlı dener ve
  örnek kodlarla geçerli oranı gösterir.
- Kurallar yüzünden aralıktaki her gövde geçerli olmaz. Bu örnekte yaklaşık %16,7'si geçerlidir.
  **Kapasite, geçerli kod sayısıdır**: aralık eklenirken 1 milyon gövdeye kadar tek tek sayılır, daha
  büyük aralıklarda 100.000 örnekle tahmin edilir (arayüzde `≈`). İmleç, kurallara uymayan gövdeleri
  atlar. Hiç geçerli değeri olmayan aralık reddedilir: örneğin `247` önekinde 2+4+7 tek sayı olduğu için
  hiçbir gövde kural 1'i sağlamaz.
- Tanım kaydedildikten sonra **değiştirilemez**, çünkü algoritma değişirse verilmiş kodlar geçersiz kalır.
  Kullanılmayan bir tip silinip yeniden tanımlanabilir.

Editörde iki hazır örnek var: **Ürün kimliği** (yukarıdaki) ve **ISBN-13**. ISBN-13 örneği, hazır tipin
formüllerle yazılmış hâlidir; testler iki yolun aynı kodları ürettiğini doğrular:

| | Formül |
|---|---|
| Gövde / kontrol | 12 hane / 1 hane |
| Kural | `number(1, 3) == 978 or number(1, 3) == 979` (ISBN 978 veya 979 ile başlar) |
| Kontrol | `(10 - weightedSum(1, 12, [1, 3]) % 10) % 10` (GS1 mod-10) |

Gerçek ISBN havuzları için **hazır ISBN-13 tipini** kullanın. Özel tipler kendi uzaylarında tutulduğu için
hazır ISBN ve EAN-13 havuzlarıyla çakışma denetimine girmez; aynı numara iki havuzdan verilebilir.

Yerleşik bir tip eklemek isterseniz [backend/internal/kinds/kinds.go](backend/internal/kinds/kinds.go)'ya kural
ekleyip `pools_kind_check` kısıtını yeni bir migration ile güncellemek yeterli
([003_mac.sql](backend/internal/db/migrations/003_mac.sql) bunun örneği).

## Tasarım kararları

**1. Değerler önceden üretilmez.** Her aralığın bir `next_value` imleci var. Yalnızca *dokunulmuş*
değerler (`allocations` tablosu) satır olarak tutulur. 16,7 milyonluk bir MAC OUI bloğu da tek satırla başlar.
Kapasite/boşta sayıları `aralıktaki geçerli değer − dağıtılamaz satırlar` olarak hesaplanır.

**2. Çakışan aralık veritabanı seviyesinde imkânsız.** `pool_ranges` üzerinde
`EXCLUDE USING gist (namespace WITH =, int8range(start, end) WITH &&)` kısıtı var. EAN-13 ve
ISBN-13 aynı `gtin13` uzayında olduğu için bir barkod havuzu bir ISBN bloğunu kapsayamaz.

**3. Aynı değer asla iki nesneye verilmez.** Tahsis sırası:
1. Aynı nesne için **advisory lock** alınır; nesnenin bu havuzda aktif değeri varsa o döner (*idempotent*: istemci ağ hatasında güvenle tekrar dener).
2. Yeniden kullanılabilir bırakılmış değer varsa `FOR UPDATE SKIP LOCKED` ile alınır.
3. Yoksa aralık satırı `FOR UPDATE` ile kilitlenip imleç bir ilerletilir. Elle alınmış/bloklu değerler `ON CONFLICT DO NOTHING` ile atlanır. Transaction geri alınırsa imleç de geri alınır, boşluk oluşmaz.

Bu davranış `go test` içinde 80 eşzamanlı istekle 30 kapasiteli havuza yüklenerek doğrulanıyor.

**4. Rezervasyon.** "Önce numarayı ayır, ürün kaydı tamamlanınca onayla" akışı için. Süresi dolan
rezervasyonları arka plan görevi (`SWEEP_INTERVAL`, varsayılan 30 sn) havuza geri bırakır; birden
çok sunucu örneğiyle güvenle çalışır.

**5. Toplu tahsis tek parça.** 1000 nesneye kadar tek istekte; havuz yetmezse hiçbir değer yazılmaz.

## Çalıştırma

### Geliştirme

```bash
docker compose up -d db                 # PostgreSQL :5433

cd backend
go run ./cmd/server                     # API :8088 (migration'lar otomatik uygulanır)

cd frontend
npm install
npm run dev                             # arayüz, /api istekleri :8088'e yönlendirilir
```

### Tek ikili dosya

```bash
cd frontend && npm run build            # çıktı backend/internal/web/dist'e yazılır
cd ../backend && go build -o bin/kaynak ./cmd/server
./bin/kaynak                            # http://localhost:8088
```

### Docker

```bash
docker compose --profile app up --build # db + uygulama, http://localhost:8088
```

### Ortam değişkenleri

| Değişken | Varsayılan |
|---|---|
| `DATABASE_URL` | `postgres://kaynak:kaynak@localhost:5433/kaynak?sslmode=disable` |
| `HTTP_ADDR` | `:8088` |
| `SWEEP_INTERVAL` | `30s` |

### Testler

```bash
cd backend
go test ./internal/kinds/                         # birim testleri (kontrol haneleri)
TEST_DATABASE_URL=postgres://kaynak:kaynak@localhost:5433/kaynak?sslmode=disable \
  go test ./...                                   # + PostgreSQL entegrasyon testleri
```

Entegrasyon testleri aynı sunucuda `kaynak_test` veritabanını sıfırdan kurar; ana veriye dokunmaz.

## API

Havuzlar sayısal kimlik **veya kod** ile adreslenebilir (`/api/pools/12` ≡ `/api/pools/urun-barkod`).
`X-Actor` başlığı denetim kaydına işlemi yapanı yazar. Hatalar `{"error": {"code", "message"}}` biçimindedir.

| Metot | Yol | Açıklama |
|---|---|---|
| `GET` | `/api/kinds` | Kaynak tipleri (hazır + özel) |
| `GET` / `POST` | `/api/custom-kinds` | Özel kod tiplerini listele / tanımla |
| `POST` | `/api/custom-kinds/test` | Tanımı kaydetmeden dene; `value` verilirse o kodu da doğrular |
| `DELETE` | `/api/custom-kinds/{id}` | Hiçbir havuzun kullanmadığı tipi sil |
| `GET` / `POST` | `/api/pools` | Havuzları listele / oluştur (ilk aralıklarla birlikte) |
| `GET` / `PATCH` / `DELETE` | `/api/pools/{pool}` | Detay+istatistik / ayar güncelle / sil (tahsis yoksa) |
| `POST` | `/api/pools/{pool}/ranges` | Aralık ekle: `{"prefix": "..."}` veya `{"start", "end"}` |
| `DELETE` | `/api/pools/{pool}/ranges/{id}` | Hiç kullanılmamış aralığı sil |
| `POST` | `/api/pools/{pool}/allocate` | **Tahsis** (201 yeni, 200 mevcut) |
| `POST` | `/api/pools/{pool}/allocate/bulk` | Toplu tahsis `{"items": [...]}` |
| `POST` | `/api/pools/{pool}/block` | Değeri dağıtım dışı bırak |
| `GET` | `/api/allocations` | Filtre: `pool_id`, `status`, `object_type`, `object_ref`, `q`, `limit`, `offset` |
| `GET` | `/api/allocations/{id}` | Detay + olay geçmişi |
| `POST` | `/api/allocations/{id}/confirm` | Rezervasyonu onayla |
| `POST` | `/api/allocations/{id}/release` | Bırak / bloğu kaldır |
| `GET` | `/api/lookup?value=` | Bu değer hangi havuzda, kimde? |
| `POST` | `/api/tools/validate` | Biçim ve kontrol hanesi doğrula |
| `GET` | `/api/events` | Son hareketler (`pool_id`, `limit`) |

Örnek: ERP'de yeni ürün açılırken barkod almak

```bash
curl -X POST http://localhost:8088/api/pools/urun-barkod/allocate \
  -H 'Content-Type: application/json' -H 'X-Actor: erp' \
  -d '{"object_type": "product", "object_ref": "SKU-10045", "metadata": {"kategori": "içecek"}}'
```

```json
{ "created": true,
  "allocation": { "id": 42, "value": "8690001000042", "status": "assigned",
                  "object_type": "product", "object_ref": "SKU-10045", ... } }
```

Aynı istek tekrar gönderilirse yeni barkod harcanmaz; aynı değer `"created": false` ile döner.

Örnek: özel tip tanımlayıp ondan havuz açmak

```bash
curl -X POST http://localhost:8088/api/custom-kinds -H 'Content-Type: application/json' -d '{
  "code": "urun-kimligi", "name": "Ürün kimliği", "body_digits": 8, "check_digits": 4,
  "rules": [
    {"expr": "digitSum(1, 3) % 2 == 0", "message": "ilk 3 hanenin toplamı çift olmalı"},
    {"expr": "digitSum(4, 8) % 3 == 0", "message": "sonraki 5 hanenin toplamı 3e bölünmeli"}],
  "check_expr": "sum(chunks(1, 8, 2))"}'

curl -X POST http://localhost:8088/api/pools -H 'Content-Type: application/json' -d '{
  "code": "urun-kimlik-ana", "name": "Ana ürün kimlikleri",
  "kind": "custom", "custom_kind": "urun-kimligi", "ranges": [{"prefix": "246"}]}'
```

## Proje yapısı

```
backend/
  cmd/server/            giriş noktası, rezervasyon süpürücüsü
  internal/db/           bağlantı + gömülü migration'lar (migrations/*.sql)
  internal/kinds/        tip tanımları, kontrol hanesi algoritmaları, özel tip motoru (custom.go)
  internal/store/        iş kuralları ve SQL (tahsis algoritması: allocations.go)
  internal/api/          REST uç noktaları, SPA sunumu
  internal/web/dist/     derlenmiş arayüz (go:embed)
frontend/
  src/routes/            Pano, Havuzlar, Havuz detayı, Tahsisler, Kod tipleri, Doğrula & Sorgula
  src/lib/               API istemcisi, tipler, bileşenler
```

## Bilinçli olarak dışarıda bırakılanlar

- **Kimlik doğrulama / yetkilendirme yok.** `X-Actor` başlığı güvenilir değildir; üretimde önüne
  bir kimlik katmanı (OIDC, reverse proxy, API anahtarı) konmalı. Özel tip tanımlamak gibi yönetim
  işlemleri ayrıca yetkiye bağlanmalı.
- **Çok seçici kurallar:** İmleç bir tahsiste en fazla 2 milyon gövde tarar (~0,5 sn). Kurallar uzun
  ardışık bölgeleri dışlıyorsa (ör. yalnızca `digit(1) == 9`) ve aralık bu bölgeden başlıyorsa tahsis
  "tarama sınırı" hatası verir. Çözüm: aralığı geçerli bölgeye denk gelen bir önekle eklemek.
- **MAC kapsamı:** Yalnızca EUI-48 (6 oktet). EUI-64 (8 oktet, ör. IPv6/Zigbee) 64 bit olduğu için
  `bigint` aralıklarına sığmaz; `numeric` sütunlar gerektirir. OUI'nin hangi firmaya ait olduğu (IEEE kayıt
  listesi) sorgulanmıyor.
- **ISBN tire biçimi** (978-605-…): grup/yayıncı sınırları ISBN Ajansı'nın aralık tablosuna bağlı, eklenmedi.
- **Çok büyük havuzlarda istatistik**: sayımlar her istekte `count(*) FILTER` ile hesaplanıyor. Milyonlarca
  dokunulmuş değerde bir sayaç tablosuna geçilmeli.
- Belirli bir değerin önüne çok sayıda blok/elle tahsis yığılırsa imleç bunları tek tek atlar.
