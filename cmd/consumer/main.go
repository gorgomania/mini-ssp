package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/segmentio/kafka-go"
)

const (
	batchSize     = 1000
	flushInterval = 5 * time.Second
)

func main() {
	kafkaAddr := flag.String("kafka", "localhost:9092", "comma-separated Kafka brokers")
	topic := flag.String("topic", "auction.events", "Kafka topic")
	chAddr := flag.String("clickhouse", "localhost:9000", "ClickHouse native address")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{*chAddr},
		Auth: clickhouse.Auth{Database: "default"},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    2,
	})
	if err != nil {
		slog.Error("clickhouse open failed", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := conn.Ping(context.Background()); err != nil {
		slog.Error("clickhouse ping failed", "err", err)
		os.Exit(1)
	}
	slog.Info("clickhouse connected", "addr", *chAddr)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(*kafkaAddr, ","),
		Topic:   *topic,
		GroupID: "clickhouse-consumer",
	})
	defer reader.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("consumer started", "kafka", *kafkaAddr, "topic", *topic)

	eventCh := make(chan events.AuctionEvent, batchSize)

	go func() {
		for {
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("kafka fetch failed", "err", err)
				continue
			}
			var e events.AuctionEvent
			if err := json.Unmarshal(msg.Value, &e); err != nil {
				slog.Warn("decode failed", "err", err, "raw", string(msg.Value))
			} else {
				eventCh <- e
			}
			reader.CommitMessages(ctx, msg) //nolint:errcheck
		}
	}()

	var batch []events.AuctionEvent
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		const maxAttempts = 3
		for attempt := range maxAttempts {
			if err := insert(ctx, conn, batch); err != nil {
				slog.Warn("clickhouse insert failed", "attempt", attempt+1, "rows", len(batch), "err", err)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			slog.Info("flushed", "rows", len(batch))
			batch = batch[:0]
			return
		}
		slog.Error("clickhouse insert failed after retries, dropping batch", "rows", len(batch))
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		case e := <-eventCh:
			batch = append(batch, e)
			if len(batch) >= batchSize {
				flush()
			}
		}
	}
}

func insert(ctx context.Context, conn clickhouse.Conn, batch []events.AuctionEvent) error {
	b, err := conn.PrepareBatch(ctx,
		"INSERT INTO auction_events (ts, user_id, geo, format, advertiser_id, clearing_price)")
	if err != nil {
		return err
	}
	for _, e := range batch {
		if err := b.Append(e.Timestamp, e.UserID, e.Geo, e.Format, e.AdvertiserID, e.ClearingPrice); err != nil {
			return err
		}
	}
	return b.Send()
}
