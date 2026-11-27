// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

package kinds

// Kullanıcı tanımlı kod tipleri.
//
// Bir özel tip; gövde uzunluğu, gövdenin sağlaması gereken kurallar ve gövdeden
// hesaplanan kontrol kısmından oluşur. Kurallar ve kontrol formülü küçük ifadeler
// olarak yazılır (github.com/expr-lang/expr; dosya/ağ erişimi yok, bellek sınırlı):
//
//	gövde 8 hane, kontrol 4 hane
//	kural 1: digitSum(1, 3) % 2 == 0    ilk 3 hanenin toplamı çift
//	kural 2: digitSum(4, 8) % 3 == 0    sonraki 5 hanenin toplamı 3'e bölünür
//	kontrol: sum(chunks(1, 8, 2))       12+34+56+78 → son 4 hane: 0180
//
// Konumlar 1'den başlar ve gövdeye göredir. Fonksiyon adları İngilizcedir
// (digit, digitSum, number, chunks, weightedSum); expr'in kendi fonksiyonlarıyla
// (sum, map, filter...) aynı camelCase düzenini izler.

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

const Custom Kind = "custom"

// Rule gövdenin sağlaması gereken bir koşul.
type Rule struct {
	Expr    string `json:"expr"`
	Message string `json:"message"`
}

