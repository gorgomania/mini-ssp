package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/server"
	pb "github.com/gorgomania/mini-ssp/proto"
)

func main() {
	dsps := []dsp.DSP{
		dsp.AdCorp{},
		dsp.MediaNet{},
		dsp.QuickAds{},
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAuctionServer(grpcServer, server.New(dsps))
	reflection.Register(grpcServer)

	log.Println("auction server listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
