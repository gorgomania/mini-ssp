package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
	"github.com/gorgomania/mini-ssp/internal/ssp"
	"github.com/gorgomania/mini-ssp/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web
var webFS embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dspAddrs  := flag.String("dsps", "", "comma-separated DSP gRPC addresses (overridden by -postgres)")
	port      := flag.String("port", "8080", "HTTP listen port")
	redisAddr := flag.String("redis", "", "Redis address for frequency capping (empty = disabled)")
	capLimit  := flag.Int64("freqcap-limit", 3, "default max impressions per user per advertiser per window")
	capWindow := flag.Duration("freqcap-window", time.Hour, "default frequency cap time window")
	kafkaBrokers := flag.String("kafka", "", "comma-separated Kafka brokers (empty = disabled)")
	kafkaTopic   := flag.String("kafka-topic", "auction.events", "Kafka topic for auction events")
	postgresDSN  := flag.String("postgres", "", "PostgreSQL DSN for DSP config and freqcap rules")
	flag.Parse()

	ctx := context.Background()

	// ── DSP list ─────────────────────────────────────────────────────────────
	var dspAddressList []string
	if *postgresDSN != "" {
		st, err := store.New(ctx, *postgresDSN)
		if err != nil {
			slog.Error("postgres connect failed", "err", err)
			os.Exit(1)
		}
		defer st.Close(ctx)

		configs, err := st.ActiveDSPs(ctx)
		if err != nil {
			slog.Error("load DSPs from postgres failed", "err", err)
			os.Exit(1)
		}
		for _, c := range configs {
			dspAddressList = append(dspAddressList, c.GRPCAddr)
			slog.Info("registered DSP", "name", c.Name, "addr", c.GRPCAddr)
		}

		if *redisAddr != "" {
			rules, err := st.FreqCapRules(ctx)
			if err != nil {
				slog.Error("load freqcap rules from postgres failed", "err", err)
				os.Exit(1)
			}
			ruleMap := make(map[string]freqcap.Rule, len(rules))
			for _, r := range rules {
				ruleMap[r.AdvertiserID] = freqcap.Rule{Limit: r.Limit, Window: r.Window}
			}
			slog.Info("loaded per-advertiser freqcap rules", "count", len(ruleMap))

			rc, err := freqcap.NewRedisCapper(*redisAddr, *capLimit, *capWindow)
			if err != nil {
				slog.Error("Redis connect failed", "err", err)
				os.Exit(1)
			}
			rc.SetRules(ruleMap)
			serve(ctx, dspAddressList, rc, *kafkaBrokers, *kafkaTopic, *port)
			return
		}
	} else {
		for addr := range strings.SplitSeq(*dspAddrs, ",") {
			if addr = strings.TrimSpace(addr); addr != "" {
				dspAddressList = append(dspAddressList, addr)
				slog.Info("registered DSP", "addr", addr)
			}
		}
	}

	// ── Capper (global rules fallback) ────────────────────────────────────────
	var capper freqcap.Capper = freqcap.NoopCapper{}
	if *redisAddr != "" {
		rc, err := freqcap.NewRedisCapper(*redisAddr, *capLimit, *capWindow)
		if err != nil {
			slog.Error("Redis connect failed", "err", err)
			os.Exit(1)
		}
		slog.Info("frequency capping enabled (global rules)", "limit", *capLimit, "window", *capWindow)
		capper = rc
	}

	serve(ctx, dspAddressList, capper, *kafkaBrokers, *kafkaTopic, *port)
}

func serve(_ context.Context, addrs []string, capper freqcap.Capper, kafkaBrokers, kafkaTopic, port string) {
	var dsps []dsp.DSP
	for _, addr := range addrs {
		c, err := dsp.NewGRPCClient(addr)
		if err != nil {
			slog.Error("connect DSP failed", "addr", addr, "err", err)
			os.Exit(1)
		}
		dsps = append(dsps, c)
	}

	var pub events.Publisher = events.NoopPublisher{}
	if kafkaBrokers != "" {
		kp := events.NewKafkaPublisher(strings.Split(kafkaBrokers, ","), kafkaTopic)
		defer kp.Close()
		pub = kp
		slog.Info("kafka publishing enabled", "brokers", kafkaBrokers, "topic", kafkaTopic)
	}

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", ssp.BidHandler(dsps, capper, pub))
	http.Handle("/metrics", promhttp.Handler())
	slog.Info("SSP HTTP listening", "port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
