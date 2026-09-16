package ssp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorgomania/mini-ssp/internal/auction"
	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/events"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
	"github.com/gorgomania/mini-ssp/internal/metrics"
)

type BidRequest struct {
	UserID     string  `json:"user_id"`
	Geo        string  `json:"geo"`
	Format     string  `json:"format"`
	FloorPrice float64 `json:"floor_price"`
}

type BidResponse struct {
	AdvertiserID string  `json:"advertiser_id"`
	Price        float64 `json:"price"`
}

func BidHandler(dsps []dsp.DSP, capper freqcap.Capper, pub events.Publisher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var req BidRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		bidCtx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
		defer cancel()

		var mu sync.Mutex
		var bids []dsp.Bid
		var wg sync.WaitGroup
		wg.Add(len(dsps))
		for _, d := range dsps {
			go func() {
				defer wg.Done()
				if b, ok := d.Bid(bidCtx, req.Geo, req.Format, req.FloorPrice); ok {
					mu.Lock()
					bids = append(bids, b)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		metrics.AuctionDuration.Observe(time.Since(start).Seconds())

		if req.UserID != "" {
			var allowed []dsp.Bid
			for _, b := range bids {
				if capper.IsCapped(r.Context(), req.UserID, b.AdvertiserID) {
					metrics.FreqCapFiltered.WithLabelValues(b.AdvertiserID).Inc()
					slog.Debug("freqcap filtered", "user", req.UserID, "advertiser", b.AdvertiserID)
				} else {
					allowed = append(allowed, b)
				}
			}
			bids = allowed
		}

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

		if req.UserID != "" {
			if !capper.Record(r.Context(), req.UserID, winner.AdvertiserID) {
				slog.Info("freqcap denied winner at record", "user", req.UserID, "advertiser", winner.AdvertiserID)
				metrics.AuctionsTotal.WithLabelValues(req.Geo, req.Format, "no_bid").Inc()
				http.Error(w, "no bids", http.StatusNoContent)
				return
			}
		}

		slog.Info("auction", "geo", req.Geo, "format", req.Format,
			"winner", winner.AdvertiserID, "bid", winner.Price, "clearing", clearingPrice)

		go func() {
			if err := pub.Publish(context.Background(), events.AuctionEvent{
				Timestamp:     time.Now(),
				UserID:        req.UserID,
				Geo:           req.Geo,
				Format:        req.Format,
				AdvertiserID:  winner.AdvertiserID,
				ClearingPrice: clearingPrice,
			}); err != nil {
				slog.Warn("kafka publish failed", "err", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BidResponse{
			AdvertiserID: winner.AdvertiserID,
			Price:        clearingPrice,
		})
	}
}
