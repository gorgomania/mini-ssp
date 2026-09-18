package main

import (
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/server"
	pb "github.com/gorgomania/mini-ssp/proto"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dsps := []dsp.DSP{
		dsp.AdCorp{},
		dsp.MediaNet{},
		dsp.QuickAds{},
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAuctionServer(grpcServer, server.New(dsps))
	reflection.Register(grpcServer)

	slog.Info("auction server listening", "port", 50051)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
