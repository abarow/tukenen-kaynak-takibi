// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kaynak/internal/kinds"
)

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

const poolColumns = `p.id, p.code, p.name, p.description, p.kind, p.prefix, p.pad_width,
	p.reuse_policy, p.cooldown_days, p.reservation_ttl_seconds, p.low_watermark, p.active,
	p.created_at, p.updated_at,
	p.custom_kind_id, ck.code, ck.name, ck.body_digits, ck.check_digits, ck.rules, ck.check_expr`

const poolFrom = ` FROM pools p LEFT JOIN custom_kinds ck ON ck.id = p.custom_kind_id`

// poolRow bir havuz satırını ve (varsa) özel tip tanımını taşır.
type poolRow struct {
	p       Pool
	ckID    *int64
	ckCode  *string
	ckName  *string
	ckBody  *int
	ckCheck *int
	ckRules []byte
	ckExpr  *string
}

func (r *poolRow) targets() []any {
	p := &r.p
	return []any{&p.ID, &p.Code, &p.Name, &p.Description, &p.Kind, &p.Prefix, &p.PadWidth,
		&p.ReusePolicy, &p.CooldownDays, &p.ReservationTTLSeconds, &p.LowWatermark, &p.Active,
		&p.CreatedAt, &p.UpdatedAt,
		&r.ckID, &r.ckCode, &r.ckName, &r.ckBody, &r.ckCheck, &r.ckRules, &r.ckExpr}
}

func (r *poolRow) finish() (*Pool, error) {
	p := &r.p
	if info, ok := kinds.Lookup(string(p.Kind)); ok {
		p.KindName = info.Name
	}
	if r.ckID != nil {
		def := kinds.SchemeDef{ID: *r.ckID, Code: *r.ckCode, Name: *r.ckName,
			BodyDigits: *r.ckBody, CheckDigits: *r.ckCheck, CheckExpr: *r.ckExpr}
		if err := json.Unmarshal(r.ckRules, &def.Rules); err != nil {
			return nil, err
		}
		s, err := schemeFor(def)
		if err != nil {
			return nil, err
		}
		p.scheme, p.CustomKind, p.KindName = s, r.ckCode, *r.ckName
	}
	return p, nil
}

// Özel tip tanımları değişmez; derlenmiş hâlleri kimliğe göre önbelleklenir.
var schemeCache sync.Map // int64 → *kinds.Scheme

func schemeFor(def kinds.SchemeDef) (*kinds.Scheme, error) {
	if s, ok := schemeCache.Load(def.ID); ok {
		return s.(*kinds.Scheme), nil
	}
	s, err := kinds.CompileScheme(def)
	if err != nil {
		return nil, fmt.Errorf("özel tip %q derlenemedi: %w", def.Code, err)
	}
	actual, _ := schemeCache.LoadOrStore(def.ID, s)
	return actual.(*kinds.Scheme), nil
}

// Kapasite = aralıklardaki geçerli değerlerin toplamı. Boşta = kapasite - "dağıtılamaz" satırlar.
// Hiç dokunulmamış değerler satır olarak bulunmadığı için sayım ucuzdur.
const poolStatsJoin = `
LEFT JOIN LATERAL (
	SELECT COALESCE(sum(COALESCE(r.valid_count, r.end_value - r.start_value + 1)), 0)::bigint AS capacity,
		COALESCE(bool_and(r.valid_count_exact), true) AS capacity_exact
	FROM pool_ranges r WHERE r.pool_id = p.id
) cap ON true
LEFT JOIN LATERAL (
	SELECT
		count(*) FILTER (WHERE a.status = 'assigned') AS assigned,
		count(*) FILTER (WHERE a.status = 'reserved') AS reserved,
		count(*) FILTER (WHERE a.status = 'released' AND (a.reusable_at IS NULL OR a.reusable_at > now())) AS released,
		count(*) FILTER (WHERE a.status = 'released' AND a.reusable_at <= now()) AS reusable,
		count(*) FILTER (WHERE a.status = 'blocked') AS blocked
	FROM allocations a WHERE a.pool_id = p.id
) st ON true`

const poolSelect = `SELECT ` + poolColumns + `,
	cap.capacity, cap.capacity_exact, st.assigned, st.reserved, st.released, st.reusable, st.blocked` +
	poolFrom + poolStatsJoin

