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

type API struct {
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
	
	api := &API{
		pgStore:    pgStore,
		redisStore: redisStore,
		router:     mux.NewRouter(),
	}
	
	api.setupRoutes()
	
	addr := cfg.API.Host + ":" + string(rune(cfg.API.Port))
	log.Info().Str("addr", addr).Msg("API server listening")
	
	go func() {
		if err := http.ListenAndServe(addr, api.router); err != nil {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("API server stopped")
}

func (a *API) setupRoutes() {
	// Pool stats
	a.router.HandleFunc("/api/pool/stats", a.handlePoolStats).Methods("GET")
	a.router.HandleFunc("/api/pool/blocks", a.handlePoolBlocks).Methods("GET")
	
	// Miner stats
	a.router.HandleFunc("/api/miner/{address}", a.handleMinerStats).Methods("GET")
	a.router.HandleFunc("/api/miner/{address}/workers", a.handleMinerWorkers).Methods("GET")
	a.router.HandleFunc("/api/miner/{address}/payments", a.handleMinerPayments).Methods("GET")
	
	// Metrics
	a.router.Handle("/metrics", promhttp.Handler())
	
	// Health
	a.router.HandleFunc("/health", a.handleHealth).Methods("GET")
}

func (a *API) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	hashrate, _ := a.redisStore.GetPoolHashrate()
	
	blocks, _ := a.pgStore.GetRecentBlocks(10)
	
	stats := map[string]interface{}{
		"hashrate":     hashrate,
		"blocks_found": len(blocks),
	}
	
	json.NewEncoder(w).Encode(stats)
}

func (a *API) handlePoolBlocks(w http.ResponseWriter, r *http.Request) {
	blocks, err := a.pgStore.GetRecentBlocks(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(blocks)
}

func (a *API) handleMinerStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	address := vars["address"]
	
	hashrate, _ := a.redisStore.GetMinerHashrate(address)
	shares, _ := a.redisStore.GetShareCount(address)
	balance, _ := a.pgStore.GetBalance(address)
	
	stats := map[string]interface{}{
		"address":  address,
		"hashrate": hashrate,
		"shares":   shares,
		"balance":  balance,
	}
	
	json.NewEncoder(w).Encode(stats)
}

func (a *API) handleMinerWorkers(w http.ResponseWriter, r *http.Request) {
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
	vars := mux.Vars(r)
	address := vars["address"]
	
	payments, err := a.pgStore.GetPaymentHistory(address, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(payments)
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
