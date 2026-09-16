package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DSPConfig struct {
	Name     string
	GRPCAddr string
}

type FreqCapRule struct {
	AdvertiserID string
	Limit        int64
	Window       time.Duration
}

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close(_ context.Context) {
	s.pool.Close()
}

func (s *Store) ActiveDSPs(ctx context.Context) ([]DSPConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name, grpc_addr FROM dsps WHERE active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DSPConfig
	for rows.Next() {
		var d DSPConfig
		if err := rows.Scan(&d.Name, &d.GRPCAddr); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) FreqCapRules(ctx context.Context) ([]FreqCapRule, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT advertiser_id, cap_limit, window_secs FROM freqcap_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FreqCapRule
	for rows.Next() {
		var r FreqCapRule
		var windowSecs int64
		if err := rows.Scan(&r.AdvertiserID, &r.Limit, &windowSecs); err != nil {
			return nil, err
		}
		r.Window = time.Duration(windowSecs) * time.Second
		result = append(result, r)
	}
	return result, rows.Err()
}
