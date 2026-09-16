package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/gorgomania/mini-ssp/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("ssp"),
		postgres.WithUsername("ssp"),
		postgres.WithPassword("ssp"),
		postgres.WithInitScripts("../../postgres/init.sql"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { c.Terminate(ctx) })

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

func TestStore_ActiveDSPs(t *testing.T) {
	ctx := context.Background()
	st, err := store.New(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close(ctx)

	dsps, err := st.ActiveDSPs(ctx)
	if err != nil {
		t.Fatalf("ActiveDSPs: %v", err)
	}
	if len(dsps) == 0 {
		t.Fatal("expected seed DSPs, got none")
	}

	names := make(map[string]string)
	for _, d := range dsps {
		if d.Name == "" || d.GRPCAddr == "" {
			t.Errorf("DSP has empty fields: %+v", d)
		}
		names[d.Name] = d.GRPCAddr
	}

	for _, name := range []string{"adcorp", "apacads", "latamads", "medianet", "quickads", "ruads"} {
		if _, ok := names[name]; !ok {
			t.Errorf("missing DSP %q in results", name)
		}
	}
}

func TestStore_ActiveDSPs_RespectsActiveFlag(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dsps SET active = FALSE WHERE name = 'quickads'`); err != nil {
		t.Fatalf("deactivate DSP: %v", err)
	}
	pool.Close()

	st, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close(ctx)

	dsps, err := st.ActiveDSPs(ctx)
	if err != nil {
		t.Fatalf("ActiveDSPs: %v", err)
	}
	for _, d := range dsps {
		if d.Name == "quickads" {
			t.Fatal("inactive DSP 'quickads' should not be returned")
		}
	}
}

func TestStore_FreqCapRules(t *testing.T) {
	ctx := context.Background()
	st, err := store.New(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close(ctx)

	rules, err := st.FreqCapRules(ctx)
	if err != nil {
		t.Fatalf("FreqCapRules: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected seed freqcap rules, got none")
	}

	byAdv := make(map[string]store.FreqCapRule)
	for _, r := range rules {
		if r.Limit <= 0 {
			t.Errorf("rule %q has non-positive limit: %d", r.AdvertiserID, r.Limit)
		}
		if r.Window <= 0 {
			t.Errorf("rule %q has non-positive window: %v", r.AdvertiserID, r.Window)
		}
		byAdv[r.AdvertiserID] = r
	}

	qr, ok := byAdv["quickads"]
	if !ok {
		t.Fatal("missing freqcap rule for 'quickads'")
	}
	if qr.Limit != 3 {
		t.Errorf("quickads limit: want 3, got %d", qr.Limit)
	}
	if qr.Window != 30*time.Minute {
		t.Errorf("quickads window: want 30m, got %v", qr.Window)
	}
}
