package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
	"github.com/gorgomania/mini-ssp/internal/middleware"
	"github.com/gorgomania/mini-ssp/internal/ssp"
	"github.com/gorgomania/mini-ssp/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "go.uber.org/automaxprocs"
)

//go:embed web
var webFS embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	dspAddrsFlag := flag.String("dsps", "", "comma-separated DSP gRPC addresses (overridden by -postgres)")
	port         := flag.String("port", "8080", "HTTP listen port")
	redisAddr    := flag.String("redis", "", "Redis address for frequency capping (empty = disabled)")
	capLimit     := flag.Int64("freqcap-limit", 3, "default max impressions per user per advertiser per window")
	capWindow    := flag.Duration("freqcap-window", time.Hour, "default frequency cap time window")
	kafkaBrokers := flag.String("kafka", "", "comma-separated Kafka brokers (empty = disabled)")
	kafkaTopic   := flag.String("kafka-topic", "auction.events", "Kafka topic for auction events")
	postgresDSN  := flag.String("postgres", "", "PostgreSQL DSN for DSP config and freqcap rules")
	rateLimit    := flag.Float64("rate-limit", 0, "max requests/sec on /bid (0 = disabled)")
	rateBurst    := flag.Int("rate-burst", 10, "burst size for rate limiter")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Step 1: DSP addresses ─────────────────────────────────────────────────
	var dspAddrs []string
	var freqCapRules map[string]freqcap.Rule

	if *postgresDSN != "" {
		st, err := store.New(ctx, *postgresDSN)
		if err != nil {
			return fmt.Errorf("postgres connect: %w", err)
		}
		defer st.Close(ctx)

		configs, err := st.ActiveDSPs(ctx)
		if err != nil {
			return fmt.Errorf("load DSPs: %w", err)
		}
		for _, c := range configs {
			dspAddrs = append(dspAddrs, c.GRPCAddr)
			slog.Info("registered DSP", "name", c.Name, "addr", c.GRPCAddr)
		}

		if *redisAddr != "" {
			rules, err := st.FreqCapRules(ctx)
			if err != nil {
				return fmt.Errorf("load freqcap rules: %w", err)
			}
			freqCapRules = make(map[string]freqcap.Rule, len(rules))
			for _, r := range rules {
				freqCapRules[r.AdvertiserID] = freqcap.Rule{Limit: r.Limit, Window: r.Window}
			}
			slog.Info("loaded per-advertiser freqcap rules", "count", len(freqCapRules))
		}
	} else {
		for addr := range strings.SplitSeq(*dspAddrsFlag, ",") {
			if addr = strings.TrimSpace(addr); addr != "" {
				dspAddrs = append(dspAddrs, addr)
				slog.Info("registered DSP", "addr", addr)
			}
		}
	}

	// ── Step 2: frequency capper ──────────────────────────────────────────────
	var capper freqcap.Capper = freqcap.NoopCapper{}
	if *redisAddr != "" {
		rc, err := freqcap.NewRedisCapper(*redisAddr, *capLimit, *capWindow)
		if err != nil {
			return fmt.Errorf("redis connect: %w", err)
		}
		if len(freqCapRules) > 0 {
			rc.SetRules(freqCapRules)
			slog.Info("frequency capping enabled (per-advertiser rules)")
		} else {
			slog.Info("frequency capping enabled (global rules)", "limit", *capLimit, "window", *capWindow)
		}
		capper = rc
	}

	// ── Step 3: Kafka publisher ───────────────────────────────────────────────
	var pub events.Publisher = events.NoopPublisher{}
	if *kafkaBrokers != "" {
		kp := events.NewKafkaPublisher(strings.Split(*kafkaBrokers, ","), *kafkaTopic)
		defer kp.Close()
		pub = kp
		slog.Info("kafka publishing enabled", "brokers", *kafkaBrokers, "topic", *kafkaTopic)
	}

	// ── Step 4: connect DSPs ──────────────────────────────────────────────────
	var dsps []dsp.DSP
	for _, addr := range dspAddrs {
		c, err := dsp.NewGRPCClient(addr)
		if err != nil {
			return fmt.Errorf("connect DSP %s: %w", addr, err)
		}
		dsps = append(dsps, c)
	}

	// ── Step 5: HTTP server with graceful shutdown ────────────────────────────
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))

	bidHandler := http.Handler(http.HandlerFunc(ssp.BidHandler(dsps, capper, pub)))
	if *rateLimit > 0 {
		bidHandler = middleware.RateLimit(*rateLimit, *rateBurst)(bidHandler)
		slog.Info("rate limiting enabled", "rps", *rateLimit, "burst", *rateBurst)
	}
	mux.Handle("/bid", bidHandler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Addr: ":" + *port, Handler: mux}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Warn("shutdown error", "err", err)
		}
	}()

	slog.Info("SSP HTTP listening", "port", *port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
