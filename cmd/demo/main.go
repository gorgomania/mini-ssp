package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
	"github.com/gorgomania/mini-ssp/internal/ssp"
	pb "github.com/gorgomania/mini-ssp/proto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web
var webFS embed.FS

// ── DSP gRPC server ──────────────────────────────────────────────────────────

type dspServer struct {
	pb.UnimplementedAuctionServer
	name       string
	basePrice  float64
	boostGeo   string
	boostPrice float64
}

func (s *dspServer) RunAuction(_ context.Context, req *pb.BidRequest) (*pb.BidResponse, error) {
	premium := s.basePrice + rand.Float64()*s.basePrice*0.5
	if s.boostGeo != "" && req.Geo == s.boostGeo {
		premium = s.boostPrice + rand.Float64()*s.boostPrice*0.5
	}
	return &pb.BidResponse{AdvertiserId: s.name, Price: req.FloorPrice + premium}, nil
}

func startDSP(name, addr string, basePrice float64, boostGeo string, boostPrice float64, ready chan<- struct{}) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("DSP listen failed", "name", name, "err", err)
		os.Exit(1)
	}
	s := grpc.NewServer()
	pb.RegisterAuctionServer(s, &dspServer{
		name: name, basePrice: basePrice, boostGeo: boostGeo, boostPrice: boostPrice,
	})
	reflection.Register(s)
	slog.Info("DSP started", "name", name, "addr", addr)
	ready <- struct{}{}
	if err := s.Serve(lis); err != nil {
		slog.Error("DSP serve failed", "name", name, "err", err)
		os.Exit(1)
	}
}

// ── main ─────────────────────────────────────────────────────────────────────

var dspConfigs = []struct {
	name       string
	addr       string
	basePrice  float64
	boostGeo   string
	boostPrice float64
}{
	{"adcorp", ":50051", 0.5, "US", 2.0},
	{"medianet", ":50052", 0.4, "EU", 1.8},
	{"quickads", ":50053", 1.0, "", 0},
	{"ruads", ":50054", 0.3, "RU", 1.2},
	{"apacads", ":50055", 0.3, "JP", 1.5},
	{"latamads", ":50056", 0.25, "BR", 1.0},
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ready := make(chan struct{}, len(dspConfigs))
	for _, cfg := range dspConfigs {
		go startDSP(cfg.name, cfg.addr, cfg.basePrice, cfg.boostGeo, cfg.boostPrice, ready)
	}
	for range dspConfigs {
		<-ready
	}

	var dsps []dsp.DSP
	for _, cfg := range dspConfigs {
		c, err := dsp.NewGRPCClient("localhost" + cfg.addr)
		if err != nil {
			slog.Error("connect DSP failed", "name", cfg.name, "err", err)
			os.Exit(1)
		}
		dsps = append(dsps, c)
	}

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", ssp.BidHandler(dsps, freqcap.NoopCapper{}, events.NoopPublisher{}))
	http.Handle("/metrics", promhttp.Handler())

	slog.Info("SSP demo", "url", "http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
