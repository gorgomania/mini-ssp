package ssp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorgomania/mini-ssp/internal/dsp"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
	"github.com/gorgomania/mini-ssp/internal/ssp"
)

var noop = freqcap.NoopCapper{}

type fixedDSP struct {
	id      string
	premium float64
	ok      bool
}

func (d fixedDSP) Bid(_, _ string, floor float64) (dsp.Bid, bool) {
	if !d.ok {
		return dsp.Bid{}, false
	}
	return dsp.Bid{AdvertiserID: d.id, Price: floor + d.premium}, true
}

func post(t *testing.T, handler http.Handler, req ssp.BidRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	return rr
}

func TestBidHandler_NoDSPs(t *testing.T) {
	rr := post(t, ssp.BidHandler(nil, noop), ssp.BidRequest{Geo: "US", Format: "banner"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestBidHandler_Winner(t *testing.T) {
	dsps := []dsp.DSP{
		fixedDSP{"a", 2.0, true},
		fixedDSP{"b", 1.0, true},
	}
	rr := post(t, ssp.BidHandler(dsps, noop), ssp.BidRequest{Geo: "US", Format: "banner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp ssp.BidResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.AdvertiserID != "a" {
		t.Fatalf("expected winner a, got %s", resp.AdvertiserID)
	}
	if resp.Price != 1.0 {
		t.Fatalf("expected clearing 1.0, got %f", resp.Price)
	}
}

func TestBidHandler_FloorPrice(t *testing.T) {
	dsps := []dsp.DSP{fixedDSP{"a", 1.0, true}}
	rr := post(t, ssp.BidHandler(dsps, noop), ssp.BidRequest{Geo: "US", Format: "banner", FloorPrice: 100.0})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp ssp.BidResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	// единственный бид → clearing = floor = 100
	if resp.Price != 100.0 {
		t.Fatalf("expected clearing 100.0, got %f", resp.Price)
	}
}

func TestBidHandler_DSPPasses(t *testing.T) {
	dsps := []dsp.DSP{fixedDSP{"a", 1.0, false}}
	rr := post(t, ssp.BidHandler(dsps, noop), ssp.BidRequest{Geo: "US", Format: "video"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestBidHandler_BadRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/bid", bytes.NewBufferString("not json"))
	rr := httptest.NewRecorder()
	ssp.BidHandler(nil, noop).ServeHTTP(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// spyCapper позволяет задать, кто закапан, и фиксирует вызовы Record.
type spyCapper struct {
	capped  map[string]bool // ключ: "userID:advertiserID"
	recorded []string       // advertiserID из каждого Record
}

func newSpyCapper(capped ...string) *spyCapper {
	m := make(map[string]bool, len(capped))
	for _, k := range capped {
		m[k] = true
	}
	return &spyCapper{capped: m}
}

func (s *spyCapper) IsCapped(_ context.Context, userID, advID string) bool {
	return s.capped[userID+":"+advID]
}

func (s *spyCapper) Record(_ context.Context, _, advID string) bool {
	s.recorded = append(s.recorded, advID)
	return true
}

func TestBidHandler_FreqCap_FiltersCapped(t *testing.T) {
	dsps := []dsp.DSP{
		fixedDSP{"a", 2.0, true},
		fixedDSP{"b", 1.0, true},
	}
	// пользователь u1 закапан для advertiser "a" → выигрывает "b"
	spy := newSpyCapper("u1:a")
	rr := post(t, ssp.BidHandler(dsps, spy), ssp.BidRequest{UserID: "u1", Geo: "US", Format: "banner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp ssp.BidResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.AdvertiserID != "b" {
		t.Fatalf("expected winner b, got %s", resp.AdvertiserID)
	}
}

func TestBidHandler_FreqCap_AllCapped(t *testing.T) {
	dsps := []dsp.DSP{
		fixedDSP{"a", 2.0, true},
		fixedDSP{"b", 1.0, true},
	}
	spy := newSpyCapper("u1:a", "u1:b")
	rr := post(t, ssp.BidHandler(dsps, spy), ssp.BidRequest{UserID: "u1", Geo: "US", Format: "banner"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestBidHandler_FreqCap_RecordsWinner(t *testing.T) {
	dsps := []dsp.DSP{
		fixedDSP{"a", 2.0, true},
		fixedDSP{"b", 1.0, true},
	}
	spy := newSpyCapper()
	post(t, ssp.BidHandler(dsps, spy), ssp.BidRequest{UserID: "u1", Geo: "US", Format: "banner"})
	if len(spy.recorded) != 1 || spy.recorded[0] != "a" {
		t.Fatalf("expected Record(a), got %v", spy.recorded)
	}
}

func TestBidHandler_FreqCap_SkippedWithoutUserID(t *testing.T) {
	dsps := []dsp.DSP{fixedDSP{"a", 2.0, true}}
	// capper закапывает всех, но user_id пустой → cap игнорируется
	spy := newSpyCapper("" + ":" + "a") // на случай если всё-таки вызовут с ""
	rr := post(t, ssp.BidHandler(dsps, spy), ssp.BidRequest{Geo: "US", Format: "banner"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(spy.recorded) != 0 {
		t.Fatalf("Record should not be called without user_id, got %v", spy.recorded)
	}
}
