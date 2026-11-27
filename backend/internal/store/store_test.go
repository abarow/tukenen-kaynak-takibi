// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

package store

// Entegrasyon testleri gerçek PostgreSQL ister:
//
//	TEST_DATABASE_URL=postgres://kaynak:kaynak@localhost:5433/kaynak?sslmode=disable go test ./...
//
// Testler bu sunucuda kaynak_test veritabanını sıfırdan oluşturur; ana veriye dokunmaz.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"kaynak/internal/db"
	"kaynak/internal/kinds"
)

var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		fmt.Println("TEST_DATABASE_URL tanımlı değil; store entegrasyon testleri atlanıyor")
		os.Exit(0)
	}
	ctx := context.Background()
	admin, err := db.Connect(ctx, raw)
	if err != nil {
		panic(err)
	}
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS kaynak_test WITH (FORCE)`); err != nil {
		panic(err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE kaynak_test`); err != nil {
		panic(err)
	}
	admin.Close()

	u, _ := url.Parse(raw)
	u.Path = "/kaynak_test"
	if testDB, err = db.Connect(ctx, u.String()); err != nil {
		panic(err)
	}
	if err := db.Migrate(ctx, testDB); err != nil {
		panic(err)
	}
	code := m.Run()
	testDB.Close()
	os.Exit(code)
}

var seq atomic.Int64