func scanPool(row pgx.Row) (*Pool, error) {
	var r poolRow
	st := &r.p.Stats
	err := row.Scan(append(r.targets(),
		&st.Capacity, &st.CapacityExact, &st.Assigned, &st.Reserved, &st.Released, &st.Reusable, &st.Blocked)...)
	if err != nil {
		return nil, err
	}
	p, err := r.finish()
	if err != nil {
		return nil, err
	}
	st = &p.Stats
	st.Available = st.Capacity - st.Assigned - st.Reserved - st.Released - st.Blocked
	st.Low = p.LowWatermark > 0 && st.Available <= p.LowWatermark
	return p, nil
}

func (s *Store) ListPools(ctx context.Context) ([]Pool, error) {
	rows, err := s.db.Query(ctx, poolSelect+` ORDER BY p.active DESC, p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pool{}
	for rows.Next() {
		p, err := scanPool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) GetPool(ctx context.Context, id int64) (*Pool, error) {
	p, err := scanPool(s.db.QueryRow(ctx, poolSelect+` WHERE p.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFoundf("havuz bulunamadı")
	}
	if err != nil {
		return nil, err
	}
	p.Ranges, err = s.listRanges(ctx, s.db, p)
	return p, err
}

// ResolvePool sayısal kimliği veya havuz kodunu kimliğe çevirir.
func (s *Store) ResolvePool(ctx context.Context, key string) (int64, error) {
	if id, err := strconv.ParseInt(key, 10, 64); err == nil {
		return id, nil
	}
	var id int64
	err := s.db.QueryRow(ctx, `SELECT id FROM pools WHERE code = $1`, strings.ToLower(key)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, notFoundf("%q kodlu havuz bulunamadı", key)
	}
	return id, err
}

