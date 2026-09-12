package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	AuctionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ssp_auctions_total",
		Help: "Total auctions by result",
	}, []string{"geo", "format", "result"}) // result: win | no_bid

	AuctionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "ssp_auction_duration_seconds",
		Help:    "Time from request to auction result",
		Buckets: []float64{.005, .01, .025, .05, .1, .25},
	})

	ClearingPrice = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ssp_clearing_price",
		Help:    "Clearing price distribution",
		Buckets: []float64{0.5, 1, 2, 5, 10, 50, 100, 500},
	}, []string{"geo", "format"})

	WinnerBids = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ssp_winner_bids_total",
		Help: "Auctions won per DSP",
	}, []string{"advertiser_id"})
)
