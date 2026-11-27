// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// txAttempts bir transaction'ın en fazla kaç kez deneneceği.
const txAttempts = 5

// txRetries toplam tekrar deneme sayısı (testler kilitlenmenin önlendiğini, yalnızca
// tekrar denemeyle örtülmediğini doğrulamak için okur).
var txRetries atomic.Int64

// inTx fn'i bir transaction içinde çalıştırır. PostgreSQL işlemi kilitlenme (deadlock)
// ya da serileştirme çakışması yüzünden iptal ederse, kısa ve rastgele bir beklemeden
// sonra baştan dener. Birden çok sunucu örneği aynı satırlar için yarıştığında bu hatalar
// kalıcı değildir; kullanıcıya 500 yerine başarılı sonuç döner.
//
// fn yan etkisini yalnızca transaction'a yazmalı ve dış değişkenleri her denemede
// baştan kurmalıdır: geri alınan denemenin bıraktığı hiçbir şey kalmamalı.
func (s *Store) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	for attempt := 1; ; attempt++ {
		err := pgx.BeginFunc(ctx, s.db, fn)
		if err == nil || attempt == txAttempts || !retryable(err) {
			return err
		}
		txRetries.Add(1)
		wait := time.Duration(attempt*attempt*5+rand.IntN(25)) * time.Millisecond
		slog.Info("transaction tekrar deneniyor", "deneme", attempt, "sebep", pgCode(err), "bekleme", wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// retryable: 40P01 deadlock_detected, 40001 serialization_failure.
func retryable(err error) bool {
	code := pgCode(err)
	return code == "40P01" || code == "40001"
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}