// getPoolTx istatistiksiz, sadece ayarları okur (tahsis yolunda hızlı olsun diye).
func getPoolTx(ctx context.Context, q pgx.Tx, id int64) (*Pool, error) {
	var r poolRow
	err := q.QueryRow(ctx, `SELECT `+poolColumns+poolFrom+` WHERE p.id = $1`, id).Scan(r.targets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFoundf("havuz bulunamadı")
	}
	if err != nil {
		return nil, err
	}
	return r.finish()
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *Store) listRanges(ctx context.Context, q querier, p *Pool) ([]Range, error) {
	rows, err := q.Query(ctx, `SELECT id, pool_id, start_value, end_value, next_value, created_at,
			COALESCE(valid_count, end_value - start_value + 1), valid_count_exact
		FROM pool_ranges WHERE pool_id = $1 ORDER BY start_value`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codec := p.Codec()
	out := []Range{}
	for rows.Next() {
		var r Range
		if err := rows.Scan(&r.ID, &r.PoolID, &r.Start, &r.End, &r.Next, &r.CreatedAt, &r.Valid, &r.ValidExact); err != nil {
			return nil, err
		}
		r.Size = r.End - r.Start + 1
		r.Untouched = r.End - r.Next + 1
		r.FirstValue = codec.RangeLabel(r.Start)
		r.LastValue = codec.RangeLabel(r.End)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- Oluşturma / güncelleme -------------------------------------------------

type RangeInput struct {
	Start  *int64 `json:"start,omitempty"`
	End    *int64 `json:"end,omitempty"`
	Prefix string `json:"prefix,omitempty"` // sabit uzunluklu tiplerde: önekten aralık hesapla
}

type CreatePoolInput struct {
	Code                  string       `json:"code"`
	Name                  string       `json:"name"`
	Description           string       `json:"description"`
	Kind                  kinds.Kind   `json:"kind"`
	CustomKind            string       `json:"custom_kind"` // kind = custom ise özel tipin kodu
	Prefix                string       `json:"prefix"`
	PadWidth              int          `json:"pad_width"`
	ReusePolicy           string       `json:"reuse_policy"`
	CooldownDays          int          `json:"cooldown_days"`
	ReservationTTLSeconds int          `json:"reservation_ttl_seconds"`
	LowWatermark          int64        `json:"low_watermark"`
	Ranges                []RangeInput `json:"ranges"`
}

type UpdatePoolInput struct {
	Name                  *string `json:"name"`
	Description           *string `json:"description"`
	ReusePolicy           *string `json:"reuse_policy"`
	CooldownDays          *int    `json:"cooldown_days"`
	ReservationTTLSeconds *int    `json:"reservation_ttl_seconds"`
	LowWatermark          *int64  `json:"low_watermark"`
	Active                *bool   `json:"active"`
}

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}$`)

func validatePolicy(policy string, cooldown, ttl int, low int64) error {
	switch policy {
	case ReuseNever, ReuseCooldown, ReuseImmediate:
	default:
		return invalidf("yeniden kullanım politikası never, cooldown veya immediate olmalı")
	}
	if cooldown < 0 || cooldown > 36500 {
		return invalidf("karantina süresi 0-36500 gün arasında olmalı")
	}
	if policy == ReuseCooldown && cooldown == 0 {
		return invalidf("cooldown politikasında karantina süresi 0'dan büyük olmalı")
	}
	if ttl < 1 || ttl > 30*24*3600 {
		return invalidf("rezervasyon süresi 1 saniye ile 30 gün arasında olmalı")
	}
	if low < 0 {
		return invalidf("uyarı eşiği negatif olamaz")
	}
	return nil
}

func (s *Store) CreatePool(ctx context.Context, in CreatePoolInput) (*Pool, error) {
	in.Code = strings.TrimSpace(strings.ToLower(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	if !codePattern.MatchString(in.Code) || !strings.ContainsAny(in.Code, "abcdefghijklmnopqrstuvwxyz") {
		return nil, invalidf("kod 2-63 karakter olmalı, en az bir harf içermeli; küçük harf, rakam, - ve _ kullanılabilir")
	}
	if in.Name == "" {
		return nil, invalidf("ad zorunlu")
	}
	var codec kinds.Codec
	var customKindID *int64
	if in.Kind == kinds.Custom {
		if in.Prefix != "" || in.PadWidth != 0 {
			return nil, invalidf("önek ve dolgu yalnızca sıra numarası tipinde kullanılabilir")
		}
		sc, err := s.customScheme(ctx, in.CustomKind)
		if err != nil {
			return nil, err
		}
		codec, customKindID = kinds.NewCustomCodec(sc), &sc.ID
	} else {
		if in.CustomKind != "" {
			return nil, invalidf("custom_kind yalnızca kind = custom ile kullanılabilir")
		}
		c, err := kinds.NewCodec(in.Kind, in.Prefix, in.PadWidth)
		if err != nil {
			return nil, fromKindsErr(err)
		}
		codec = c
	}
	if in.ReusePolicy == "" {
		in.ReusePolicy = codec.Info().DefaultPolicy
	}
	if in.ReservationTTLSeconds == 0 {
		in.ReservationTTLSeconds = 900
	}
	if err := validatePolicy(in.ReusePolicy, in.CooldownDays, in.ReservationTTLSeconds, in.LowWatermark); err != nil {
		return nil, err
	}

	var id int64
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO pools
			(code, name, description, kind, prefix, pad_width, reuse_policy, cooldown_days, reservation_ttl_seconds,
			 low_watermark, custom_kind_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			in.Code, in.Name, in.Description, in.Kind, in.Prefix, in.PadWidth,
			in.ReusePolicy, in.CooldownDays, in.ReservationTTLSeconds, in.LowWatermark, customKindID).Scan(&id)
		if isUniqueViolation(err, "pools_code_key") {
			return conflictf("duplicate_code", "%q kodlu bir havuz zaten var", in.Code)
		}
		if err != nil {
			return err
		}
		for _, r := range in.Ranges {
			if _, err := addRangeTx(ctx, tx, id, codec, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetPool(ctx, id)
}

func (s *Store) UpdatePool(ctx context.Context, id int64, in UpdatePoolInput) (*Pool, error) {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		p, err := getPoolTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.Name != nil {
			if strings.TrimSpace(*in.Name) == "" {
				return invalidf("ad boş olamaz")
			}
			p.Name = strings.TrimSpace(*in.Name)
		}
		if in.Description != nil {
			p.Description = *in.Description
		}
		if in.ReusePolicy != nil {
			p.ReusePolicy = *in.ReusePolicy
		}
		if in.CooldownDays != nil {
			p.CooldownDays = *in.CooldownDays
		}
		if in.ReservationTTLSeconds != nil {
			p.ReservationTTLSeconds = *in.ReservationTTLSeconds
		}
		if in.LowWatermark != nil {
			p.LowWatermark = *in.LowWatermark
		}
		if in.Active != nil {
			p.Active = *in.Active
		}
		if err := validatePolicy(p.ReusePolicy, p.CooldownDays, p.ReservationTTLSeconds, p.LowWatermark); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE pools SET name=$2, description=$3, reuse_policy=$4, cooldown_days=$5,
			reservation_ttl_seconds=$6, low_watermark=$7, active=$8, updated_at=now() WHERE id=$1`,
			id, p.Name, p.Description, p.ReusePolicy, p.CooldownDays, p.ReservationTTLSeconds, p.LowWatermark, p.Active)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetPool(ctx, id)
}

