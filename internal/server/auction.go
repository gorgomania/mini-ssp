package server

import (
	"context"
	"log"

	"github.com/gorgomania/mini-ssp/internal/auction"
	"github.com/gorgomania/mini-ssp/internal/dsp"
	pb "github.com/gorgomania/mini-ssp/proto"
)

type AuctionServer struct {
	pb.UnimplementedAuctionServer
	dsps []dsp.DSP
}

func New(dsps []dsp.DSP) *AuctionServer {
	return &AuctionServer{dsps: dsps}
}

func (s *AuctionServer) RunAuction(ctx context.Context, req *pb.BidRequest) (*pb.BidResponse, error) {
	var bids []dsp.Bid
	for _, d := range s.dsps {
		if b, ok := d.Bid(req.Geo, req.Format); ok {
			bids = append(bids, b)
		}
	}

	winner, clearingPrice, ok := auction.SecondPrice(bids, 0)
	if !ok {
		log.Printf("no bids geo=%s format=%s", req.Geo, req.Format)
		return &pb.BidResponse{}, nil
	}

	log.Printf("auction geo=%s format=%s winner=%s bid=%.3f clearing=%.3f",
		req.Geo, req.Format, winner.AdvertiserID, winner.Price, clearingPrice)

	return &pb.BidResponse{
		AdvertiserId: winner.AdvertiserID,
		Price:        clearingPrice,
	}, nil
}
