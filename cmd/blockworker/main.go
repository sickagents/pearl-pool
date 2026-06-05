package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/accounting"
	"github.com/pearl-mining/pearl-pool/pkg/config"
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
	
	log.Info().Msg("Starting block confirmation worker")
	
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
	
	worker := NewBlockConfirmationWorker(
		rpcClient,
		pgStore,
		cfg.Pool.ConfirmationDepth,
		cfg.Pool.RewardMode,
		cfg.Pool.PPLNSWindow,
		cfg.Pool.Fee,
	)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go worker.Start(ctx, 60*time.Second)
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	log.Info().Msg("Block confirmation worker stopped")
}

type BlockConfirmationWorker struct {
	rpcClient         *rpc.Client
	pgStore           *storage.PostgresStore
	confirmationDepth int
	rewardMode        string
	pplnsWindow       int
	poolFee           float64
}

func NewBlockConfirmationWorker(
	rpcClient *rpc.Client,
	pgStore *storage.PostgresStore,
	confirmationDepth int,
	rewardMode string,
	pplnsWindow int,
	poolFee float64,
) *BlockConfirmationWorker {
	return &BlockConfirmationWorker{
		rpcClient:         rpcClient,
		pgStore:           pgStore,
		confirmationDepth: confirmationDepth,
		rewardMode:        rewardMode,
		pplnsWindow:       pplnsWindow,
		poolFee:           poolFee,
	}
}

func (w *BlockConfirmationWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processBlocks(); err != nil {
				log.Error().Err(err).Msg("Failed to process blocks")
			}
		}
	}
}

func (w *BlockConfirmationWorker) processBlocks() error {
	blocks, err := w.pgStore.GetPendingBlocks()
	if err != nil {
		return err
	}
	
	if len(blocks) == 0 {
		return nil
	}
	
	currentHeight, err := w.rpcClient.GetBlockCount()
	if err != nil {
		return err
	}
	
	for _, block := range blocks {
		confirmations := int(currentHeight - block.Height + 1)
		
		// Check if block still exists at this height
		hashAtHeight, err := w.rpcClient.GetBlockHash(block.Height)
		if err != nil {
			log.Error().Err(err).Int64("height", block.Height).Msg("Failed to get block hash")
			continue
		}
		
		// Block is orphaned if hash doesn't match
		if hashAtHeight != block.Hash {
			log.Warn().
				Str("hash", block.Hash).
				Int64("height", block.Height).
				Msg("Block orphaned")
			
			if err := w.pgStore.UpdateBlockStatus(block.Hash, "orphaned", 0); err != nil {
				log.Error().Err(err).Msg("Failed to update block status")
			}
			continue
		}
		
		// Update confirmations
		if err := w.pgStore.UpdateBlockStatus(block.Hash, "confirming", confirmations); err != nil {
			log.Error().Err(err).Msg("Failed to update confirmations")
		}
		
		// If reached confirmation depth, credit rewards
		if confirmations >= w.confirmationDepth {
			log.Info().
				Str("hash", block.Hash).
				Int64("height", block.Height).
				Int("confirmations", confirmations).
				Msg("Block confirmed, crediting rewards")
			
			if err := w.creditRewards(block); err != nil {
				log.Error().Err(err).Msg("Failed to credit rewards")
				continue
			}
			
			if err := w.pgStore.UpdateBlockStatus(block.Hash, "confirmed", confirmations); err != nil {
				log.Error().Err(err).Msg("Failed to update block status")
			}
		}
	}
	
	return nil
}

func (w *BlockConfirmationWorker) creditRewards(block storage.Block) error {
	// Get shares for this block's round
	shares, err := w.pgStore.GetPendingShares(w.pplnsWindow * 2)
	if err != nil {
		return err
	}
	
	// Filter shares up to this block height
	var roundShares []accounting.ShareRecord
	for _, share := range shares {
		if share.Height <= block.Height {
			roundShares = append(roundShares, accounting.ShareRecord{
				Address:    share.Address,
				Difficulty: share.Difficulty,
				Height:     share.Height,
			})
		}
	}
	
	// Calculate rewards
	calc := accounting.NewRewardCalculator(
		w.rewardMode,
		w.pplnsWindow,
		w.poolFee,
		w.confirmationDepth,
	)
	
	var rewards []accounting.Reward
	if w.rewardMode == "pplns" {
		rewards = calc.CalculatePPLNS(block.Reward, roundShares)
	} else {
		// Find round start (last block height or 0)
		roundStart := int64(0)
		for i := len(shares) - 1; i >= 0; i-- {
			if shares[i].IsBlock {
				roundStart = shares[i].Height
				break
			}
		}
		rewards = calc.CalculatePROP(block.Reward, roundShares, roundStart)
	}
	
	// Credit balances
	for _, reward := range rewards {
		if err := w.pgStore.CreditBalance(reward.Address, reward.Amount, block.ID); err != nil {
			log.Error().Err(err).Str("address", reward.Address).Msg("Failed to credit balance")
			continue
		}
		
		log.Info().
			Str("address", reward.Address).
			Int64("amount", reward.Amount).
			Msg("Balance credited")
	}
	
	// Mark shares as credited
	var shareIDs []int64
	for _, share := range shares {
		if share.Height <= block.Height {
			shareIDs = append(shareIDs, share.ID)
		}
	}
	
	if len(shareIDs) > 0 {
		if err := w.pgStore.MarkSharesCredited(shareIDs); err != nil {
			return err
		}
	}
	
	return nil
}
