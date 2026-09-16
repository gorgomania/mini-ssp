package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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
	conn *pgx.Conn
}

func New(ctx context.Context, dsn string) (*Store, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.conn.Close(ctx)
}

func (s *Store) ActiveDSPs(ctx context.Context) ([]DSPConfig, error) {
	rows, err := s.conn.Query(ctx,
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
	rows, err := s.conn.Query(ctx,
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
