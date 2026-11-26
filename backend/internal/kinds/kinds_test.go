package kinds

import "testing"

func codec(t *testing.T, k Kind, prefix string, pad int) Codec {
	t.Helper()
	c, err := NewCodec(k, prefix, pad)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCatalogExamplesAreValid(t *testing.T) {
	for _, info := range All() {
		prefix := ""
		pad := 0
		if info.Code == Sequence {
			prefix, pad = "FTR-", 6
		}
		r := codec(t, info.Code, prefix, pad).Validate(info.Example)
		if !r.Valid {
			t.Errorf("%s örneği %q geçersiz: %s", info.Code, info.Example, r.Error)
		}
	}
}

func TestKnownValues(t *testing.T) {
	cases := []struct {
		kind  Kind
		value string
		ok    bool
	}{
		{ISBN13, "978-0-306-40615-7", true},
		{ISBN13, "9780306406158", false}, // kontrol hanesi hatalı
		{ISBN13, "4006381333931", false}, // geçerli EAN ama 978/979 değil
		{EAN13, "4006381333931", true},
		{EAN13, "400638133393", false}, // eksik hane
		{EAN8, "96385074", true},
		{EAN8, "96385075", false},
		{MAC, "00:00:5E:00:53:01", true},
		{MAC, "00-00-5e-00-53-01", true},  // Windows yazımı, küçük harf
		{MAC, "0000.5E00.5301", true},     // Cisco yazımı
		{MAC, "02:4B:59:00:00:01", true},  // yerel yönetilen (U/L biti 1) ama tekil: geçerli
		{MAC, "01:00:5E:00:00:FB", false}, // IPv4 multicast (mDNS)
		{MAC, "FF:FF:FF:FF:FF:FF", false}, // broadcast
		{MAC, "33:33:00:00:00:01", false}, // IPv6 multicast
		{MAC, "00:00:5E:00:53", false},    // eksik oktet
		{MAC, "00:00:5E:00:53:G1", false},
	}
	for _, tc := range cases {
		r := codec(t, tc.kind, "", 0).Validate(tc.value)
		if r.Valid != tc.ok {
			t.Errorf("%s %q: valid=%v beklenen %v (%s)", tc.kind, tc.value, r.Valid, tc.ok, r.Error)
		}
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	cases := []struct {
		c    Codec
		base int64
	}{
		{codec(t, EAN13, "", 0), 869123400000},
		{codec(t, EAN13, "", 0), 1},
		{codec(t, EAN8, "", 0), 9638507},
		{codec(t, ISBN13, "", 0), 978605456700},
		{codec(t, MAC, "", 0), 0x00005E005301},
		{codec(t, MAC, "", 0), 0x024B59FFFFFF},
		{codec(t, MAC, "", 0), 0xFCFFFFFFFFFF}, // en büyük tekil ilk oktet
		{codec(t, Sequence, "FTR-", 6), 42},
		{codec(t, Sequence, "", 0), 7},
	}
	for _, tc := range cases {
		v := tc.c.Format(tc.base)
		got, err := tc.c.Parse(v)
		if err != nil || got != tc.base {
			t.Errorf("%s: Format(%d)=%q Parse=%d err=%v", tc.c.Kind, tc.base, v, got, err)
		}
	}
	if v := codec(t, Sequence, "FTR-", 6).Format(42); v != "FTR-000042" {
		t.Errorf("sequence biçimi: %q", v)
	}
	if v := codec(t, MAC, "", 0).Format(0x00005E005301); v != "00:00:5E:00:53:01" {
		t.Errorf("MAC biçimi: %q", v)
	}
}

func TestMACDetails(t *testing.T) {
	mac := codec(t, MAC, "", 0)
	r := mac.Validate("0000.5e00.5301")
	if !r.Valid || r.Normalized != "00:00:5E:00:53:01" || r.Base != "00005E005301" || r.Details != "OUI 00:00:5E · evrensel (IEEE OUI) · unicast" {
		t.Fatalf("evrensel adres: %+v", r)
	}
	if r := mac.Validate("02:4B:59:00:00:01"); r.Details != "OUI 02:4B:59 · yerel yönetilen (U/L biti 1) · unicast" {
		t.Fatalf("yerel yönetilen adres: %+v", r)
	}
	if r := mac.Validate("01:00:5E:00:00:FB"); r.Valid || r.Error == "" {
		t.Fatalf("multicast açıklamalı hata vermeli: %+v", r)
	}
}

func TestSequenceParseRejectsNonCanonical(t *testing.T) {
	c := codec(t, Sequence, "FTR-", 6)
	for _, v := range []string{"FTR-42", "FTR-0000042", "XYZ-000042", "FTR-", "FTR-00004a"} {
		if _, err := c.Parse(v); err == nil {
			t.Errorf("%q kabul edilmemeliydi", v)
		}
	}
}

func TestPrefixRange(t *testing.T) {
	start, end, err := codec(t, EAN13, "", 0).PrefixRange("8691234")
	if err != nil || start != 869123400000 || end != 869123499999 {
		t.Fatalf("EAN-13 önek aralığı: %d-%d %v", start, end, err)
	}
	start, end, err = codec(t, ISBN13, "", 0).PrefixRange("978-605-4567")
	if err != nil || start != 978605456700 || end != 978605456799 {
		t.Fatalf("ISBN önek aralığı: %d-%d %v", start, end, err)
	}
	if _, _, err := codec(t, ISBN13, "", 0).PrefixRange("8691234"); err == nil {
		t.Fatal("978/979 dışı ISBN öneki kabul edilmemeli")
	}
	// IEEE blok boyları: MA-L (24 bit önek), MA-M (28 bit), MA-S (36 bit).
	mac := codec(t, MAC, "", 0)
	for prefix, size := range map[string]int64{"00:00:5E": 1 << 24, "00:00:5E:0": 1 << 20, "00-00-5E-00-5": 1 << 12, "00:00:5E:00:53": 256} {
		start, end, err := mac.PrefixRange(prefix)
		if err != nil || end-start+1 != size || mac.Format(start)[:8] != "00:00:5E" {
			t.Errorf("MAC önek %s: %s-%s (%d) %v", prefix, mac.Format(start), mac.Format(end), end-start+1, err)
		}
	}
	for _, bad := range []string{"01:00:5E", "0", "00:00:5E:00:53:01", "XY"} {
		if _, _, err := mac.PrefixRange(bad); err == nil {
			t.Errorf("MAC önek %q reddedilmeli (multicast / birden çok ilk oktet / tam adres / onaltılık değil)", bad)
		}
	}
}

func TestMACRangeMustStayInOneUnicastOctet(t *testing.T) {
	mac := codec(t, MAC, "", 0)
	if err := mac.ValidateRange(0x02_0000000000, 0x03_0000000000); err == nil {
		t.Fatal("02..03 aralığı multicast 03 oktetini kapsıyor, reddedilmeli")
	}
	if err := mac.ValidateRange(0x02_0000000000, 0x02_FFFFFFFFFF); err != nil {
		t.Fatalf("tek tekil oktet geçerli olmalı: %v", err)
	}
	if err := mac.ValidateRange(0, 0x1_000000000000); err == nil {
		t.Fatal("48 biti aşan değer reddedilmeli")
	}
}

func TestNamespaces(t *testing.T) {
	if codec(t, EAN13, "", 0).Namespace() != codec(t, ISBN13, "", 0).Namespace() {
		t.Fatal("EAN-13 ve ISBN-13 aynı uzayı paylaşmalı")
	}
	if codec(t, Sequence, "A-", 0).Namespace() == codec(t, Sequence, "B-", 0).Namespace() {
		t.Fatal("farklı önekler farklı uzaylar olmalı")
	}
}

func TestCodecRejectsBadConfig(t *testing.T) {
	if _, err := NewCodec(EAN13, "X", 0); err == nil {
		t.Error("EAN-13 öneki kabul etmemeli")
	}
	if _, err := NewCodec(Sequence, "INV1", 0); err == nil {
		t.Error("rakamla biten önek reddedilmeli")
	}
	if _, err := NewCodec("foo", "", 0); err == nil {
		t.Error("bilinmeyen tip reddedilmeli")
	}
}
