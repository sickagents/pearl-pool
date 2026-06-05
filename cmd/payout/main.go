package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pearl-mining/pearl-pool/pkg/config"
	"github.com/pearl-mining/pearl-pool/pkg/payout"
	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
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
	
	log.Info().Msg("Starting payout engine")
	
	rpcClient := rpc.NewClient(
		cfg.Node.Host,
		cfg.Node.Port,
		cfg.Node.RPCUser,
		cfg.Node.RPCPass,
		cfg.Node.TLS,
		cfg.Node.SubmitTimeout,
	)
	
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
	
	engine := payout.NewEngine(
		rpcClient,
		pgStore,
		cfg.Payout.MinPayout,
		cfg.Payout.MaxBatchSize,
		cfg.Payout.GasBuffer,
	)
	
	if !cfg.Payout.Enabled {
		log.Warn().Msg("Payout engine is disabled in config")
		return
	}
	
	go engine.StartScheduled(cfg.Payout.Interval)
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("Payout engine stopped")
}
