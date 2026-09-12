package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/gorgomania/mini-ssp/internal/auction"
	"github.com/gorgomania/mini-ssp/internal/dsp"
	pb "github.com/gorgomania/mini-ssp/proto"
)

//go:embed web
var webFS embed.FS

// ── DSP gRPC server ──────────────────────────────────────────────────────────

type dspServer struct {
	pb.UnimplementedAuctionServer
	name       string
	basePrice  float64
	boostGeo   string
	boostPrice float64
}

func (s *dspServer) RunAuction(_ context.Context, req *pb.BidRequest) (*pb.BidResponse, error) {
	price := s.basePrice + rand.Float64()*s.basePrice*0.5
	if s.boostGeo != "" && req.Geo == s.boostGeo {
		price = s.boostPrice + rand.Float64()*s.boostPrice*0.5
	}
	return &pb.BidResponse{AdvertiserId: s.name, Price: price}, nil
}

func startDSP(name, addr string, basePrice float64, boostGeo string, boostPrice float64) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("DSP %s listen: %v", name, err)
	}
	s := grpc.NewServer()
	pb.RegisterAuctionServer(s, &dspServer{
		name: name, basePrice: basePrice, boostGeo: boostGeo, boostPrice: boostPrice,
	})
	reflection.Register(s)
	log.Printf("DSP %-10s on %s", name, addr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("DSP %s serve: %v", name, err)
	}
}

// ── SSP HTTP server ──────────────────────────────────────────────────────────

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

// ── main ─────────────────────────────────────────────────────────────────────

var dspConfigs = []struct {
	name       string
	addr       string
	basePrice  float64
	boostGeo   string
	boostPrice float64
}{
	{"adcorp", ":50051", 0.5, "US", 2.0},
	{"medianet", ":50052", 0.4, "EU", 1.8},
	{"quickads", ":50053", 1.0, "", 0},
}

func main() {
	for _, cfg := range dspConfigs {
		cfg := cfg
		go startDSP(cfg.name, cfg.addr, cfg.basePrice, cfg.boostGeo, cfg.boostPrice)
	}

	var dsps []dsp.DSP
	for _, cfg := range dspConfigs {
		c, err := dsp.NewGRPCClient("localhost" + cfg.addr)
		if err != nil {
			log.Fatalf("connect DSP %s: %v", cfg.name, err)
		}
		dsps = append(dsps, c)
	}

	static, _ := fs.Sub(webFS, "web")
	http.Handle("/", http.FileServer(http.FS(static)))
	http.HandleFunc("/bid", makeBidHandler(dsps))

	log.Println("SSP demo → http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
