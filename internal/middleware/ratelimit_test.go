package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

var noop = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})

func req(ip string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = ip + ":1234"
	return r
}

func TestRateLimit_AllowsUpToBurst(t *testing.T) {
	handler := RateLimit(1, 3)(noop)
	for i := range 3 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req("1.2.3.4"))
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimit_BlocksOverBurst(t *testing.T) {
	handler := RateLimit(1, 3)(noop)
	for range 3 {
		handler.ServeHTTP(httptest.NewRecorder(), req("1.2.3.4"))
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req("1.2.3.4"))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
}

func TestRateLimit_IndependentPerIP(t *testing.T) {
	handler := RateLimit(1, 1)(noop)

	// IP A exhausts its burst
	handler.ServeHTTP(httptest.NewRecorder(), req("1.2.3.4"))

	// IP B is unaffected
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req("5.6.7.8"))
	if rr.Code != http.StatusOK {
		t.Fatalf("IP B should not be limited by IP A; got %d", rr.Code)
	}

	// IP A is now blocked
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req("1.2.3.4"))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("IP A should be limited; got %d", rr.Code)
	}
}

func TestRateLimit_BadRemoteAddr(t *testing.T) {
	handler := RateLimit(10, 10)(noop)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "malformed"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for malformed addr, got %d", rr.Code)
	}
}
