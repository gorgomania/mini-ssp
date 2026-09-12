package main

import (
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorgomania/mini-ssp/internal/auction"
	"github.com/gorgomania/mini-ssp/internal/dsp"
)

//go:embed web
var webFS embed.FS

type bidRequest struct {
	Geo    string `json:"geo"`
	Format string `json:"format"`
}

type bidResponse struct {
	AdvertiserID string  `json:"advertiser_id"`
	Price        float64 `json:"price"`
}

func makeBidHandler(dsps []dsp.DSP) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req bidRequest
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
				if b, ok := d.Bid(req.Geo, req.Format); ok {
					mu.Lock()
					bids = append(bids, b)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		winner, clearingPrice, ok := auction.SecondPrice(bids)
		if !ok {
			log.Printf("no bids geo=%s format=%s", req.Geo, req.Format)
			http.Error(w, "no bids", http.StatusNoContent)
			return
		}

		log.Printf("auction geo=%s format=%s winner=%s bid=%.3f clearing=%.3f",
			req.Geo, req.Format, winner.AdvertiserID, winner.Price, clearingPrice)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bidResponse{
			AdvertiserID: winner.AdvertiserID,
			Price:        clearingPrice,
		})
	}
}

func main() {
	dspAddrs := flag.String("dsps", "localhost:50051,localhost:50052,localhost:50053", "comma-separated DSP gRPC addresses")
	port := flag.String("port", "8080", "HTTP listen port")
	flag.Parse()

	var dsps []dsp.DSP
	for addr := range strings.SplitSeq(*dspAddrs, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		c, err := dsp.NewGRPCClient(addr)
		if err != nil {
			log.Fatalf("connect DSP %s: %v", addr, err)
		}
		log.Printf("registered DSP at %s", addr)
		dsps = append(dsps, c)
	}

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", makeBidHandler(dsps))
	log.Printf("SSP HTTP listening on :%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
