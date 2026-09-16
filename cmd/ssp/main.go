package main

import (
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web
var webFS embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dspAddrs := flag.String("dsps", "localhost:50051,localhost:50052,localhost:50053", "comma-separated DSP gRPC addresses")
	port := flag.String("port", "8080", "HTTP listen port")
	redisAddr := flag.String("redis", "", "Redis address for frequency capping (empty = disabled)")
	capLimit := flag.Int64("freqcap-limit", 3, "max impressions per user per advertiser per window")
	capWindow := flag.Duration("freqcap-window", time.Hour, "frequency cap time window")
	kafkaBrokers := flag.String("kafka", "", "comma-separated Kafka brokers (empty = disabled)")
	kafkaTopic := flag.String("kafka-topic", "auction.events", "Kafka topic for auction events")
	flag.Parse()

	var dsps []dsp.DSP
	for addr := range strings.SplitSeq(*dspAddrs, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		c, err := dsp.NewGRPCClient(addr)
		if err != nil {
			slog.Error("connect DSP failed", "addr", addr, "err", err)
			os.Exit(1)
		}
		slog.Info("registered DSP", "addr", addr)
		dsps = append(dsps, c)
	}

	var capper freqcap.Capper = freqcap.NoopCapper{}
	if *redisAddr != "" {
		rc, err := freqcap.NewRedisCapper(*redisAddr, *capLimit, *capWindow)
		if err != nil {
			slog.Error("Redis connect failed", "addr", *redisAddr, "err", err)
			os.Exit(1)
		}
		slog.Info("frequency capping enabled", "redis", *redisAddr, "limit", *capLimit, "window", *capWindow)
		capper = rc
	}

	var pub events.Publisher = events.NoopPublisher{}
	if *kafkaBrokers != "" {
		brokers := strings.Split(*kafkaBrokers, ",")
		kp := events.NewKafkaPublisher(brokers, *kafkaTopic)
		defer kp.Close()
		pub = kp
		slog.Info("kafka publishing enabled", "brokers", *kafkaBrokers, "topic", *kafkaTopic)
	}

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", ssp.BidHandler(dsps, capper, pub))
	http.Handle("/metrics", promhttp.Handler())
	slog.Info("SSP HTTP listening", "port", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
