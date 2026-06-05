package main

import (
	"context"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/accounting"
	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

// BlockConfirmationLoop monitors pending blocks and updates their status
type BlockConfirmationLoop struct {
	rpcClient   *rpc.Client
	pgStore     *storage.PostgresStore
	calculator  *accounting.RewardCalculator
	confirmDepth int
	pollInterval time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewBlockConfirmationLoop(
	rpcClient *rpc.Client,
	pgStore *storage.PostgresStore,
	calculator *accounting.RewardCalculator,
	confirmDepth int,
	pollInterval time.Duration,
) *BlockConfirmationLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockConfirmationLoop{
		rpcClient:    rpcClient,
		pgStore:      pgStore,
		calculator:   calculator,
		confirmDepth: confirmDepth,
		pollInterval: pollInterval,
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (b *BlockConfirmationLoop) Start() {
	go b.run()
}

func (b *BlockConfirmationLoop) Stop() {
	b.cancel()
}

func (b *BlockConfirmationLoop) run() {
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()
	
	log.Info().Dur("interval", b.pollInterval).Int("depth", b.confirmDepth).Msg("Block confirmation loop started")
	
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if err := b.checkPendingBlocks(); err != nil {
				log.Error().Err(err).Msg("Failed to check pending blocks")
			}
		}
	}
}

func (b *BlockConfirmationLoop) checkPendingBlocks() error {
	// Get current blockchain height
	currentHeight, err := b.rpcClient.GetBlockCount()
	if err != nil {
		return err
	}
	
	// Get pending blocks from database
	blocks, err := b.pgStore.GetPendingBlocks()
	if err != nil {
		return err
	}
	
	for _, block := range blocks {
		confirmations := int(currentHeight - block.Height + 1)
		
		if confirmations < 1 {
			// Block not yet in chain (orphaned or propagating)
			continue
		}
		
		// Fetch block from node to verify it's still in main chain
		blockHash, err := b.rpcClient.GetBlockHash(block.Height)
		if err != nil {
			log.Error().Err(err).Int64("height", block.Height).Msg("Failed to get block hash")
			continue
		}
		
		// Check if our block is orphaned
		if blockHash != block.Hash {
			log.Warn().Str("hash", block.Hash).Int64("height", block.Height).Msg("Block orphaned")
			b.pgStore.UpdateBlockStatus(block.Hash, "orphaned", confirmations)
			continue
		}
		
		// Update confirmations
		if block.Status == "pending" && confirmations >= 1 {
			b.pgStore.UpdateBlockStatus(block.Hash, "confirming", confirmations)
			log.Info().Str("hash", block.Hash).Int("confirmations", confirmations).Msg("Block confirming")
		}
		
		// Credit rewards when fully confirmed
		if confirmations >= b.confirmDepth && block.Status != "confirmed" {
			if err := b.creditBlockRewards(block); err != nil {
				log.Error().Err(err).Str("hash", block.Hash).Msg("Failed to credit rewards")
				continue
			}
			
			b.pgStore.UpdateBlockStatus(block.Hash, "confirmed", confirmations)
			log.Info().
				Str("hash", block.Hash).
				Int64("height", block.Height).
				Int64("reward", block.Reward).
				Msg("Block confirmed, rewards credited")
		}
	}
	
	return nil
}

func (b *BlockConfirmationLoop) creditBlockRewards(block storage.Block) error {
	// Get shares for this block's round
	// For PPLNS: last N shares before block
	// For PROP: all shares in current round
	
	shares, err := b.pgStore.GetPendingShares(b.calculator.GetPPLNSWindow())
	if err != nil {
		return err
	}
	
	// Convert to accounting.ShareRecord
	var shareRecords []accounting.ShareRecord
	for _, share := range shares {
		shareRecords = append(shareRecords, accounting.ShareRecord{
			Address:    share.Address,
			Difficulty: share.Difficulty,
			Height:     share.Height,
		})
	}
	
	// Calculate rewards based on mode
	var rewards []accounting.Reward
	
	// TODO: Check if block finder was solo mining
	isSolo := false // Get from block.Finder metadata
	
	if isSolo {
		rewards = b.calculator.CalculateSolo(block.Reward, block.Finder)
	} else {
		// Use pool reward mode
		rewards = b.calculator.CalculatePPLNS(block.Reward, shareRecords)
	}
	
	// Credit balances
	for _, reward := range rewards {
		if err := b.pgStore.CreditBalance(reward.Address, reward.Amount, block.ID); err != nil {
			log.Error().Err(err).Str("address", reward.Address).Int64("amount", reward.Amount).Msg("Failed to credit balance")
			continue
		}
	}
	
	// Mark shares as credited
	var shareIDs []int64
	for _, share := range shares {
		shareIDs = append(shareIDs, share.ID)
	}
	b.pgStore.MarkSharesCredited(shareIDs)
	
	log.Info().Int("recipients", len(rewards)).Int64("block_id", block.ID).Msg("Rewards credited")
	return nil
}
