package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/pearl-mining/pearl-pool/pkg/config"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	
	cfg, err := config.Load("")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	
	log.Info().Msg("Starting API server")
	
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
	
	router := mux.NewRouter()
	
	// Pool stats
	router.HandleFunc("/api/pool/stats", api.GetPoolStats).Methods("GET")
	router.HandleFunc("/api/pool/blocks", api.GetRecentBlocks).Methods("GET")
	
	// Miner stats
	router.HandleFunc("/api/miner/{address}", api.GetMinerStats).Methods("GET")
	router.HandleFunc("/api/miner/{address}/workers", api.GetMinerWorkers).Methods("GET")
	router.HandleFunc("/api/miner/{address}/payments", api.GetMinerPayments).Methods("GET")
	
	// Metrics
	if cfg.API.EnableMetrics {
		router.Handle(cfg.API.MetricsPath, promhttp.Handler())
	}
	
	// CORS
	if cfg.API.CORSEnabled {
		router.Use(corsMiddleware(cfg.API.CORSOrigins))
	}
	
	addr := cfg.API.Host + ":" + string(rune(cfg.API.Port))
	log.Info().Str("addr", addr).Msg("API server listening")
	
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
}

func NewAPI(cfg *config.Config, pg *storage.PostgresStore, redis *storage.RedisStore) *API {
	return &API{
		cfg:        cfg,
		pgStore:    pg,
		redisStore: redis,
	}
}

func (a *API) GetPoolStats(w http.ResponseWriter, r *http.Request) {
	hashrate, _ := a.redisStore.GetPoolHashrate()
	
	// TODO: Get from storage
	stats := map[string]interface{}{
		"hashrate":    hashrate,
		"miners":      0,
		"workers":     0,
		"blocks":      0,
		"fee":         a.cfg.Pool.Fee,
		"minPayout":   a.cfg.Pool.MinPayout,
		"rewardMode":  a.cfg.Pool.RewardMode,
	}
	
	respondJSON(w, stats)
}

func (a *API) GetRecentBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := a.pgStore.GetPendingBlocks()
	if err != nil {
		respondError(w, 500, "Failed to fetch blocks")
		return
	}
	
	respondJSON(w, blocks)
}

func (a *API) GetMinerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	hashrate, _ := a.redisStore.GetMinerHashrate(address)
	shares, _ := a.redisStore.GetShareCount(address)
	
	stats := map[string]interface{}{
		"address":  address,
		"hashrate": hashrate,
		"shares":   shares,
	}
	
	respondJSON(w, stats)
}

func (a *API) GetMinerWorkers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	workers, err := a.redisStore.GetOnlineWorkers(address)
	if err != nil {
		respondError(w, 500, "Failed to fetch workers")
		return
	}
	
	respondJSON(w, map[string]interface{}{"workers": workers})
}

func (a *API) GetMinerPayments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	// TODO: Implement GetPaymentsByAddress in storage
	_ = address
	
	respondJSON(w, []interface{}{})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func corsMiddleware(origins []string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			
			// Check if origin is allowed
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
