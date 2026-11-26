package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"kaynak/internal/kinds"
)

// CustomKind kullanıcı tanımlı kod tipi.
type CustomKind struct {
	ID          int64        `json:"id"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	BodyDigits  int          `json:"body_digits"`
	CheckDigits int          `json:"check_digits"`
	Rules       []kinds.Rule `json:"rules"`
	CheckExpr   string       `json:"check_expr"`
	Density     float64      `json:"density"`
	Example     string       `json:"example"`
	Pools       int64        `json:"pools"` // bu tipi kullanan havuz sayısı
	CreatedAt   time.Time    `json:"created_at"`
}

type CustomKindInput struct {
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	BodyDigits  int          `json:"body_digits"`
	CheckDigits int          `json:"check_digits"`
	Rules       []kinds.Rule `json:"rules"`
	CheckExpr   string       `json:"check_expr"`
}

func (in *CustomKindInput) def(id int64) kinds.SchemeDef {
	rules := in.Rules
	if rules == nil {
		rules = []kinds.Rule{}
	}
	return kinds.SchemeDef{ID: id, Code: in.Code, Name: in.Name, BodyDigits: in.BodyDigits,
		CheckDigits: in.CheckDigits, Rules: rules, CheckExpr: in.CheckExpr}
}

const customKindSelect = `SELECT ck.id, ck.code, ck.name, ck.description, ck.body_digits, ck.check_digits, ck.rules,
	ck.check_expr, ck.density, ck.example, ck.created_at,
	(SELECT count(*) FROM pools p WHERE p.custom_kind_id = ck.id)
	FROM custom_kinds ck`

func scanCustomKind(row pgx.Row) (*CustomKind, error) {
	var k CustomKind
	var rules []byte
	if err := row.Scan(&k.ID, &k.Code, &k.Name, &k.Description, &k.BodyDigits, &k.CheckDigits, &rules,
		&k.CheckExpr, &k.Density, &k.Example, &k.CreatedAt, &k.Pools); err != nil {
		return nil, err
	}
	return &k, json.Unmarshal(rules, &k.Rules)
}

func (k *CustomKind) scheme() (*kinds.Scheme, error) {
	return schemeFor(kinds.SchemeDef{ID: k.ID, Code: k.Code, Name: k.Name, BodyDigits: k.BodyDigits,
		CheckDigits: k.CheckDigits, Rules: k.Rules, CheckExpr: k.CheckExpr})
}

func (s *Store) ListCustomKinds(ctx context.Context) ([]CustomKind, error) {
	rows, err := s.db.Query(ctx, customKindSelect+` ORDER BY ck.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomKind{}
	for rows.Next() {
		k, err := scanCustomKind(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// customScheme özel tipi kodundan bulup derlenmiş hâlini döndürür.
func (s *Store) customScheme(ctx context.Context, code string) (*kinds.Scheme, error) {
	code = strings.TrimSpace(strings.ToLower(code))
	if code == "" {
		return nil, invalidf("kind = custom için custom_kind (özel tip kodu) zorunlu")
	}
	k, err := scanCustomKind(s.db.QueryRow(ctx, customKindSelect+` WHERE ck.code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFoundf("%q kodlu özel tip bulunamadı", code)
	}
	if err != nil {
		return nil, err
	}
	return k.scheme()
}

// CustomCodec doğrulama aracı için özel tipin codec'i.
func (s *Store) CustomCodec(ctx context.Context, code string) (kinds.Codec, error) {
	sc, err := s.customScheme(ctx, code)
	if err != nil {
		return kinds.Codec{}, err
	}
	return kinds.NewCustomCodec(sc), nil
}

func validateCustomKindInput(in *CustomKindInput) error {
	in.Code = strings.TrimSpace(strings.ToLower(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	if !codePattern.MatchString(in.Code) || !strings.ContainsAny(in.Code, "abcdefghijklmnopqrstuvwxyz") {
		return invalidf("kod 2-63 karakter olmalı, en az bir harf içermeli; küçük harf, rakam, - ve _ kullanılabilir")
	}
	if in.Name == "" {
		return invalidf("ad zorunlu")
	}
	return nil
}

func (s *Store) CreateCustomKind(ctx context.Context, in CustomKindInput) (*CustomKind, error) {
	if err := validateCustomKindInput(&in); err != nil {
		return nil, err
	}
	sc, err := kinds.CompileScheme(in.def(0))
	if err != nil {
		return nil, fromKindsErr(err)
	}
	rules, _ := json.Marshal(sc.Rules)
	var id int64
	err = s.db.QueryRow(ctx, `INSERT INTO custom_kinds
			(code, name, description, body_digits, check_digits, rules, check_expr, density, example)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		in.Code, in.Name, in.Description, sc.BodyDigits, sc.CheckDigits, rules, sc.CheckExpr, sc.Density, sc.Example).Scan(&id)
	if isUniqueViolation(err, "custom_kinds_code_key") {
		return nil, conflictf("duplicate_code", "%q kodlu bir özel tip zaten var", in.Code)
	}
	if err != nil {
		return nil, err
	}
	return scanCustomKind(s.db.QueryRow(ctx, customKindSelect+` WHERE ck.id = $1`, id))
}

// DeleteCustomKind yalnızca hiçbir havuzun kullanmadığı tipi siler.
func (s *Store) DeleteCustomKind(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM custom_kinds ck WHERE ck.id = $1
		AND NOT EXISTS (SELECT 1 FROM pools p WHERE p.custom_kind_id = ck.id)`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM custom_kinds WHERE id = $1)`, id).Scan(&exists)
		if !exists {
			return notFoundf("özel tip bulunamadı")
		}
		return conflictf("kind_in_use", "bu tipi kullanan havuz var; önce havuzları silin")
	}
	schemeCache.Delete(id)
	return nil
}

// CustomKindTest kaydetmeden tanımı dener.
type CustomKindTest struct {
	OK       bool                    `json:"ok"`
	Error    string                  `json:"error,omitempty"`
	Density  float64                 `json:"density,omitempty"`
	Examples []string                `json:"examples,omitempty"`
	Value    *kinds.ValidationResult `json:"value,omitempty"`
}

func TestCustomKind(in CustomKindInput, value string) CustomKindTest {
	sc, err := kinds.CompileScheme(in.def(0))
	if err != nil {
		return CustomKindTest{Error: strings.TrimPrefix(err.Error(), kinds.ErrInvalid.Error()+": ")}
	}
	res := CustomKindTest{OK: true, Density: sc.Density, Examples: sc.Samples(6)}
	if strings.TrimSpace(value) != "" {
		v := kinds.NewCustomCodec(sc).Validate(value)
		res.Value = &v
	}
	return res
}

// KindInfos yerleşik tiplerin ve özel tiplerin birleşik listesi (havuz oluşturma ve doğrulama için).
func (s *Store) KindInfos(ctx context.Context) ([]kinds.Info, error) {
	out := kinds.All()
	customs, err := s.ListCustomKinds(ctx)
	if err != nil {
		return nil, err
	}
	for i := range customs {
		sc, err := customs[i].scheme()
		if err != nil {
			return nil, err
		}
		info := kinds.NewCustomCodec(sc).Info()
		if customs[i].Description != "" {
			info.Description = customs[i].Description + " — " + info.Description
		}
		out = append(out, info)
	}
	return out, nil
}