// DeletePool yalnızca hiç tahsis yapılmamış havuzu siler; geçmişi olan havuz pasife alınmalı.
func (s *Store) DeletePool(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM pools p WHERE p.id = $1
		AND NOT EXISTS (SELECT 1 FROM allocations a WHERE a.pool_id = p.id)`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pools WHERE id = $1)`, id).Scan(&exists)
		if !exists {
			return notFoundf("havuz bulunamadı")
		}
		return conflictf("pool_in_use", "bu havuzdan tahsis yapılmış; silmek yerine pasife alın")
	}
	return nil
}

// ---- Aralıklar --------------------------------------------------------------

func (s *Store) AddRange(ctx context.Context, poolID int64, in RangeInput) (*Pool, error) {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		p, err := getPoolTx(ctx, tx, poolID)
		if err != nil {
			return err
		}
		_, err = addRangeTx(ctx, tx, poolID, p.Codec(), in)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetPool(ctx, poolID)
}

func addRangeTx(ctx context.Context, tx pgx.Tx, poolID int64, codec kinds.Codec, in RangeInput) (int64, error) {
	var start, end int64
	switch {
	case in.Prefix != "":
		var err error
		if start, end, err = codec.PrefixRange(in.Prefix); err != nil {
			return 0, fromKindsErr(err)
		}
	case in.Start != nil && in.End != nil:
		start, end = *in.Start, *in.End
		if err := codec.ValidateRange(start, end); err != nil {
			return 0, fromKindsErr(err)
		}
	default:
		return 0, invalidf("aralık için ya önek ya da başlangıç+bitiş verilmeli")
	}

	// Kurallı tiplerde aralıktaki her gövde geçerli değil: kapasite için geçerlileri say.
	var validCount *int64
	exact := true
	if codec.Filtered() {
		n, ex, err := codec.CountValid(start, end)
		if err != nil {
			return 0, fromKindsErr(err)
		}
		if n == 0 {
			return 0, invalidf("%s – %s aralığında kurallara uyan hiç değer yok", codec.RangeLabel(start), codec.RangeLabel(end))
		}
		validCount, exact = &n, ex
	}

	ns := codec.Namespace()
	var id int64
	// Savepoint: çakışma hatası dış transaction'ı bozmasın, anlamlı mesaj üretebilelim.
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		return sp.QueryRow(ctx, `INSERT INTO pool_ranges
				(pool_id, namespace, start_value, end_value, next_value, valid_count, valid_count_exact)
			VALUES ($1, $2, $3, $4, $3, $5, $6) RETURNING id`, poolID, ns, start, end, validCount, exact).Scan(&id)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" { // exclusion_violation
		var code string
		var cs, ce int64
		_ = tx.QueryRow(ctx, `SELECT p.code, r.start_value, r.end_value FROM pool_ranges r JOIN pools p ON p.id = r.pool_id
			WHERE r.namespace = $1 AND int8range(r.start_value, r.end_value, '[]') && int8range($2, $3, '[]')
			ORDER BY r.start_value LIMIT 1`, ns, start, end).Scan(&code, &cs, &ce)
		return 0, conflictf("range_overlap", "aralık %q havuzundaki %s – %s aralığıyla çakışıyor",
			code, codec.Format(cs), codec.Format(ce))
	}
	return id, err
}

// DeleteRange yalnızca hiç kullanılmamış aralığı siler.
func (s *Store) DeleteRange(ctx context.Context, poolID, rangeID int64) (*Pool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM pool_ranges r WHERE r.id = $1 AND r.pool_id = $2
		AND r.next_value = r.start_value
		AND NOT EXISTS (SELECT 1 FROM allocations a WHERE a.range_id = r.id)`, rangeID, poolID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pool_ranges WHERE id = $1 AND pool_id = $2)`, rangeID, poolID).Scan(&exists)
		if !exists {
			return nil, notFoundf("aralık bulunamadı")
		}
		return nil, conflictf("range_in_use", "bu aralıktan değer dağıtılmış; silinemez")
	}
	return s.GetPool(ctx, poolID)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && (constraint == "" || pgErr.ConstraintName == constraint)
}
