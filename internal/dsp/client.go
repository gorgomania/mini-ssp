package dsp

import (
	"context"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/gorgomania/mini-ssp/proto"
)

type GRPCClient struct {
	addr    string
	client  pb.AuctionClient
	breaker *gobreaker.CircuitBreaker[*pb.BidResponse]
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	cb := gobreaker.NewCircuitBreaker[*pb.BidResponse](gobreaker.Settings{
		Name:        addr,
		MaxRequests: 1,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("DSP circuit breaker", "dsp", name, "from", from, "to", to)
		},
	})
	return &GRPCClient{addr: addr, client: pb.NewAuctionClient(conn), breaker: cb}, nil
}

func (c *GRPCClient) Bid(ctx context.Context, geo, format string, floor float64) (Bid, bool) {
	resp, err := c.breaker.Execute(func() (*pb.BidResponse, error) {
		return c.client.RunAuction(ctx, &pb.BidRequest{Geo: geo, Format: format, FloorPrice: floor})
	})
	if err != nil {
		slog.Error("DSP call failed", "addr", c.addr, "err", err)
		return Bid{}, false
	}
	return Bid{AdvertiserID: resp.AdvertiserId, Price: resp.Price}, true
}
