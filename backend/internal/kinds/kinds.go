// Package kinds kaynak tiplerini (EAN-13, ISBN-13, MAC adresi, ...) tanımlar:
// biçimlendirme, doğrulama ve kontrol hanesi / yapı kuralları.
//
// Her değer bir "gövde" (base) sayısı ile temsil edilir. Aralıklar gövde
// üzerinde tanımlanır. Örneğin EAN-13 için gövde ilk 12 onluk hanedir ve
// 13. hane hesaplanır; MAC adresi için gövde 48 bitlik sayının kendisidir
// (12 onaltılık hane), kontrol hanesi yoktur ama bit kuralları vardır.
package kinds

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	EAN13    Kind = "ean13"
	EAN8     Kind = "ean8"
	ISBN13   Kind = "isbn13"
	MAC      Kind = "mac"
	Sequence Kind = "sequence"
)

// Info, arayüzün ve API istemcilerinin tip hakkında bilmesi gerekenler.
type Info struct {
	Code          Kind   `json:"code"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	BaseDigits    int    `json:"base_digits"`  // kontrol haneleri hariç gövde uzunluğu (Radix tabanında); 0 = serbest
	CheckDigits   int    `json:"check_digits"` // eklenen kontrol hanesi sayısı
	Radix         int    `json:"radix"`        // gövdenin sayı tabanı: 10 veya 16
	Example       string `json:"example"`
	DefaultPolicy string `json:"default_reuse_policy"`
	// Yalnızca özel tiplerde: tip kodu ve kurallara uyan gövdelerin tahmini oranı.
	CustomKind string  `json:"custom_kind,omitempty"`
	Density    float64 `json:"density,omitempty"`
}

var catalog = []Info{
	{EAN13, "EAN-13 / GTIN-13", "GS1 ürün barkodu. Gövde: firma öneki + ürün numarası (12 hane), 13. hane kontrol hanesi.", 12, 1, 10, "8691234567890", "never", "", 0},
	{EAN8, "EAN-8 / GTIN-8", "Küçük ambalajlar için kısa GS1 barkodu. 7 hane gövde + 1 kontrol hanesi.", 7, 1, 10, "96385074", "never", "", 0},
	{ISBN13, "ISBN-13", "Kitap numarası. 978/979 + grup + yayıncı + yayın numarası (12 hane) + kontrol hanesi.", 12, 1, 10, "9780306406157", "never", "", 0},
	{MAC, "MAC adresi (EUI-48)", "Fiziksel ağ adresi. 48 bit, 12 onaltılık hane. IEEE'den alınan blok: MA-L (6 hane önek, 16,7 milyon adres), MA-M (7 hane, 1 milyon), MA-S (9 hane, 4096). Multicast adresler (ilk oktet tek) cihaza verilemez.", 12, 0, 16, "00:00:5E:00:53:01", "never", "", 0},
	{Sequence, "Sıra numarası", "Kontrol hanesi olmayan, önekli ve sıfır dolgulu genel sıra numarası (fatura no, seri no, lisans no...).", 0, 0, 10, "FTR-000123", "never", "", 0},
}

func All() []Info { return append([]Info(nil), catalog...) }

func Lookup(code string) (Info, bool) {
	for _, k := range catalog {
		if string(k.Code) == code {
			return k, true
		}
	}
	return Info{}, false
}

// ErrInvalid doğrulama hatalarını sarar; mesajlar kullanıcıya gösterilebilir.
var ErrInvalid = errors.New("geçersiz değer")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

var prefixPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{0,16}$`)

// Codec bir havuzun tip ayarlarına göre değer üretir ve çözümler.
type Codec struct {
	Kind   Kind
	Prefix string // yalnızca sequence
	Pad    int    // yalnızca sequence
	info   Info
	scheme *Scheme // yalnızca custom
}

func NewCodec(kind Kind, prefix string, pad int) (Codec, error) {
	info, ok := Lookup(string(kind))
	if !ok {
		return Codec{}, invalid("bilinmeyen kaynak tipi %q", kind)
	}
	c := Codec{Kind: kind, info: info}
	if kind == Sequence {
		if !prefixPattern.MatchString(prefix) {
			return Codec{}, invalid("önek en fazla 16 karakter olabilir ve yalnızca harf, rakam, . _ / - içerebilir")
		}
		if prefix != "" && isDigit(prefix[len(prefix)-1]) {
			return Codec{}, invalid("önek rakamla bitemez (değerin sayı kısmıyla karışır)")
		}
		if pad < 0 || pad > 18 {
			return Codec{}, invalid("dolgu genişliği 0-18 arasında olmalı")
		}
		c.Prefix, c.Pad = prefix, pad
	} else if prefix != "" || pad != 0 {
		return Codec{}, invalid("önek ve dolgu yalnızca sıra numarası tipinde kullanılabilir")
	}
	return c, nil
}

