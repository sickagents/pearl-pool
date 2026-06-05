package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/pearl-mining/pearl-pool/pkg/config"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	poolHashrate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pool_hashrate",
		Help: "Total pool hashrate",
	})
	
	poolMiners = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pool_miners",
		Help: "Active miners count",
	})
	
	poolWorkers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pool_workers",
		Help: "Active workers count",
	})
	
	poolBlocksFound = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pool_blocks_found",
		Help: "Total blocks found",
	})
	
	poolSharesAccepted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pool_shares_accepted",
		Help: "Total accepted shares",
	})
	
	poolSharesRejected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "pool_shares_rejected",
		Help: "Total rejected shares",
	})
)

func init() {
	prometheus.MustRegister(poolHashrate)
	prometheus.MustRegister(poolMiners)
	prometheus.MustRegister(poolWorkers)
	prometheus.MustRegister(poolBlocksFound)
	prometheus.MustRegister(poolSharesAccepted)
	prometheus.MustRegister(poolSharesRejected)
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	
	cfg, err := config.Load("")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	
	log.Info().Int("port", cfg.API.Port).Msg("Starting API server")
	
	pgStore, err := storage.NewPostgresStore(
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Database,
		cfg.Database.MaxConns,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer pgStore.Close()
	
	redisStore, err := storage.NewRedisStore(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisStore.Close()
	
	api := NewAPI(cfg, pgStore, redisStore)
	
	srv := &http.Server{
		Addr:         cfg.API.Host + ":" + string(rune(cfg.API.Port)),
		Handler:      api.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("API server stopped")
}

type API struct {
	cfg        *config.Config
	pgStore    *storage.PostgresStore
	redisStore *storage.RedisStore
	router     *mux.Router
}

func NewAPI(cfg *config.Config, pg *storage.PostgresStore, redis *storage.RedisStore) *API {
	api := &API{
		cfg:        cfg,
		pgStore:    pg,
		redisStore: redis,
		router:     mux.NewRouter(),
	}
	
	api.setupRoutes()
	return api
}

func (a *API) setupRoutes() {
	// Pool stats
	a.router.HandleFunc("/api/pool/stats", a.handlePoolStats).Methods("GET")
	
	// Miner stats
	a.router.HandleFunc("/api/miner/{address}", a.handleMinerStats).Methods("GET")
	
	// Blocks
	a.router.HandleFunc("/api/blocks", a.handleBlocks).Methods("GET")
	
	// Payments
	a.router.HandleFunc("/api/payments/{address}", a.handlePayments).Methods("GET")
	
	// Metrics
	if a.cfg.API.EnableMetrics {
		a.router.Handle(a.cfg.API.MetricsPath, promhttp.Handler())
	}
	
	// CORS
	if a.cfg.API.CORSEnabled {
		a.router.Use(corsMiddleware(a.cfg.API.CORSOrigins))
	}
}

func (a *API) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	hashrate, _ := a.redisStore.GetPoolHashrate()
	
	// Get miner count from Redis
	miners, _ := a.redisStore.GetTopMiners(1000)
	
	respondJSON(w, map[string]interface{}{
		"hashrate":    hashrate,
		"miners":      len(miners),
		"pool_fee":    a.cfg.Pool.Fee,
		"min_payout":  a.cfg.Pool.MinPayout,
		"reward_mode": a.cfg.Pool.RewardMode,
	})
}

func (a *API) handleMinerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	hashrate, _ := a.redisStore.GetMinerHashrate(address)
	shares, _ := a.redisStore.GetShareCount(address)
	workers, _ := a.redisStore.GetOnlineWorkers(address)
	
	// Get balance from PostgreSQL
	balances, err := a.pgStore.GetPayoutDue(0)
	var balance int64
	if err == nil {
		for _, bal := range balances {
			if bal.Address == address {
				balance = bal.Balance
				break
			}
		}
	}
	
	respondJSON(w, map[string]interface{}{
		"address":  address,
		"hashrate": hashrate,
		"shares":   shares,
		"workers":  len(workers),
		"balance":  float64(balance) / 1e8,
	})
}

func (a *API) handleBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := a.pgStore.GetPendingBlocks()
	if err != nil {
		respondError(w, "Failed to fetch blocks", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, map[string]interface{}{
		"blocks": blocks,
		"count":  len(blocks),
	})
}

func (a *API) handlePayments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	// TODO: Get payments from database
	respondJSON(w, map[string]interface{}{
		"address":  address,
		"payments": []interface{}{},
	})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func corsMiddleware(origins []string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			
			allowed := false
			for _, o := range origins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
			
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}
