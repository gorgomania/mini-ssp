package dsp

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/gorgomania/mini-ssp/proto"
)

type GRPCClient struct {
	addr   string
	client pb.AuctionClient
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &GRPCClient{addr: addr, client: pb.NewAuctionClient(conn)}, nil
}

func (c *GRPCClient) Bid(geo, format string, floor float64) (Bid, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	resp, err := c.client.RunAuction(ctx, &pb.BidRequest{Geo: geo, Format: format, FloorPrice: floor})
	if err != nil {
		log.Printf("DSP %s error: %v", c.addr, err)
		return Bid{}, false
	}
	return Bid{AdvertiserID: resp.AdvertiserId, Price: resp.Price}, true
}