func (c Codec) Info() Info { return c.info }

// Namespace aralık çakışma kontrolünün yapıldığı uzay.
func (c Codec) Namespace() string {
	switch c.Kind {
	case EAN13, ISBN13:
		return "gtin13" // ISBN-13 aslında 978/979 önekli bir EAN-13'tür
	case EAN8:
		return "gtin8"
	case MAC:
		return "mac"
	case Custom:
		return "custom:" + strconv.FormatInt(c.scheme.ID, 10)
	default:
		return "seq:" + c.Prefix
	}
}

// MaxBase gövdenin alabileceği en büyük değer.
func (c Codec) MaxBase() int64 {
	if c.info.BaseDigits == 0 {
		return 999_999_999_999_999_999
	}
	return ipow(int64(c.info.Radix), c.info.BaseDigits) - 1
}

// ValidateRange aralığın bu tip için anlamlı olup olmadığını kontrol eder.
func (c Codec) ValidateRange(start, end int64) error {
	if start < 0 || end < 0 {
		return invalid("aralık değerleri negatif olamaz")
	}
	if start > end {
		return invalid("başlangıç bitişten büyük olamaz")
	}
	if end > c.MaxBase() {
		return invalid("bitiş değeri %d haneli gövdeyi aşıyor", c.info.BaseDigits)
	}
	switch c.Kind {
	case ISBN13:
		lo, hi := int64(978_000_000_000), int64(979_999_999_999)
		if start < lo || end > hi {
			return invalid("ISBN-13 gövdesi 978 veya 979 ile başlamalı")
		}
	case MAC:
		// İlk oktet = 48 bitlik sayının en üst 8 biti. Ardışık iki oktetten biri mutlaka
		// tek sayı (multicast) olacağı için aralık tek bir ilk oktet içinde kalmalı.
		first := start >> 40
		if end>>40 != first {
			return invalid("MAC aralığı tek bir ilk oktet içinde kalmalı (önek en az 2 onaltılık hane)")
		}
		if first&1 == 1 {
			return invalid("ilk oktet %02X tek sayı (I/G biti 1): multicast/broadcast adresler cihaza atanamaz", first)
		}
	}
	return nil
}

// PrefixRange, sabit uzunluklu tiplerde bir önekin kapsadığı tüm gövde aralığını verir.
//
//	EAN-13, önek "8691234"      → 869123400000 .. 869123499999 (100.000 barkod)
//	MAC,    önek "00:00:5E" (OUI) → 00:00:5E:00:00:00 .. 00:00:5E:FF:FF:FF (16.777.216 adres)
func (c Codec) PrefixRange(prefix string) (int64, int64, error) {
	n := c.info.BaseDigits
	if n == 0 {
		return 0, 0, invalid("önekten aralık yalnızca sabit uzunluklu tiplerde hesaplanabilir")
	}
	p := c.normalizeBody(prefix)
	if p == "" || !c.bodyChars(p) {
		return 0, 0, invalid("önek yalnızca %s oluşmalı", c.charsLabel())
	}
	if len(p) >= n {
		return 0, 0, invalid("önek gövdeden (%d hane) kısa olmalı", n)
	}
	radix := int64(c.info.Radix)
	v, _ := strconv.ParseInt(p, c.info.Radix, 64)
	scale := ipow(radix, n-len(p))
	start, end := v*scale, (v+1)*scale-1
	return start, end, c.ValidateRange(start, end)
}

// Format gövdeden tam değeri üretir (kontrol haneleriyle ya da standart yazımla).
func (c Codec) Format(base int64) string {
	switch c.Kind {
	case EAN13, EAN8, ISBN13:
		body := padDigits(base, c.info.BaseDigits)
		return body + strconv.Itoa(GTINCheckDigit(body))
	case MAC:
		h := fmt.Sprintf("%012X", base)
		var b strings.Builder
		for i := 0; i < 12; i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			b.WriteString(h[i : i+2])
		}
		return b.String()
	case Custom:
		return c.scheme.format(base)
	default:
		return c.Prefix + fmt.Sprintf("%0*d", c.Pad, base)
	}
}

