package pool

import (
	"context"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

type BlockMonitor struct {
	rpcClient         *rpc.Client
	pgStore           *storage.PostgresStore
	confirmationDepth int
	pollInterval      time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
}

func NewBlockMonitor(rpcClient *rpc.Client, pgStore *storage.PostgresStore, confirmDepth int, interval time.Duration) *BlockMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockMonitor{
		rpcClient:         rpcClient,
		pgStore:           pgStore,
		confirmationDepth: confirmDepth,
		pollInterval:      interval,
		ctx:               ctx,
		cancel:            cancel,
	}
}

func (bm *BlockMonitor) Start() {
	go bm.run()
}

func (bm *BlockMonitor) Stop() {
	bm.cancel()
}

func (bm *BlockMonitor) run() {
	ticker := time.NewTicker(bm.pollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-bm.ctx.Done():
			return
		case <-ticker.C:
			if err := bm.checkPendingBlocks(); err != nil {
				log.Error().Err(err).Msg("Block monitor check failed")
			}
		}
	}
}

func (bm *BlockMonitor) checkPendingBlocks() error {
	blocks, err := bm.pgStore.GetPendingBlocks()
	if err != nil {
		return err
	}
	
	if len(blocks) == 0 {
		return nil
	}
	
	currentHeight, err := bm.rpcClient.GetBlockCount()
	if err != nil {
		return err
	}
	
	for _, block := range blocks {
		confirmations := int(currentHeight - block.Height + 1)
		
		if confirmations < 0 {
			// Block not yet in chain (orphaned or reorg)
			if err := bm.pgStore.UpdateBlockStatus(block.Hash, "orphaned", 0); err != nil {
				log.Error().Err(err).Str("hash", block.Hash).Msg("Failed to mark block orphaned")
			}
			continue
		}
		
		if confirmations < bm.confirmationDepth {
			// Still confirming
			if err := bm.pgStore.UpdateBlockStatus(block.Hash, "confirming", confirmations); err != nil {
				log.Error().Err(err).Str("hash", block.Hash).Msg("Failed to update confirmations")
			}
			continue
		}
		
		// Fully confirmed - credit rewards
		if err := bm.pgStore.UpdateBlockStatus(block.Hash, "confirmed", confirmations); err != nil {
			log.Error().Err(err).Str("hash", block.Hash).Msg("Failed to confirm block")
			continue
		}
		
		log.Info().Str("hash", block.Hash).Int64("height", block.Height).Int("confirmations", confirmations).Msg("Block confirmed")
	}
	
	return nil
}
