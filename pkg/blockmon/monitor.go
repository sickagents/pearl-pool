package blockmon

import (
	"context"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

// Monitor watches pending blocks and updates their status
type Monitor struct {
	rpcClient     *rpc.Client
	pgStore       *storage.PostgresStore
	confirmDepth  int
	pollInterval  time.Duration
}

func NewMonitor(rpc *rpc.Client, pg *storage.PostgresStore, confirmDepth int, interval time.Duration) *Monitor {
	return &Monitor{
		rpcClient:    rpc,
		pgStore:      pg,
		confirmDepth: confirmDepth,
		pollInterval: interval,
	}
}

func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	
	log.Info().Msg("Block monitor started")
	
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Block monitor stopped")
			return
		case <-ticker.C:
			if err := m.checkPendingBlocks(); err != nil {
				log.Error().Err(err).Msg("Failed to check pending blocks")
			}
		}
	}
}

func (m *Monitor) checkPendingBlocks() error {
	blocks, err := m.pgStore.GetPendingBlocks()
	if err != nil {
		return err
	}
	
	if len(blocks) == 0 {
		return nil
	}
	
	currentHeight, err := m.rpcClient.GetBlockCount()
	if err != nil {
		return err
	}
	
	for _, block := range blocks {
		if err := m.checkBlock(&block, currentHeight); err != nil {
			log.Error().Err(err).Str("hash", block.Hash).Msg("Failed to check block")
			continue
		}
	}
	
	return nil
}

func (m *Monitor) checkBlock(block *storage.Block, currentHeight int64) error {
	// Get block info from node
	blockInfo, err := m.rpcClient.GetBlock(block.Hash, 1)
	if err != nil {
		// Block not found - likely orphaned
		log.Warn().Str("hash", block.Hash).Msg("Block not found on chain, marking as orphaned")
		return m.pgStore.UpdateBlockStatus(block.Hash, "orphaned", 0)
	}
	
	confirmations := int(currentHeight - blockInfo.Height + 1)
	
	if confirmations < 0 {
		confirmations = 0
	}
	
	var newStatus string
	switch {
	case confirmations == 0:
		newStatus = "pending"
	case confirmations < m.confirmDepth:
		newStatus = "confirming"
	case confirmations >= m.confirmDepth:
		newStatus = "confirmed"
	}
	
	// Update status
	if err := m.pgStore.UpdateBlockStatus(block.Hash, newStatus, confirmations); err != nil {
		return err
	}
	
	// If newly confirmed, credit rewards
	if newStatus == "confirmed" && block.Status != "confirmed" {
		log.Info().Str("hash", block.Hash).Int64("height", block.Height).Msg("Block confirmed, crediting rewards")
		// TODO: Trigger reward distribution
	}
	
	return nil
}