// SchemeDef bir özel tipin saklanan tanımı.
type SchemeDef struct {
	ID          int64  `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	BodyDigits  int    `json:"body_digits"`
	CheckDigits int    `json:"check_digits"`
	Rules       []Rule `json:"rules"`
	CheckExpr   string `json:"check_expr"`
}

// Scheme derlenmiş, kullanıma hazır tanım. Eşzamanlı kullanım için güvenlidir.
type Scheme struct {
	SchemeDef
	Density float64 // örneklemle ölçülen geçerli gövde oranı
	Example string
	evals   sync.Pool
}

// Fonksiyon başvurusu: arayüzde ve hata mesajlarında gösterilir.
const FunctionHelp = `digit(i) → i. hane · digitSum(a, b) → a..b hanelerinin toplamı · number(a, b) → a..b hanelerinin oluşturduğu sayı · ` +
	`chunks(a, b, n) → a..b hanelerini n haneli sayılara böler · weightedSum(a, b, [3, 1]) → hanelerin ağırlıklı toplamı (ağırlıklar soldan döner) · ` +
	`ayrıca sum, map, filter, all, any, count, reduce, len, abs, max, min ve % + - * operatörleri`

var allowedBuiltins = []string{"all", "none", "any", "one", "filter", "map", "find", "count", "sum", "reduce",
	"len", "abs", "max", "min", "int", "first", "last", "take", "reverse"}

// evaluator tek bir goroutine'e aittir: derlenmiş programlar kendi hane tamponuna bağlıdır.
type evaluator struct {
	d     []int
	rules []*vm.Program
	check *vm.Program
	vm    vm.VM
}

func (s *Scheme) compileInto(ev *evaluator) error {
	n := s.BodyDigits
	ev.d = make([]int, n)
	pos := func(i int) error {
		if i < 1 || i > n {
			return fmt.Errorf("konum %d gövdenin dışında (1..%d)", i, n)
		}
		return nil
	}
	span := func(a, b int) error {
		if err := pos(a); err != nil {
			return err
		}
		if err := pos(b); err != nil {
			return err
		}
		if a > b {
			return fmt.Errorf("başlangıç %d bitiş %d'den büyük", a, b)
		}
		return nil
	}
	opts := []expr.Option{
		expr.Env(struct{}{}),
		expr.DisableAllBuiltins(),
		expr.MaxNodes(400),
		expr.Function("digit", func(p ...any) (any, error) {
			i := p[0].(int)
			if err := pos(i); err != nil {
				return nil, err
			}
			return ev.d[i-1], nil
		}, new(func(int) int)),
		expr.Function("digitSum", func(p ...any) (any, error) {
			a, b := p[0].(int), p[1].(int)
			if err := span(a, b); err != nil {
				return nil, err
			}
			t := 0
			for _, x := range ev.d[a-1 : b] {
				t += x
			}
			return t, nil
		}, new(func(int, int) int)),
		expr.Function("number", func(p ...any) (any, error) {
			a, b := p[0].(int), p[1].(int)
			if err := span(a, b); err != nil {
				return nil, err
			}
			v := 0
			for _, x := range ev.d[a-1 : b] {
				v = v*10 + x
			}
			return v, nil
		}, new(func(int, int) int)),
		expr.Function("chunks", func(p ...any) (any, error) {
			a, b, g := p[0].(int), p[1].(int), p[2].(int)
			if err := span(a, b); err != nil {
				return nil, err
			}
			if g < 1 || (b-a+1)%g != 0 {
				return nil, fmt.Errorf("%d..%d arası (%d hane) %d haneli gruplara tam bölünmüyor", a, b, b-a+1, g)
			}
			out := make([]any, 0, (b-a+1)/g)
			for i := a - 1; i < b; i += g {
				v := 0
				for _, x := range ev.d[i : i+g] {
					v = v*10 + x
				}
				out = append(out, v)
			}
			return out, nil
		}, new(func(int, int, int) []any)),
		expr.Function("weightedSum", func(p ...any) (any, error) {
			a, b := p[0].(int), p[1].(int)
			if err := span(a, b); err != nil {
				return nil, err
			}
			ws, ok := p[2].([]any)
			if !ok || len(ws) == 0 {
				return nil, fmt.Errorf("ağırlıklar bir sayı listesi olmalı, ör. [3, 1]")
			}
			t := 0
			for k, x := range ev.d[a-1 : b] {
				w, ok := ws[k%len(ws)].(int)
				if !ok {
					return nil, fmt.Errorf("ağırlıklar tam sayı olmalı")
				}
				t += x * w
			}
			return t, nil
		}, new(func(int, int, []any) int)),
	}
	for _, b := range allowedBuiltins {
		opts = append(opts, expr.EnableBuiltin(b))
	}

	ev.rules = ev.rules[:0]
	for i, r := range s.Rules {
		prog, err := expr.Compile(r.Expr, append(opts, expr.AsBool())...)
		if err != nil {
			return fmt.Errorf("kural %d (%s): %s", i+1, r.Expr, firstLine(err))
		}
		ev.rules = append(ev.rules, prog)
	}
	if s.CheckDigits > 0 {
		prog, err := expr.Compile(s.CheckExpr, append(opts, expr.AsInt())...)
		if err != nil {
			return fmt.Errorf("kontrol formülü (%s): %s", s.CheckExpr, firstLine(err))
		}
		ev.check = prog
	}
	return nil
}

var exprPosition = regexp.MustCompile(`\s*\(\d+:\d+\)$`)

// firstLine expr hatalarının çok satırlı konum gösterimini atıp tek satırlık mesaj bırakır.
func firstLine(err error) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return exprPosition.ReplaceAllString(msg, "")
}

// CompileScheme tanımı doğrular, derler ve rastgele gövdelerle sınar: çalışma
// hatası veren ya da hiçbir değeri kabul etmeyen tanımlar reddedilir.
func CompileScheme(def SchemeDef) (*Scheme, error) {
	if def.BodyDigits < 1 || def.BodyDigits > 15 {
		return nil, invalid("gövde 1-15 hane olmalı")
	}
	if def.CheckDigits < 0 || def.CheckDigits > 6 {
		return nil, invalid("kontrol kısmı 0-6 hane olmalı")
	}
	if def.BodyDigits+def.CheckDigits > 18 {
		return nil, invalid("toplam uzunluk 18 haneyi aşamaz")
	}
	if len(def.Rules) > 20 {
		return nil, invalid("en fazla 20 kural tanımlanabilir")
	}
	for i := range def.Rules {
		def.Rules[i].Expr = strings.TrimSpace(def.Rules[i].Expr)
		def.Rules[i].Message = strings.TrimSpace(def.Rules[i].Message)
		if def.Rules[i].Expr == "" || len(def.Rules[i].Expr) > 500 || len(def.Rules[i].Message) > 200 {
			return nil, invalid("kural %d boş olamaz; formül en fazla 500, mesaj en fazla 200 karakter", i+1)
		}
	}
	def.CheckExpr = strings.TrimSpace(def.CheckExpr)
	if def.CheckDigits > 0 && def.CheckExpr == "" {
		return nil, invalid("kontrol kısmı için bir formül gerekli")
	}
	if def.CheckDigits == 0 && def.CheckExpr != "" {
		return nil, invalid("kontrol formülü için kontrol hane sayısı 0'dan büyük olmalı")
	}

	s := &Scheme{SchemeDef: def}
	first := &evaluator{}
	if err := s.compileInto(first); err != nil {
		return nil, invalid("%s", err.Error())
	}

	// Rastgele gövdelerle sına: çalışma zamanı hatalarını şimdi yakala, yoğunluğu ölç.
	// 100.000 örnek ~30 ms; binde 2 gibi seyrek kurallarda bile ~200 isabetle makul tahmin verir.
	const samples = 100_000
	rng := rand.New(rand.NewPCG(uint64(def.BodyDigits), 42))
	maxBase := ipow(10, def.BodyDigits)
	valid := 0
	for range samples {
		base := rng.Int64N(maxBase)
		ok, failed, err := first.accepts(s, base)
		if err != nil {
			return nil, invalid("kural %d, gövde %s için hata verdi: %s", failed+1, padDigits(base, def.BodyDigits), firstLine(err))
		}
		if !ok {
			continue
		}
		if _, err := first.checkPart(s, base); err != nil {
			return nil, invalid("kontrol formülü, gövde %s için hata verdi: %s", padDigits(base, def.BodyDigits), firstLine(err))
		}
		if valid == 0 {
			c, _ := first.checkPart(s, base)
			s.Example = padDigits(base, def.BodyDigits) + c
		}
		valid++
	}
	if valid == 0 {
		return nil, invalid("kurallar %d rastgele gövdenin hiçbirini kabul etmedi; tanım kullanılamaz", samples)
	}
	s.Density = float64(valid) / samples
	s.evals.Put(first)
	return s, nil
}

func (s *Scheme) get() *evaluator {
	if ev, ok := s.evals.Get().(*evaluator); ok {
		return ev
	}
	ev := &evaluator{}
	if err := s.compileInto(ev); err != nil {
		panic(err) // CompileScheme aynı tanımı zaten derledi
	}
	return ev
}

func (s *Scheme) put(ev *evaluator) { s.evals.Put(ev) }

func (ev *evaluator) load(base int64) {
	for i := len(ev.d) - 1; i >= 0; i-- {
		ev.d[i] = int(base % 10)
		base /= 10
	}
}

// accepts kuralları sırayla dener; başarısız olan kuralın sırasını döndürür.
func (ev *evaluator) accepts(s *Scheme, base int64) (bool, int, error) {
	ev.load(base)
	return ev.acceptsLoaded()
}

func (ev *evaluator) acceptsLoaded() (bool, int, error) {
	for i, p := range ev.rules {
		out, err := ev.vm.Run(p, nil)
		if err != nil {
			return false, i, err
		}
		if out != true {
			return false, i, nil
		}
	}
	return true, -1, nil
}

// checkPart kontrol formülünün sonucunun son k hanesini sıfır dolgulu döndürür.
func (ev *evaluator) checkPart(s *Scheme, base int64) (string, error) {
	if s.CheckDigits == 0 {
		return "", nil
	}
	ev.load(base)
	out, err := ev.vm.Run(ev.check, nil)
	if err != nil {
		return "", err
	}
	v, ok := out.(int)
	if !ok {
		return "", fmt.Errorf("kontrol formülü tam sayı döndürmeli")
	}
	if v < 0 {
		return "", fmt.Errorf("kontrol formülü negatif sonuç verdi (%d)", v)
	}
	return padDigits(int64(v)%ipow(10, s.CheckDigits), s.CheckDigits), nil
}

// Samples rastgele seçilmiş, kurallara uyan n örnek kod üretir (tanım denerken gösterilir).
func (s *Scheme) Samples(n int) []string {
	ev := s.get()
	defer s.put(ev)
	rng := rand.New(rand.NewPCG(7, uint64(s.BodyDigits)))
	maxBase := ipow(10, s.BodyDigits)
	var out []string
	for tries := 0; len(out) < n && tries < 200_000; tries++ {
		base := rng.Int64N(maxBase)
		if ok, _, err := ev.accepts(s, base); err != nil || !ok {
			continue
		}
		c, err := ev.checkPart(s, base)
		if err != nil {
			continue
		}
		out = append(out, padDigits(base, s.BodyDigits)+c)
	}
	return out
}

// ---- Codec entegrasyonu ------------------------------------------------------

// NewCustomCodec özel tip için codec kurar.
func NewCustomCodec(s *Scheme) Codec {
	return Codec{Kind: Custom, scheme: s, info: Info{
		Code: Custom, Name: s.Name, BaseDigits: s.BodyDigits, CheckDigits: s.CheckDigits, Radix: 10,
		Example: s.Example, DefaultPolicy: "never", CustomKind: s.Code, Density: s.Density,
		Description: s.describe(),
	}}
}

func (s *Scheme) describe() string {
	parts := []string{fmt.Sprintf("%d hane gövde + %d hane kontrol", s.BodyDigits, s.CheckDigits)}
	for _, r := range s.Rules {
		if r.Message != "" {
			parts = append(parts, r.Message)
		} else {
			parts = append(parts, r.Expr)
		}
	}
	if s.CheckDigits > 0 {
		parts = append(parts, "kontrol = son "+strconv.Itoa(s.CheckDigits)+" hane: "+s.CheckExpr)
	}
	return strings.Join(parts, " · ")
}

func (s *Scheme) ruleError(idx int, err error) error {
	if err != nil {
		return invalid("kural %d çalıştırılamadı: %s", idx+1, firstLine(err))
	}
	r := s.Rules[idx]
	if r.Message != "" {
		return invalid("kural %d sağlanmıyor: %s", idx+1, r.Message)
	}
	return invalid("kural %d sağlanmıyor: %s", idx+1, r.Expr)
}

func (s *Scheme) format(base int64) string {
	ev := s.get()
	defer s.put(ev)
	c, err := ev.checkPart(s, base)
	if err != nil {
		c = strings.Repeat("?", s.CheckDigits)
	}
	return padDigits(base, s.BodyDigits) + c
}

func (s *Scheme) parse(c Codec, value string) (int64, error) {
	v := c.normalizeBody(value)
	total := s.BodyDigits + s.CheckDigits
	if !allDigits(v) {
		return 0, invalid("yalnızca rakam içermeli")
	}
	if len(v) != total {
		return 0, invalid("%d hane olmalı, %d hane girildi", total, len(v))
	}
	base := mustInt(v[:s.BodyDigits], 10)
	ev := s.get()
	defer s.put(ev)
	ok, idx, err := ev.accepts(s, base)
	if !ok {
		return 0, s.ruleError(idx, err)
	}
	want, err := ev.checkPart(s, base)
	if err != nil {
		return 0, invalid("kontrol formülü çalıştırılamadı: %s", err)
	}
	if v[s.BodyDigits:] != want {
		return 0, invalid("kontrol kısmı hatalı, doğrusu %s olmalı", want)
	}
	return base, nil
}

// ErrScanLimit imleç, kurallara uyan değer ararken tarama sınırına takıldı.
var ErrScanLimit = fmt.Errorf("%w: kurallara uyan değer ararken tarama sınırına ulaşıldı; aralığı önekle daraltın", ErrInvalid)

// FirstValid [from, to] içinde kurallara uyan ilk gövdeyi bulur.
func (c Codec) FirstValid(from, to int64, limit int) (int64, bool, error) {
	if c.scheme == nil || len(c.scheme.Rules) == 0 {
		return from, from <= to, nil
	}
	s := c.scheme
	ev := s.get()
	defer s.put(ev)
	ev.load(from)
	for v, n := from, 0; v <= to; v, n = v+1, n+1 {
		if n >= limit {
			return 0, false, ErrScanLimit
		}
		ok, idx, err := ev.acceptsLoaded()
		if err != nil {
			return 0, false, s.ruleError(idx, err)
		}
		if ok {
			return v, true, nil
		}
		ev.increment()
	}
	return 0, false, nil
}

// increment tampondaki gövdeyi bir artırır (sayaç gibi); her adımda sayıyı yeniden bölmekten hızlı.
func (ev *evaluator) increment() {
	for i := len(ev.d) - 1; i >= 0; i-- {
		if ev.d[i] < 9 {
			ev.d[i]++
			return
		}
		ev.d[i] = 0
	}
}

// RangeLabel aralık sınırlarını göstermek için: kurallı tiplerde sınır gövdesi geçerli bir
// kod olmayabileceğinden yalnızca gövde, kontrol kısmı yerine nokta gösterilir.
func (c Codec) RangeLabel(base int64) string {
	if c.Filtered() {
		return padDigits(base, c.scheme.BodyDigits) + strings.Repeat("·", c.scheme.CheckDigits)
	}
	return c.Format(base)
}

// Filtered kuralları olan (her gövdenin geçerli olmadığı) bir tip mi?
func (c Codec) Filtered() bool { return c.scheme != nil && len(c.scheme.Rules) > 0 }

// CountExactLimit bu boyuta kadar aralıklar tek tek sayılır; üstü örneklemle tahmin edilir.
const CountExactLimit = 1_000_000

// CountValid aralıktaki geçerli gövde sayısını verir. Büyük aralıklarda
// 100.000 örnekle tahmin eder ve exact=false döner.
func (c Codec) CountValid(start, end int64) (count int64, exact bool, err error) {
	size := end - start + 1
	if !c.Filtered() {
		return size, true, nil
	}
	s := c.scheme
	ev := s.get()
	defer s.put(ev)
	if size <= CountExactLimit {
		ev.load(start)
		for v := start; v <= end; v++ {
			ok, idx, err := ev.acceptsLoaded()
			if err != nil {
				return 0, false, s.ruleError(idx, err)
			}
			if ok {
				count++
			}
			ev.increment()
		}
		return count, true, nil
	}
	const samples = 100_000
	rng := rand.New(rand.NewPCG(uint64(start), uint64(end)))
	hits := 0
	for range samples {
		ok, idx, err := ev.accepts(s, start+rng.Int64N(size))
		if err != nil {
			return 0, false, s.ruleError(idx, err)
		}
		if ok {
			hits++
		}
	}
	return int64(float64(size) * float64(hits) / samples), false, nil
}