// Parse kullanıcının girdiği değeri doğrular ve gövdesini döndürür.
func (c Codec) Parse(value string) (int64, error) {
	switch c.Kind {
	case EAN13, EAN8, ISBN13:
		v := c.normalizeBody(value)
		total := c.info.BaseDigits + c.info.CheckDigits
		if !c.bodyChars(v) {
			return 0, invalid("yalnızca rakam içermeli")
		}
		if len(v) != total {
			return 0, invalid("%d hane olmalı, %d hane girildi", total, len(v))
		}
		body := v[:c.info.BaseDigits]
		if want := c.Format(mustInt(body, 10)); want != v {
			return 0, invalid("kontrol hanesi hatalı, doğrusu %s olmalı", want[c.info.BaseDigits:])
		}
		base := mustInt(body, 10)
		if err := c.ValidateRange(base, base); err != nil {
			return 0, err
		}
		return base, nil
	case MAC:
		// 00:00:5E:00:53:01, 00-00-5E-00-53-01 (Windows), 0000.5E00.5301 (Cisco), 00005e005301
		v := c.normalizeBody(value)
		if !c.bodyChars(v) {
			return 0, invalid("yalnızca onaltılık karakterler (0-9, A-F) ve : - . ayraçları kullanılabilir")
		}
		if len(v) != 12 {
			return 0, invalid("12 onaltılık hane olmalı (ör. %s), %d hane girildi", c.info.Example, len(v))
		}
		base := mustInt(v, 16)
		if err := c.ValidateRange(base, base); err != nil {
			return 0, err
		}
		return base, nil
	case Custom:
		return c.scheme.parse(c, value)
	default:
		v := strings.TrimSpace(value)
		if !strings.HasPrefix(v, c.Prefix) {
			return 0, invalid("%q öneki ile başlamalı", c.Prefix)
		}
		num := v[len(c.Prefix):]
		if num == "" || !allDigits(num) || len(num) > 18 {
			return 0, invalid("önekten sonra en fazla 18 rakam gelmeli")
		}
		if c.Pad > 0 && len(num) < c.Pad {
			return 0, invalid("sayı kısmı en az %d hane olmalı", c.Pad)
		}
		base := mustInt(num, 10)
		if c.Format(base) != v {
			return 0, invalid("standart biçim %s olmalı", c.Format(base))
		}
		return base, nil
	}
}

// ValidationResult doğrulama aracının çıktısı.
type ValidationResult struct {
	Valid      bool   `json:"valid"`
	Normalized string `json:"normalized,omitempty"`
	Base       string `json:"base,omitempty"`
	Details    string `json:"details,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (c Codec) Validate(value string) ValidationResult {
	base, err := c.Parse(value)
	if err != nil {
		return ValidationResult{Error: strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")}
	}
	r := ValidationResult{Valid: true, Normalized: c.Format(base)}
	switch {
	case c.Kind == MAC:
		r.Base = fmt.Sprintf("%012X", base)
		r.Details = macDetails(base)
	case c.info.BaseDigits > 0:
		r.Base = padDigits(base, c.info.BaseDigits)
	default:
		r.Base = strconv.FormatInt(base, 10)
	}
	return r
}

// macDetails ilk oktetteki bayrakları okunur hale getirir.
//
//	bit 0 (I/G): 0 = tekil (unicast), 1 = grup (multicast) — Parse multicast'i zaten reddeder
//	bit 1 (U/L): 0 = evrensel (IEEE'nin verdiği OUI), 1 = yerel yönetilen (sanal makine, konteyner...)
func macDetails(base int64) string {
	first := base >> 40
	scope := "evrensel (IEEE OUI)"
	if first&2 != 0 {
		scope = "yerel yönetilen (U/L biti 1)"
	}
	return fmt.Sprintf("OUI %02X:%02X:%02X · %s · unicast", first, (base>>32)&0xFF, (base>>24)&0xFF, scope)
}

// GTINCheckDigit GS1 mod-10 algoritması (EAN-8/13, UPC, ISBN-13 ortak).
// Sağdan başlayarak gövde hanelerine sırayla 3,1,3,1... ağırlıkları uygulanır.
func GTINCheckDigit(body string) int {
	sum := 0
	weight := 3
	for i := len(body) - 1; i >= 0; i-- {
		sum += int(body[i]-'0') * weight
		weight = 4 - weight
	}
	return (10 - sum%10) % 10
}

// normalizeBody ayraçları atar; onaltılık tiplerde harfleri büyütür.
func (c Codec) normalizeBody(s string) string {
	s = strings.TrimSpace(s)
	if c.info.Radix == 16 {
		return strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "", " ", "", "\t", "").Replace(s))
	}
	return strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(s)
}

func (c Codec) bodyChars(s string) bool {
	if c.info.Radix == 16 {
		return allHex(s)
	}
	return allDigits(s)
}

func (c Codec) charsLabel() string {
	if c.info.Radix == 16 {
		return "onaltılık karakterlerden (0-9, A-F)"
	}
	return "rakamlardan"
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func allHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) && (s[i] < 'A' || s[i] > 'F') {
			return false
		}
	}
	return true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func padDigits(v int64, width int) string { return fmt.Sprintf("%0*d", width, v) }

func mustInt(s string, radix int) int64 {
	v, err := strconv.ParseInt(s, radix, 64)
	if err != nil {
		panic(err)
	}
	return v
}

func ipow(base int64, n int) int64 {
	v := int64(1)
	for range n {
		v *= base
	}
	return v
}
