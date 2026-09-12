package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"net"

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
	price := s.basePrice + rand.Float64()*s.basePrice*0.5
	if s.boostGeo != "" && req.Geo == s.boostGeo {
		price = s.boostPrice + rand.Float64()*s.boostPrice*0.5
	}
	log.Printf("bid geo=%s format=%s price=%.3f", req.Geo, req.Format, price)
	return &pb.BidResponse{AdvertiserId: s.name, Price: price}, nil
}

func main() {
	name := flag.String("name", "dsp", "advertiser ID")
	port := flag.String("port", "50051", "gRPC listen port")
	basePrice := flag.Float64("base-price", 0.5, "base bid price")
	boostGeo := flag.String("boost-geo", "", "geo to apply boost price")
	boostPrice := flag.Float64("boost-price", 2.0, "boosted bid price")
	flag.Parse()

	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterAuctionServer(s, &dspServer{
		name:       *name,
		basePrice:  *basePrice,
		boostGeo:   *boostGeo,
		boostPrice: *boostPrice,
	})
	reflection.Register(s)

	log.Printf("DSP %q on :%s (base=%.2f boost_geo=%s boost=%.2f)",
		*name, *port, *basePrice, *boostGeo, *boostPrice)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
