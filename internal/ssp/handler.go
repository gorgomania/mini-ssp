package ssp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorgomania/mini-ssp/internal/auction"
	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/metrics"
)

type BidRequest struct {
	Geo        string  `json:"geo"`
	Format     string  `json:"format"`
	FloorPrice float64 `json:"floor_price"`
}

type BidResponse struct {
	AdvertiserID string  `json:"advertiser_id"`
	Price        float64 `json:"price"`
}

func BidHandler(dsps []dsp.DSP) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var req BidRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var mu sync.Mutex
		var bids []dsp.Bid
		var wg sync.WaitGroup
		wg.Add(len(dsps))
		for _, d := range dsps {
			go func() {
				defer wg.Done()
				if b, ok := d.Bid(req.Geo, req.Format, req.FloorPrice); ok {
					mu.Lock()
					bids = append(bids, b)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		metrics.AuctionDuration.Observe(time.Since(start).Seconds())

		winner, clearingPrice, ok := auction.SecondPrice(bids, req.FloorPrice)
		if !ok {
			slog.Info("no bids", "geo", req.Geo, "format", req.Format)
			metrics.AuctionsTotal.WithLabelValues(req.Geo, req.Format, "no_bid").Inc()
			http.Error(w, "no bids", http.StatusNoContent)
			return
		}

		metrics.AuctionsTotal.WithLabelValues(req.Geo, req.Format, "win").Inc()
		metrics.ClearingPrice.WithLabelValues(req.Geo, req.Format).Observe(clearingPrice)
		metrics.WinnerBids.WithLabelValues(winner.AdvertiserID).Inc()

		slog.Info("auction", "geo", req.Geo, "format", req.Format,
			"winner", winner.AdvertiserID, "bid", winner.Price, "clearing", clearingPrice)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BidResponse{
			AdvertiserID: winner.AdvertiserID,
			Price:        clearingPrice,
		})
	}
}
