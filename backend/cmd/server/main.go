// Copyright 2026 Mehmet ERGULCU and the abarow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kaynak/internal/api"
	"kaynak/internal/db"
	"kaynak/internal/store"
	"kaynak/internal/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("sunucu durdu", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := env("DATABASE_URL", "postgres://kaynak:kaynak@localhost:5433/kaynak?sslmode=disable")
	addr := env("HTTP_ADDR", ":8088")
	sweepEvery, err := time.ParseDuration(env("SWEEP_INTERVAL", "30s"))
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	st := store.New(pool)
	go sweepReservations(ctx, st, sweepEvery)

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, web.FS()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("dinleniyor", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("kapanıyor")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}

// sweepReservations süresi dolan rezervasyonları düzenli aralıklarla boşa düşürür.
// Birden çok sunucu örneği çalışsa da güvenlidir (FOR UPDATE SKIP LOCKED).
func sweepReservations(ctx context.Context, st *store.Store, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		n, err := st.ExpireReservations(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("rezervasyon temizliği başarısız", "err", err)
		} else if n > 0 {
			slog.Info("süresi dolan rezervasyonlar bırakıldı", "adet", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
