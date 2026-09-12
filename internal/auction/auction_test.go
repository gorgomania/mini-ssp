package auction

import (
	"testing"

	"github.com/gorgomania/mini-ssp/internal/dsp"
)

func bid(id string, price float64) dsp.Bid {
	return dsp.Bid{AdvertiserID: id, Price: price}
}

func TestSecondPrice_NoBids(t *testing.T) {
	_, _, ok := SecondPrice(nil, 0)
	if ok {
		t.Fatal("expected no winner for empty bids")
	}
}

func TestSecondPrice_OneBid(t *testing.T) {
	winner, clearing, ok := SecondPrice([]dsp.Bid{bid("a", 2.0)}, 0)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.AdvertiserID != "a" {
		t.Fatalf("expected winner a, got %s", winner.AdvertiserID)
	}
	if clearing != 0 {
		t.Fatalf("expected clearing price 0, got %f", clearing)
	}
}

func TestSecondPrice_Normal(t *testing.T) {
	bids := []dsp.Bid{bid("a", 1.0), bid("b", 2.5), bid("c", 1.8)}
	winner, clearing, ok := SecondPrice(bids, 0)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.AdvertiserID != "b" {
		t.Fatalf("expected winner b, got %s", winner.AdvertiserID)
	}
	if clearing != 1.8 {
		t.Fatalf("expected clearing price 1.8, got %f", clearing)
	}
}

func TestSecondPrice_EqualPrices(t *testing.T) {
	bids := []dsp.Bid{bid("a", 1.5), bid("b", 1.5)}
	winner, clearing, ok := SecondPrice(bids, 0)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.AdvertiserID != "a" {
		t.Fatalf("expected winner a, got %s", winner.AdvertiserID)
	}
	if clearing != 1.5 {
		t.Fatalf("expected clearing price 1.5, got %f", clearing)
	}
}

func TestSecondPrice_WinnerPaysSecond(t *testing.T) {
	bids := []dsp.Bid{bid("a", 5.0), bid("b", 3.0), bid("c", 1.0)}
	_, clearing, _ := SecondPrice(bids, 0)
	if clearing != 3.0 {
		t.Fatalf("expected clearing price 3.0, got %f", clearing)
	}
}

func TestSecondPrice_FloorRejectsAll(t *testing.T) {
	bids := []dsp.Bid{bid("a", 1.0), bid("b", 0.5)}
	_, _, ok := SecondPrice(bids, 2.0)
	if ok {
		t.Fatal("expected no winner when all bids below floor")
	}
}

func TestSecondPrice_FloorRejectsSome(t *testing.T) {
	bids := []dsp.Bid{bid("a", 3.0), bid("b", 0.5)}
	winner, clearing, ok := SecondPrice(bids, 1.0)
	if !ok {
		t.Fatal("expected winner")
	}
	if winner.AdvertiserID != "a" {
		t.Fatalf("expected winner a, got %s", winner.AdvertiserID)
	}
	// единственный eligible бид → платит floor, не 0
	if clearing != 1.0 {
		t.Fatalf("expected clearing price 1.0 (floor), got %f", clearing)
	}
}

func TestSecondPrice_FloorRaisesClearing(t *testing.T) {
	// floor выше второго места → победитель платит floor
	bids := []dsp.Bid{bid("a", 3.0), bid("b", 1.0)}
	_, clearing, _ := SecondPrice(bids, 1.5)
	if clearing != 1.5 {
		t.Fatalf("expected clearing price 1.5 (floor), got %f", clearing)
	}
}

func TestSecondPrice_FloorBelowSecond(t *testing.T) {
	// floor ниже второго места → победитель платит второе место
	bids := []dsp.Bid{bid("a", 3.0), bid("b", 2.0)}
	_, clearing, _ := SecondPrice(bids, 1.0)
	if clearing != 2.0 {
		t.Fatalf("expected clearing price 2.0 (second bid), got %f", clearing)
	}
}
