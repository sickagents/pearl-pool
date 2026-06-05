package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/pearl-mining/pearl-pool/pkg/config"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type API struct {
	pgStore    *storage.PostgresStore
	redisStore *storage.RedisStore
	router     *mux.Router
	server     *http.Server
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
	
	api := &API{
		pgStore:    pgStore,
		redisStore: redisStore,
		router:     mux.NewRouter(),
	}
	
	api.setupRoutes()
	
	// Setup CORS
	var handler http.Handler = api.router
	if cfg.API.CORSEnabled {
		c := cors.New(cors.Options{
			AllowedOrigins:   cfg.API.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
		})
		handler = c.Handler(api.router)
	}
	
	addr := fmt.Sprintf("%s:%d", cfg.API.Host, cfg.API.Port)
	api.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	log.Info().Str("addr", addr).Msg("API server listening")
	
	go func() {
		if err := api.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()
	
	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("Shutting down API server...")
	
	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := api.server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("API server shutdown error")
	}
	
	log.Info().Msg("API server stopped")
}

func (a *API) setupRoutes() {
	// Pool stats
	a.router.HandleFunc("/api/pool/stats", a.handlePoolStats).Methods("GET")
	a.router.HandleFunc("/api/blocks", a.handleBlocks).Methods("GET")
	
	// Miner stats
	a.router.HandleFunc("/api/miner/{address}", a.handleMinerStats).Methods("GET")
	a.router.HandleFunc("/api/miner/{address}/workers", a.handleMinerWorkers).Methods("GET")
	a.router.HandleFunc("/api/payments/{address}", a.handleMinerPayments).Methods("GET")
	
	// Metrics
	a.router.Handle("/metrics", promhttp.Handler())
	
	// Health
	a.router.HandleFunc("/health", a.handleHealth).Methods("GET")
}

func (a *API) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	hashrate, _ := a.redisStore.GetPoolHashrate()
	miners, _ := a.redisStore.GetTopMiners(100)
	blocks, _ := a.pgStore.GetRecentBlocks(10)
	
	// Count confirmed blocks
	confirmedBlocks := 0
	for _, block := range blocks {
		if block.Status == "confirmed" {
			confirmedBlocks++
		}
	}
	
	stats := map[string]interface{}{
		"hashrate":        hashrate,
		"miners":          len(miners),
		"blocks_found":    len(blocks),
		"blocks_confirmed": confirmedBlocks,
	}
	
	json.NewEncoder(w).Encode(stats)
}

func (a *API) handleBlocks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	blocks, err := a.pgStore.GetRecentBlocks(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"blocks": blocks,
	})
}

func (a *API) handleMinerStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	vars := mux.Vars(r)
	address := vars["address"]
	
	hashrate, _ := a.redisStore.GetMinerHashrate(address)
	shares, _ := a.redisStore.GetShareCount(address)
	balance, _ := a.pgStore.GetBalance(address)
	workers, _ := a.redisStore.GetOnlineWorkers(address)
	
	stats := map[string]interface{}{
		"address":  address,
		"hashrate": hashrate,
		"shares":   shares,
		"balance":  balance,
		"workers":  len(workers),
	}
	
	json.NewEncoder(w).Encode(stats)
}

func (a *API) handleMinerWorkers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	vars := mux.Vars(r)
	address := vars["address"]
	
	workers, err := a.redisStore.GetOnlineWorkers(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"workers": workers,
	})
}

func (a *API) handleMinerPayments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	vars := mux.Vars(r)
	address := vars["address"]
	
	payments, err := a.pgStore.GetPaymentHistory(address, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(map[string]interface{}{
		"payments": payments,
	})
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
