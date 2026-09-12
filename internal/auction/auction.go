package auction

import "github.com/gorgomania/mini-ssp/internal/dsp"

// SecondPrice runs a Vickrey auction: winner pays the second-highest price.
// Returns the winning bid and clearing price, or false if no bids.
func SecondPrice(bids []dsp.Bid) (winner dsp.Bid, clearingPrice float64, ok bool) {
	if len(bids) == 0 {
		return dsp.Bid{}, 0, false
	}

	first, second := bids[0], dsp.Bid{}
	for _, b := range bids[1:] {
		if b.Price > first.Price {
			second = first
			first = b
		} else if b.Price > second.Price {
			second = b
		}
	}

	return first, second.Price, true
}
