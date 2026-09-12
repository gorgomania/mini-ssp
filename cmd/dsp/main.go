package main

import (
	"context"
	"flag"
	"log/slog"
	"math/rand"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/gorgomania/mini-ssp/proto"
)

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
	price := req.FloorPrice + premium
	slog.Info("bid", "geo", req.Geo, "format", req.Format, "price", price)
	return &pb.BidResponse{AdvertiserId: s.name, Price: price}, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	name := flag.String("name", "dsp", "advertiser ID")
	port := flag.String("port", "50051", "gRPC listen port")
	basePrice := flag.Float64("base-price", 0.5, "base bid price")
	boostGeo := flag.String("boost-geo", "", "geo to apply boost price")
	boostPrice := flag.Float64("boost-price", 2.0, "boosted bid price")
	flag.Parse()

	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}

	s := grpc.NewServer()
	pb.RegisterAuctionServer(s, &dspServer{
		name:       *name,
		basePrice:  *basePrice,
		boostGeo:   *boostGeo,
		boostPrice: *boostPrice,
	})
	reflection.Register(s)

	slog.Info("DSP started", "name", *name, "port", *port, "boost_geo", *boostGeo)
	if err := s.Serve(lis); err != nil {
		slog.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
