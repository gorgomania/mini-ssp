package auction

import "github.com/gorgomania/mini-ssp/internal/dsp"

// SecondPrice runs a Vickrey auction with an optional floor price.
// Bids below floor are rejected. Winner pays max(second_price, floor).
// Returns false if no eligible bids.
func SecondPrice(bids []dsp.Bid, floor float64) (winner dsp.Bid, clearingPrice float64, ok bool) {
	var eligible []dsp.Bid
	for _, b := range bids {
		if b.Price >= floor {
			eligible = append(eligible, b)
		}
	}
	if len(eligible) == 0 {
		return dsp.Bid{}, 0, false
	}

	first, second := eligible[0], dsp.Bid{}
	for _, b := range eligible[1:] {
		if b.Price > first.Price {
			second = first
			first = b
		} else if b.Price > second.Price {
			second = b
		}
	}

	clearing := second.Price
	if floor > clearing {
		clearing = floor
	}

	return first, clearing, true
}
