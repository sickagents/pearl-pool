package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/accounting"
	"github.com/pearl-mining/pearl-pool/pkg/config"
	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/pearl-mining/pearl-pool/pkg/stratum"
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
	
	log.Info().Str("pool", cfg.Pool.Name).Float64("fee", cfg.Pool.Fee).Msg("Starting PEARL mining pool")
	
	// Initialize RPC client
	rpcClient := rpc.NewClient(
		cfg.Node.Host,
		cfg.Node.Port,
		cfg.Node.RPCUser,
		cfg.Node.RPCPass,
		cfg.Node.TLS,
		cfg.Node.SubmitTimeout,
	)
	
	// Test node connection
	height, err := rpcClient.GetBlockCount()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PEARL node")
	}
	log.Info().Int64("height", height).Msg("Connected to PEARL node")
	
	// Initialize storage
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
	
	// Initialize job manager
	jobManager := stratum.NewJobManager(100)
	
	// Fetch initial block template
	template, err := rpcClient.GetBlockTemplate()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get block template")
	}
	
	templateMap := map[string]interface{}{
		"previousblockhash": template.PreviousBlockHash,
		"coinbasevalue":     float64(template.CoinbaseValue),
		"height":            float64(template.Height),
		"bits":              template.Bits,
		"curtime":           float64(template.CurTime),
		"transactions":      template.Transactions,
	}
	
	job, err := jobManager.NewJob(templateMap)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create initial job")
	}
	log.Info().Str("job_id", job.ID).Int64("height", job.Height).Msg("Initial job created")
	
	// Initialize accounting
	calculator := accounting.NewRewardCalculator(
		cfg.Pool.RewardMode,
		cfg.Pool.PPLNSWindow,
		cfg.Pool.Fee,
		cfg.Pool.ConfirmationDepth,
	)
	
	// Start Stratum servers
	var servers []*stratum.Server
	for _, portCfg := range cfg.Stratum.Ports {
		server := stratum.NewServer(portCfg.Port, portCfg.Difficulty, jobManager)
		if err := server.Start(); err != nil {
			log.Fatal().Err(err).Int("port", portCfg.Port).Msg("Failed to start Stratum server")
		}
		servers = append(servers, server)
	}
	
	// Start block confirmation loop
	confirmationLoop := NewBlockConfirmationLoop(
		rpcClient,
		pgStore,
		calculator,
		cfg.Pool.ConfirmationDepth,
		1*time.Minute, // Check every minute
	)
	confirmationLoop.Start()
	defer confirmationLoop.Stop()
	
	// Start stats collector
	statsCollector := NewStatsCollector(pgStore, redisStore, 30*time.Second)
	statsCollector.Start()
	defer statsCollector.Stop()
	
	// Start block template poller
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go pollBlockTemplate(ctx, rpcClient, jobManager, servers, cfg.Node.PollInterval)
	
	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("Shutting down...")
	cancel()
	
	for _, server := range servers {
		server.Stop()
	}
	
	log.Info().Msg("Pool stopped")
}

func pollBlockTemplate(ctx context.Context, rpcClient *rpc.Client, jobManager *stratum.JobManager, servers []*stratum.Server, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			template, err := rpcClient.GetBlockTemplate()
			if err != nil {
				log.Error().Err(err).Msg("Failed to poll block template")
				continue
			}
			
			currentJob := jobManager.GetCurrentJob()
			if currentJob != nil && currentJob.PrevHash == template.PreviousBlockHash {
				continue
			}
			
			templateMap := map[string]interface{}{
				"previousblockhash": template.PreviousBlockHash,
				"coinbasevalue":     float64(template.CoinbaseValue),
				"height":            float64(template.Height),
				"bits":              template.Bits,
				"curtime":           float64(template.CurTime),
				"transactions":      template.Transactions,
			}
			
			job, err := jobManager.NewJob(templateMap)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create new job")
				continue
			}
			
			log.Info().Str("job_id", job.ID).Int64("height", job.Height).Msg("New job created")
			
			for _, server := range servers {
				server.BroadcastJob(job)
			}
		}
	}
}