// newPool her test için kendi uzayında çakışmayan bir sıra numarası havuzu açar.
func newSeqPool(t *testing.T, size int64, policy string) (*Store, *Pool) {
	t.Helper()
	st := New(testDB)
	n := seq.Add(1)
	start, end := int64(1), size
	p, err := st.CreatePool(context.Background(), CreatePoolInput{
		Code: fmt.Sprintf("t%d-%s", n, policy), Name: t.Name(), Kind: kinds.Sequence,
		Prefix: fmt.Sprintf("T%d-", n), PadWidth: 4, ReusePolicy: policy, CooldownDays: cooldownFor(policy),
		Ranges: []RangeInput{{Start: &start, End: &end}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return st, p
}

func cooldownFor(policy string) int {
	if policy == ReuseCooldown {
		return 30
	}
	return 0
}

func alloc(t *testing.T, st *Store, p *Pool, ref string) (*Allocation, bool) {
	t.Helper()
	a, created, err := st.Allocate(context.Background(), p.ID, AllocateRequest{ObjectType: "test", ObjectRef: ref}, "test")
	if err != nil {
		t.Fatalf("%s için tahsis: %v", ref, err)
	}
	return a, created
}

func errCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestSequentialAndIdempotent(t *testing.T) {
	st, p := newSeqPool(t, 10, ReuseNever)
	a1, c1 := alloc(t, st, p, "A")
	a1b, c1b := alloc(t, st, p, "A")
	b, _ := alloc(t, st, p, "B")
	if !c1 || c1b {
		t.Fatalf("created bayrakları yanlış: %v %v", c1, c1b)
	}
	if a1.Value != a1b.Value || a1.ID != a1b.ID {
		t.Fatalf("aynı nesne farklı değer aldı: %s / %s", a1.Value, a1b.Value)
	}
	want := p.Codec().Format(1)
	if a1.Value != want || b.Value != p.Codec().Format(2) {
		t.Fatalf("sıra beklenmiyor: %s %s", a1.Value, b.Value)
	}
}

func TestEAN13AllocationFormatsCheckDigit(t *testing.T) {
	st := New(testDB)
	p, err := st.CreatePool(context.Background(), CreatePoolInput{
		Code: "ean-test", Name: "EAN test", Kind: kinds.EAN13,
		Ranges: []RangeInput{{Prefix: "8690000001"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.Capacity != 100 {
		t.Fatalf("kapasite 100 olmalı: %d", p.Stats.Capacity)
	}
	a, _ := alloc(t, st, p, "SKU-1")
	if r := p.Codec().Validate(a.Value); !r.Valid || a.Value[:12] != "869000000100" {
		t.Fatalf("üretilen barkod geçersiz: %s (%s)", a.Value, r.Error)
	}
}

func TestMACAllocationFromOUI(t *testing.T) {
	ctx := context.Background()
	st := New(testDB)
	p, err := st.CreatePool(ctx, CreatePoolInput{
		Code: "mac-test", Name: "MAC test", Kind: kinds.MAC,
		Ranges: []RangeInput{{Prefix: "02:4B:59"}}, // MA-L boyunda, yerel yönetilen
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.Capacity != 1<<24 {
		t.Fatalf("bir MA-L bloğu 16.777.216 adres içermeli: %d", p.Stats.Capacity)
	}
	a, _ := alloc(t, st, p, "vm-1")
	b, _ := alloc(t, st, p, "vm-2")
	if a.Value != "02:4B:59:00:00:00" || b.Value != "02:4B:59:00:00:01" {
		t.Fatalf("MAC sırası/biçimi beklenmiyor: %s %s", a.Value, b.Value)
	}
	// Farklı yazımla belirli değer isteği standart biçime çevrilmeli.
	c, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "vm-3", Value: "024b.5900.00aa"}, "test")
	if err != nil || c.Value != "02:4B:59:00:00:AA" {
		t.Fatalf("Cisco yazımı: %v %+v", err, c)
	}
	if m, _ := st.Lookup(ctx, "02-4b-59-00-00-01"); len(m) != 1 || *m[0].Allocation.ObjectRef != "vm-2" {
		t.Fatalf("Windows yazımıyla sorgu bulmalı: %+v", m)
	}
	// Çakışma: aynı uzayda daha dar bir blok (MA-S) reddedilmeli.
	if _, err := st.AddRange(ctx, p.ID, RangeInput{Prefix: "02:4B:59:00:0"}); errCode(err) != "range_overlap" {
		t.Fatalf("çakışan MAC bloğu reddedilmeli: %v", err)
	}
	if _, err := st.AddRange(ctx, p.ID, RangeInput{Prefix: "03:00:00"}); errCode(err) != "invalid_value" {
		t.Fatalf("multicast blok reddedilmeli: %v", err)
	}
}

func TestCustomKindPoolAllocatesOnlyValidCodes(t *testing.T) {
	ctx := context.Background()
	st := New(testDB)
	k, err := st.CreateCustomKind(ctx, CustomKindInput{
		Code: fmt.Sprintf("urun-kimligi-%d", seq.Add(1)), Name: "Ürün kimliği", BodyDigits: 8, CheckDigits: 4,
		Rules: []kinds.Rule{
			{Expr: "digitSum(1, 3) % 2 == 0", Message: "ilk 3 hanenin toplamı çift olmalı"},
			{Expr: "digitSum(4, 8) % 3 == 0", Message: "sonraki 5 hanenin toplamı 3'e bölünmeli"},
		},
		CheckExpr: "sum(chunks(1, 8, 2))",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 123456xx: 100 gövdeden yalnızca (h7+h8) % 3 == 0 olan 34'ü geçerli.
	p, err := st.CreatePool(ctx, CreatePoolInput{Code: fmt.Sprintf("urun-%d", seq.Add(1)), Name: "Ürün", Kind: kinds.Custom,
		CustomKind: k.Code, Ranges: []RangeInput{{Prefix: "123456"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Stats.Capacity != 34 || !p.Stats.CapacityExact || p.Stats.Available != 34 {
		t.Fatalf("kapasite geçerli kod sayısı olmalı: %+v", p.Stats)
	}

	// Belirli değer: kurala uymayan reddedilir, uyan alınır ve imleç onu atlar.
	_, _, err = st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "t", ObjectRef: "X", Value: "123456010101"}, "test")
	if errCode(err) != "invalid_value" || !strings.Contains(err.Error(), "3'e bölünmeli") {
		t.Fatalf("kurala uymayan değer kural mesajıyla reddedilmeli: %v", err)
	}
	if _, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "t", ObjectRef: "S", Value: "123456030105"}, "test"); err != nil {
		t.Fatal(err)
	}
	first, _ := alloc(t, st, p, "ilk")
	if first.Value != "123456000102" { // 12+34+56+00 = 102
		t.Fatalf("ilk geçerli kod: %s", first.Value)
	}

	items := make([]AllocateRequest, 32)
	for i := range items {
		items[i] = AllocateRequest{ObjectType: "t", ObjectRef: fmt.Sprintf("u%d", i)}
	}
	res, err := st.AllocateBulk(ctx, p.ID, items, "test")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{first.Value: true, "123456030105": true}
	codec := p.Codec()
	for _, r := range res {
		if seen[r.Allocation.Value] {
			t.Fatalf("%s iki kez verildi", r.Allocation.Value)
		}
		seen[r.Allocation.Value] = true
		if _, err := codec.Parse(r.Allocation.Value); err != nil {
			t.Fatalf("geçersiz kod dağıtıldı %s: %v", r.Allocation.Value, err)
		}
	}
	if _, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "t", ObjectRef: "fazla"}, "test"); errCode(err) != "pool_exhausted" {
		t.Fatalf("34 kod bitince havuz tükenmeli: %v", err)
	}

	if _, err := st.AddRange(ctx, p.ID, RangeInput{Prefix: "124"}); errCode(err) != "invalid" {
		t.Fatalf("hiç geçerli değeri olmayan aralık reddedilmeli: %v", err)
	}
	if err := st.DeleteCustomKind(ctx, k.ID); errCode(err) != "kind_in_use" {
		t.Fatalf("kullanılan tip silinmemeli: %v", err)
	}
	if m, _ := st.Lookup(ctx, "1234-5600-0102"); len(m) != 1 || *m[0].Allocation.ObjectRef != "ilk" {
		t.Fatalf("özel tip değeri sorgulanabilmeli: %+v", m)
	}
}

// 005 migration'ı eski (Türkçe) fonksiyon adlarıyla kayıtlı tanımı İngilizceye çevirmeli;
// kural mesajları aynı kalmalı ve tanım aynı kodları üretmeye devam etmeli.
func TestMigrationRenamesRuleFunctions(t *testing.T) {
	ctx := context.Background()
	sqlText, err := os.ReadFile("../db/migrations/005_english_function_names.sql")
	if err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf("eski-adlar-%d", seq.Add(1))
	oldRules := `[{"expr": "toplam(1, 3) % 2 == 0", "message": "toplam(1,3) çift olmalı"},
		{"expr": "hane(1) != 0 and sayi(2, 3) < 99 and agirlikli(1, 8, [3, 1]) >= 0", "message": ""}]`
	var id int64
	if err := testDB.QueryRow(ctx, `INSERT INTO custom_kinds (code, name, body_digits, check_digits, rules, check_expr, density, example)
		VALUES ($1, 'Eski', 8, 4, $2, 'sum(gruplar(1, 8, 2))', 0.1, '') RETURNING id`, code, oldRules).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(ctx, string(sqlText)); err != nil {
		t.Fatal(err)
	}
	var rules []kinds.Rule
	var raw []byte
	var check string
	if err := testDB.QueryRow(ctx, `SELECT rules, check_expr FROM custom_kinds WHERE id = $1`, id).Scan(&raw, &check); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &rules)
	want := []string{"digitSum(1, 3) % 2 == 0", "digit(1) != 0 and number(2, 3) < 99 and weightedSum(1, 8, [3, 1]) >= 0"}
	if len(rules) != 2 || rules[0].Expr != want[0] || rules[1].Expr != want[1] || check != "sum(chunks(1, 8, 2))" {
		t.Fatalf("çeviri beklenmiyor: %+v / %s", rules, check)
	}
	if rules[0].Message != "toplam(1,3) çift olmalı" {
		t.Fatalf("mesaj değişmemeli: %q", rules[0].Message)
	}
	s, err := kinds.CompileScheme(kinds.SchemeDef{ID: id, Code: code, Name: "Eski", BodyDigits: 8, CheckDigits: 4,
		Rules: rules, CheckExpr: check})
	if err != nil {
		t.Fatalf("çevrilen tanım derlenmeli: %v", err)
	}
	if v := kinds.NewCustomCodec(s).Format(12345678); v != "123456780180" {
		t.Fatalf("çevrilen tanım aynı kodu üretmeli: %s", v)
	}
}

// Birden çok düğümde görülen durum: toplu istekler ve tekil istekler aynı nesneleri farklı
// sırada ister. Eskiden toplu istek aralık kilidini tutarken bir nesne kilidini, tekil istek
// o nesne kilidini tutarken aralık kilidini bekliyor ve PostgreSQL birini "deadlock" ile
// iptal ediyordu. Kilitler artık sıralı alındığı için kilitlenme hiç oluşmamalı.
func TestBulkAndSingleOverlapDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 100_000, ReuseNever)
	shared := make([]string, 25)
	for i := range shared {
		shared[i] = fmt.Sprintf("ortak-%d", i)
	}
	before := txRetries.Load()

	var wg sync.WaitGroup
	var failures atomic.Int64
	for b := range 30 {
		wg.Go(func() {
			refs := slices.Clone(shared)
			rand.Shuffle(len(refs), func(i, j int) { refs[i], refs[j] = refs[j], refs[i] })
			items := []AllocateRequest{}
			for _, r := range refs[:12] {
				items = append(items, AllocateRequest{ObjectType: "t", ObjectRef: r})
			}
			for j := range 8 {
				items = append(items, AllocateRequest{ObjectType: "t", ObjectRef: fmt.Sprintf("toplu-%d-%d", b, j)})
			}
			if _, err := st.AllocateBulk(ctx, p.ID, items, "test"); err != nil {
				failures.Add(1)
				t.Errorf("toplu: %v", err)
			}
		})
	}
	for s := range 150 {
		wg.Go(func() {
			if _, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "t", ObjectRef: shared[s%len(shared)]}, "test"); err != nil {
				failures.Add(1)
				t.Errorf("tekil: %v", err)
			}
		})
	}
	wg.Wait()
	if failures.Load() > 0 {
		t.Fatalf("%d istek başarısız", failures.Load())
	}
	if n := txRetries.Load() - before; n != 0 {
		t.Fatalf("kilitlenme önlenmeliydi, ama %d kez tekrar denemeye düşüldü", n)
	}
	got, _ := st.GetPool(ctx, p.ID)
	if want := int64(len(shared) + 30*8); got.Stats.Assigned != want {
		t.Fatalf("her nesne tam bir değer almalı: %d, beklenen %d", got.Stats.Assigned, want)
	}
}

func TestConcurrentAllocationNeverDuplicates(t *testing.T) {
	const capacity, workers = 30, 80
	st, p := newSeqPool(t, capacity, ReuseNever)

	var wg sync.WaitGroup
	var mu sync.Mutex
	values := map[string]string{}
	var exhausted, other atomic.Int64
	for i := range workers {
		wg.Go(func() {
			ref := fmt.Sprintf("obj-%d", i)
			a, _, err := st.Allocate(context.Background(), p.ID, AllocateRequest{ObjectType: "test", ObjectRef: ref}, "test")
			switch {
			case errCode(err) == "pool_exhausted":
				exhausted.Add(1)
			case err != nil:
				other.Add(1)
				t.Errorf("beklenmeyen hata: %v", err)
			default:
				mu.Lock()
				if prev, dup := values[a.Value]; dup {
					t.Errorf("%s iki nesneye verildi: %s ve %s", a.Value, prev, ref)
				}
				values[a.Value] = ref
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(values) != capacity || exhausted.Load() != workers-capacity {
		t.Fatalf("başarılı=%d tükendi=%d diğer=%d", len(values), exhausted.Load(), other.Load())
	}
	got, _ := st.GetPool(context.Background(), p.ID)
	if got.Stats.Available != 0 || got.Stats.Assigned != capacity {
		t.Fatalf("istatistik hatalı: %+v", got.Stats)
	}
}

func TestConcurrentSameObjectGetsOneValue(t *testing.T) {
	st, p := newSeqPool(t, 100, ReuseNever)
	var wg sync.WaitGroup
	var created atomic.Int64
	var mu sync.Mutex
	seen := map[string]bool{}
	for range 25 {
		wg.Go(func() {
			a, c, err := st.Allocate(context.Background(), p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "same"}, "test")
			if err != nil {
				t.Error(err)
				return
			}
			if c {
				created.Add(1)
			}
			mu.Lock()
			seen[a.Value] = true
			mu.Unlock()
		})
	}
	wg.Wait()
	if created.Load() != 1 || len(seen) != 1 {
		t.Fatalf("aynı nesne için %d yeni tahsis, %d farklı değer", created.Load(), len(seen))
	}
}

func TestReleasePolicies(t *testing.T) {
	ctx := context.Background()

	st, never := newSeqPool(t, 10, ReuseNever)
	a, _ := alloc(t, st, never, "A")
	if _, err := st.Release(ctx, a.ID, "", "test"); err != nil {
		t.Fatal(err)
	}
	if b, _ := alloc(t, st, never, "B"); b.Value == a.Value {
		t.Fatal("never politikasında bırakılan değer tekrar verildi")
	}

	_, immediate := newSeqPool(t, 10, ReuseImmediate)
	a, _ = alloc(t, st, immediate, "A")
	if _, err := st.Release(ctx, a.ID, "", "test"); err != nil {
		t.Fatal(err)
	}
	if b, _ := alloc(t, st, immediate, "B"); b.Value != a.Value {
		t.Fatalf("immediate politikasında bırakılan değer önce kullanılmalıydı: %s != %s", b.Value, a.Value)
	}

	_, cooldown := newSeqPool(t, 10, ReuseCooldown)
	a, _ = alloc(t, st, cooldown, "A")
	rel, err := st.Release(ctx, a.ID, "", "test")
	if err != nil || rel.ReusableAt == nil {
		t.Fatalf("cooldown bırakma: %v %+v", err, rel)
	}
	if b, _ := alloc(t, st, cooldown, "B"); b.Value == a.Value {
		t.Fatal("karantinadaki değer verildi")
	}
	// Karantina bitmiş gibi yap.
	if _, err := testDB.Exec(ctx, `UPDATE allocations SET reusable_at = now() - interval '1 second' WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := alloc(t, st, cooldown, "C"); c.Value != a.Value {
		t.Fatalf("karantinası biten değer verilmeliydi: %s != %s", c.Value, a.Value)
	}
}

func TestReservationExpiryFreesValue(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 10, ReuseNever)
	r, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "R", Mode: ModeReserve}, "test")
	if err != nil || r.Status != StatusReserved || r.ReservedUntil == nil {
		t.Fatalf("rezervasyon: %v %+v", err, r)
	}
	if _, err := testDB.Exec(ctx, `UPDATE allocations SET reserved_until = now() - interval '1 second' WHERE id = $1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Confirm(ctx, r.ID, "", "test"); errCode(err) != "reservation_expired" {
		t.Fatalf("süresi dolan rezervasyon onaylanmamalı: %v", err)
	}
	if n, err := st.ExpireReservations(ctx); err != nil || n < 1 {
		t.Fatalf("temizlik: %d %v", n, err)
	}
	// never politikası olsa bile hiç kullanılmamış değer geri döner.
	if b, _ := alloc(t, st, p, "B"); b.Value != r.Value {
		t.Fatalf("süresi dolan rezervasyonun değeri tekrar verilmeliydi: %s != %s", b.Value, r.Value)
	}
}

func TestReserveThenAssignConfirms(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 10, ReuseNever)
	r, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "X", Mode: ModeReserve}, "test")
	if err != nil {
		t.Fatal(err)
	}
	a, created := alloc(t, st, p, "X") // mode=assign aynı nesne → onay
	if created || a.ID != r.ID || a.Status != StatusAssigned {
		t.Fatalf("rezervasyon onaylanmalıydı: created=%v %+v", created, a)
	}
}

func TestSpecificValueAndCursorSkip(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 10, ReuseNever)
	c := p.Codec()

	if _, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "S", Value: c.Format(2)}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Block(ctx, p.ID, c.Format(3), "hasarlı etiket", "test"); err != nil {
		t.Fatal(err)
	}
	_, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "T", Value: c.Format(2)}, "test")
	if errCode(err) != "value_taken" {
		t.Fatalf("alınmış değer ikinci kez verilmemeli: %v", err)
	}
	var got []string
	for _, ref := range []string{"A", "B", "C"} {
		a, _ := alloc(t, st, p, ref)
		got = append(got, a.Value)
	}
	want := []string{c.Format(1), c.Format(4), c.Format(5)}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("imleç elle alınan/bloklu değerleri atlamalı: %v, beklenen %v", got, want)
	}
	if _, _, err := st.Allocate(ctx, p.ID, AllocateRequest{ObjectType: "test", ObjectRef: "Z", Value: c.Format(99)}, "test"); errCode(err) != "invalid" {
		t.Fatalf("aralık dışı değer reddedilmeli: %v", err)
	}
}

