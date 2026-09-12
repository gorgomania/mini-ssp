package main

import (
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/ssp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web
var webFS embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dspAddrs := flag.String("dsps", "localhost:50051,localhost:50052,localhost:50053", "comma-separated DSP gRPC addresses")
	port := flag.String("port", "8080", "HTTP listen port")
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

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", ssp.BidHandler(dsps))
	http.Handle("/metrics", promhttp.Handler())
	slog.Info("SSP HTTP listening", "port", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
