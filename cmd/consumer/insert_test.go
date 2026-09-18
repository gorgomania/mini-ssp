package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startClickHouse(t *testing.T) clickhouse.Conn {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:24",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor:   wait.ForListeningPort("9000/tcp").WithStartupTimeout(90 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:         []string{fmt.Sprintf("%s:%s", host, port.Port())},
		Auth:         clickhouse.Auth{Database: "default"},
		DialTimeout:  10 * time.Second,
		MaxOpenConns: 2,
	})
	if err != nil {
		t.Fatalf("clickhouse open: %v", err)
	}

	if err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS default.auction_events (
			ts             DateTime,
			user_id        String,
			geo            LowCardinality(String),
			format         LowCardinality(String),
			advertiser_id  LowCardinality(String),
			clearing_price Float64
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (ts, geo, format, advertiser_id)
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	return conn
}

func TestInsert_WritesRows(t *testing.T) {
	conn := startClickHouse(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	batch := []events.AuctionEvent{
		{Timestamp: now, UserID: "u1", Geo: "US", Format: "banner", AdvertiserID: "adcorp", ClearingPrice: 1.5},
		{Timestamp: now, UserID: "u2", Geo: "EU", Format: "video", AdvertiserID: "medianet", ClearingPrice: 2.0},
	}

	if err := insert(ctx, conn, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var count uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM auction_events").Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != uint64(len(batch)) {
		t.Fatalf("expected %d rows, got %d", len(batch), count)
	}
}

func TestInsert_EmptyBatch(t *testing.T) {
	conn := startClickHouse(t)
	if err := insert(context.Background(), conn, nil); err != nil {
		t.Fatalf("insert empty batch: %v", err)
	}
}
