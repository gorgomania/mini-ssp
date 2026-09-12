package dsp

import "math/rand"

type Bid struct {
	AdvertiserID string
	Price        float64
}

type DSP interface {
	Bid(geo, format string) (Bid, bool)
}

// AdCorp bids high on US traffic
type AdCorp struct{}

func (d AdCorp) Bid(geo, format string) (Bid, bool) {
	price := 0.5 + rand.Float64()*0.5
	if geo == "US" {
		price = 2.0 + rand.Float64()*1.5
	}
	return Bid{AdvertiserID: "adcorp", Price: price}, true
}

// MediaNet bids high on EU traffic
type MediaNet struct{}

func (d MediaNet) Bid(geo, format string) (Bid, bool) {
	price := 0.4 + rand.Float64()*0.4
	if geo == "EU" {
		price = 1.8 + rand.Float64()*1.2
	}
	return Bid{AdvertiserID: "medianet", Price: price}, true
}

// QuickAds always bids a flat rate, but passes on video
type QuickAds struct{}

func (d QuickAds) Bid(geo, format string) (Bid, bool) {
	if format == "video" {
		return Bid{}, false
	}
	return Bid{AdvertiserID: "quickads", Price: 1.0 + rand.Float64()*0.3}, true
}