func TestOverlappingRangesRejectedAcrossKinds(t *testing.T) {
	ctx := context.Background()
	st := New(testDB)
	if _, err := st.CreatePool(ctx, CreatePoolInput{Code: "isbn-test", Name: "ISBN", Kind: kinds.ISBN13,
		Ranges: []RangeInput{{Prefix: "9786050001"}}}); err != nil {
		t.Fatal(err)
	}
	// Aynı GTIN-13 uzayı: daha geniş bir EAN-13 öneki ISBN bloğunu kapsıyor.
	_, err := st.CreatePool(ctx, CreatePoolInput{Code: "ean-overlap", Name: "EAN", Kind: kinds.EAN13,
		Ranges: []RangeInput{{Prefix: "97860500"}}})
	if errCode(err) != "range_overlap" {
		t.Fatalf("çakışan aralık reddedilmeliydi: %v", err)
	}
	// Transaction geri alındığı için havuz da oluşmamış olmalı.
	if _, err := st.ResolvePool(ctx, "ean-overlap"); errCode(err) != "not_found" {
		t.Fatalf("başarısız oluşturma havuz bırakmamalı: %v", err)
	}
}

func TestBulkIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 3, ReuseNever)
	items := []AllocateRequest{{ObjectType: "t", ObjectRef: "1"}, {ObjectType: "t", ObjectRef: "2"},
		{ObjectType: "t", ObjectRef: "3"}, {ObjectType: "t", ObjectRef: "4"}}
	if _, err := st.AllocateBulk(ctx, p.ID, items, "test"); errCode(err) != "pool_exhausted" {
		t.Fatalf("kapasite aşımı hata vermeli: %v", err)
	}
	got, _ := st.GetPool(ctx, p.ID)
	if got.Stats.Available != 3 {
		t.Fatalf("başarısız toplu işlem hiçbir şey yazmamalı: %+v", got.Stats)
	}
	res, err := st.AllocateBulk(ctx, p.ID, items[:3], "test")
	if err != nil || len(res) != 3 {
		t.Fatalf("toplu tahsis: %v", err)
	}
}

func TestLookup(t *testing.T) {
	ctx := context.Background()
	st, p := newSeqPool(t, 10, ReuseNever)
	a, _ := alloc(t, st, p, "L")
	m, err := st.Lookup(ctx, a.Value)
	if err != nil || len(m) != 1 || m[0].State != StatusAssigned || *m[0].Allocation.ObjectRef != "L" {
		t.Fatalf("lookup: %v %+v", err, m)
	}
	m, _ = st.Lookup(ctx, p.Codec().Format(9))
	if len(m) != 1 || m[0].State != "available" {
		t.Fatalf("boş değer available görünmeli: %+v", m)
	}
}
