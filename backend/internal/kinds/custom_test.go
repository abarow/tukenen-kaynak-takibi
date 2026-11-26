package kinds

import (
	"strings"
	"testing"
)

// Kullanıcının tarif ettiği ürün kimliği:
// ilk 3 hane toplamı çift, sonraki 5 hane toplamı 3'e bölünür,
// son 4 hane = ilk 8 hanenin 2'şerli 4 sayı olarak toplamının son 4 hanesi.
func productScheme(t *testing.T) *Scheme {
	t.Helper()
	s, err := CompileScheme(SchemeDef{
		ID: 1, Code: "urun-kimligi", Name: "Ürün kimliği", BodyDigits: 8, CheckDigits: 4,
		Rules: []Rule{
			{Expr: "digitSum(1, 3) % 2 == 0", Message: "ilk 3 hanenin toplamı çift olmalı"},
			{Expr: "digitSum(4, 8) % 3 == 0", Message: "sonraki 5 hanenin toplamı 3'e bölünmeli"},
		},
		CheckExpr: "sum(chunks(1, 8, 2))",
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestProductSchemeFormatAndParse(t *testing.T) {
	c := NewCustomCodec(productScheme(t))
	// 1+2+3=6 çift, 4+5+6+7+8=30 → 3'e bölünür, 12+34+56+78=180 → "0180"
	if v := c.Format(12345678); v != "123456780180" {
		t.Fatalf("Format: %q", v)
	}
	cases := []struct {
		value, errPart string
	}{
		{"123456780180", ""},
		{"1234-5678-0180", ""},
		{"123456780181", "doğrusu 0180"},
		{"124456780180", "ilk 3 hanenin toplamı çift"}, // 1+2+4=7
		{"222222220088", "3'e bölünmeli"},              // 2*5=10
		{"12345678018", "12 hane olmalı"},
	}
	for _, tc := range cases {
		_, err := c.Parse(tc.value)
		switch {
		case tc.errPart == "" && err != nil:
			t.Errorf("%s geçerli olmalı: %v", tc.value, err)
		case tc.errPart != "" && (err == nil || !strings.Contains(err.Error(), tc.errPart)):
			t.Errorf("%s: %q içeren hata bekleniyordu, alınan: %v", tc.value, tc.errPart, err)
		}
	}
}

func TestProductSchemeDensityAndCount(t *testing.T) {
	s := productScheme(t)
	// P(3 hane toplamı çift) = 1/2, P(5 hane toplamı ≡ 0 mod 3) ≈ 1/3 → ≈ %16,7
	if s.Density < 0.155 || s.Density > 0.18 {
		t.Fatalf("yoğunluk beklenmiyor: %v", s.Density)
	}
	c := NewCustomCodec(s)
	// 123456xx: kural 1 hep sağlanır, kural 2 için (h7+h8) % 3 == 0 → 100 çiftten 34'ü
	start, end, _ := c.PrefixRange("123456")
	n, exact, err := c.CountValid(start, end)
	if err != nil || !exact || n != 34 {
		t.Fatalf("123456xx sayımı: %d exact=%v %v", n, exact, err)
	}
	// 124xxxxx: 1+2+4 tek → hiç geçerli değer yok
	start, end, _ = c.PrefixRange("124")
	if n, _, _ := c.CountValid(start, end); n != 0 {
		t.Fatalf("124 önekinde geçerli değer olmamalı: %d", n)
	}
	// Büyük aralık örneklemle tahmin edilir.
	n, exact, _ = c.CountValid(0, 99_999_999)
	if exact || n < 15_500_000 || n > 18_000_000 {
		t.Fatalf("tüm uzay tahmini: %d exact=%v", n, exact)
	}
}

func TestFirstValidSkipsInvalidBodies(t *testing.T) {
	c := NewCustomCodec(productScheme(t))
	v, found, err := c.FirstValid(12345679, 12345699, 1000)
	if err != nil || !found {
		t.Fatal(found, err)
	}
	for b := int64(12345679); b < v; b++ {
		if _, err := c.Parse(c.Format(b)); err == nil {
			t.Fatalf("%d geçerliydi ama atlandı", b)
		}
	}
	if _, err := c.Parse(c.Format(v)); err != nil {
		t.Fatalf("bulunan %d geçersiz: %v", v, err)
	}
	if _, found, _ := c.FirstValid(12400000, 12499999, 1_000_000); found {
		t.Fatal("124 önekinde değer bulunmamalı")
	}
	if _, _, err := c.FirstValid(12400000, 12499999, 10); err != ErrScanLimit {
		t.Fatalf("tarama sınırı hatası bekleniyordu: %v", err)
	}
}

func TestCustomSchemeCanExpressGS1(t *testing.T) {
	// agirlikli ile EAN-13 kontrol hanesini yeniden tanımla; yerleşik tiple aynı sonucu vermeli.
	s, err := CompileScheme(SchemeDef{ID: 2, Code: "gs1", Name: "GS1", BodyDigits: 12, CheckDigits: 1,
		CheckExpr: "(10 - weightedSum(1, 12, [1, 3]) % 10) % 10"})
	if err != nil {
		t.Fatal(err)
	}
	custom, ean := NewCustomCodec(s), codec(t, EAN13, "", 0)
	for _, b := range []int64{869123456789, 400638133393, 978030640615, 1, 999999999999} {
		if custom.Format(b) != ean.Format(b) {
			t.Errorf("%d: özel %s, EAN-13 %s", b, custom.Format(b), ean.Format(b))
		}
	}
}

// Arayüzdeki "ISBN-13" örneği: hazır ISBN-13 tipiyle birebir aynı kodları üretmeli ve kabul etmeli.
func TestCustomSchemeISBNExampleMatchesBuiltin(t *testing.T) {
	s, err := CompileScheme(SchemeDef{ID: 3, Code: "isbn-formul", Name: "ISBN-13 (formülle)", BodyDigits: 12, CheckDigits: 1,
		Rules:     []Rule{{Expr: "number(1, 3) == 978 or number(1, 3) == 979", Message: "ISBN 978 veya 979 ile başlamalı"}},
		CheckExpr: "(10 - weightedSum(1, 12, [1, 3]) % 10) % 10"})
	if err != nil {
		t.Fatal(err)
	}
	custom, isbn := NewCustomCodec(s), codec(t, ISBN13, "", 0)
	for _, b := range []int64{978030640615, 978605999900, 979100000000, 978999999999} {
		if custom.Format(b) != isbn.Format(b) {
			t.Errorf("%d: formül %s, hazır tip %s", b, custom.Format(b), isbn.Format(b))
		}
	}
	for _, v := range []string{"978-0-306-40615-7", "9786059999014", "9780306406158", "8691234567890"} {
		_, errC := custom.Parse(v)
		_, errB := isbn.Parse(v)
		if (errC == nil) != (errB == nil) {
			t.Errorf("%s: formül %v, hazır tip %v", v, errC, errB)
		}
	}
	// Yalnızca 978/979 ile başlayan gövdeler geçerli: 12 haneli uzayın binde 2'si.
	if s.Density < 0.001 || s.Density > 0.003 {
		t.Errorf("yoğunluk beklenmiyor: %v", s.Density)
	}
}

func TestCompileSchemeRejectsBadDefinitions(t *testing.T) {
	cases := map[string]SchemeDef{
		"bilinmeyen fonksiyon":      {BodyDigits: 8, Rules: []Rule{{Expr: "topla(1, 3) % 2 == 0"}}},
		"gövde dışı konum":          {BodyDigits: 8, Rules: []Rule{{Expr: "digit(9) > 0"}}},
		"hiçbir değeri kabul etmez": {BodyDigits: 8, Rules: []Rule{{Expr: "digit(1) == 10"}}},
		"deterministik değil":       {BodyDigits: 8, Rules: []Rule{{Expr: "now() != nil"}}},
		"kural bool değil":          {BodyDigits: 8, Rules: []Rule{{Expr: "digitSum(1, 3)"}}},
		"negatif kontrol":           {BodyDigits: 8, CheckDigits: 2, CheckExpr: "0 - digitSum(1, 8)"},
		"kontrol formülü eksik":     {BodyDigits: 8, CheckDigits: 2},
		"bölünemeyen gruplar":       {BodyDigits: 8, CheckDigits: 2, CheckExpr: "sum(chunks(1, 8, 3))"},
		"çok uzun":                  {BodyDigits: 15, CheckDigits: 4, CheckExpr: "1"},
	}
	for name, def := range cases {
		if _, err := CompileScheme(def); err == nil {
			t.Errorf("%s: reddedilmeliydi", name)
		}
	}
}

func BenchmarkCountValid10k(b *testing.B) {
	s, _ := CompileScheme(SchemeDef{ID: 1, Code: "b", Name: "b", BodyDigits: 8, CheckDigits: 4,
		Rules:     []Rule{{Expr: "digitSum(1, 3) % 2 == 0"}, {Expr: "digitSum(4, 8) % 3 == 0"}},
		CheckExpr: "sum(chunks(1, 8, 2))"})
	c := NewCustomCodec(s)
	for b.Loop() {
		c.CountValid(12340000, 12349999)
	}
}
