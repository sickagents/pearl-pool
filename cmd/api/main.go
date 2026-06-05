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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type APIServer struct {
	cfg        *config.Config
	pgStore    *storage.PostgresStore
	redisStore *storage.RedisStore
	router     *mux.Router
}

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
	
	server := &APIServer{
		cfg:        cfg,
		pgStore:    pgStore,
		redisStore: redisStore,
		router:     mux.NewRouter(),
	}
	
	server.setupRoutes()
	
	addr := cfg.API.Host + ":" + string(rune(cfg.API.Port))
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	go func() {
		log.Info().Str("addr", addr).Msg("API server listening")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("API server stopped")
}

func (s *APIServer) setupRoutes() {
	// CORS middleware
	if s.cfg.API.CORSEnabled {
		s.router.Use(corsMiddleware(s.cfg.API.CORSOrigins))
	}
	
	// API routes
	api := s.router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/pool/stats", s.handlePoolStats).Methods("GET")
	api.HandleFunc("/pool/blocks", s.handlePoolBlocks).Methods("GET")
	api.HandleFunc("/miner/{address}", s.handleMinerStats).Methods("GET")
	api.HandleFunc("/miner/{address}/workers", s.handleMinerWorkers).Methods("GET")
	api.HandleFunc("/miner/{address}/payments", s.handleMinerPayments).Methods("GET")
	
	// Metrics endpoint
	if s.cfg.API.EnableMetrics {
		s.router.Handle(s.cfg.API.MetricsPath, promhttp.Handler())
	}
	
	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
}

func (s *APIServer) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	hashrate, _ := s.redisStore.GetPoolHashrate()
	
	// Get miner count
	miners, _ := s.redisStore.GetTopMiners(1000)
	
	// Get blocks found (last 24h)
	// TODO: Query from PostgreSQL with time filter
	
	stats := map[string]interface{}{
		"hashrate":     hashrate,
		"miners":       len(miners),
		"workers":      0, // TODO: aggregate from Redis
		"blocks_24h":   0, // TODO
		"pool_fee":     s.cfg.Pool.Fee,
		"min_payout":   s.cfg.Pool.MinPayout,
		"reward_mode":  s.cfg.Pool.RewardMode,
	}
	
	respondJSON(w, stats)
}

func (s *APIServer) handlePoolBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := s.pgStore.GetPendingBlocks()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch blocks")
		return
	}
	
	// Convert to response format
	var response []map[string]interface{}
	for _, block := range blocks {
		response = append(response, map[string]interface{}{
			"hash":          block.Hash,
			"height":        block.Height,
			"reward":        float64(block.Reward) / 1e8,
			"finder":        block.Finder,
			"status":        block.Status,
			"confirmations": block.Confirmations,
			"created_at":    block.CreatedAt.Unix(),
		})
	}
	
	respondJSON(w, response)
}

func (s *APIServer) handleMinerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	hashrate, _ := s.redisStore.GetMinerHashrate(address)
	shares, _ := s.redisStore.GetShareCount(address)
	balance, _ := s.pgStore.GetPayoutDue(0) // TODO: single address query
	
	stats := map[string]interface{}{
		"address":  address,
		"hashrate": hashrate,
		"shares":   shares,
		"balance":  0.0, // TODO
		"workers":  0,   // TODO
	}
	
	respondJSON(w, stats)
}

func (s *APIServer) handleMinerWorkers(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	workers, err := s.redisStore.GetOnlineWorkers(address)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to fetch workers")
		return
	}
	
	respondJSON(w, map[string]interface{}{
		"workers": workers,
		"count":   len(workers),
	})
}

func (s *APIServer) handleMinerPayments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	// TODO: Query payouts from PostgreSQL
	_ = address
	
	respondJSON(w, []map[string]interface{}{})
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, map[string]string{"status": "ok"})
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
			if origin == "" {
				origin = "*"
			}
			
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
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}
