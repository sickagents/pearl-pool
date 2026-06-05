package payout

import (
	"fmt"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

type Engine struct {
	rpcClient     *rpc.Client
	pgStore       *storage.PostgresStore
	minPayout     int64 // in satoshis
	maxBatchSize  int
	gasBuffer     int64
	enabled       bool
}

func NewEngine(rpcClient *rpc.Client, pgStore *storage.PostgresStore, minPayout float64, maxBatch int, gasBuffer float64) *Engine {
	return &Engine{
		rpcClient:    rpcClient,
		pgStore:      pgStore,
		minPayout:    int64(minPayout * 1e8), // PEARL to satoshis
		maxBatchSize: maxBatch,
		gasBuffer:    int64(gasBuffer * 1e8),
		enabled:      true,
	}
}

func (e *Engine) Run() error {
	if !e.enabled {
		log.Info().Msg("Payout engine disabled")
		return nil
	}
	
	balances, err := e.pgStore.GetPayoutDue(e.minPayout)
	if err != nil {
		return fmt.Errorf("failed to get payout due: %w", err)
	}
	
	if len(balances) == 0 {
		log.Info().Msg("No payouts due")
		return nil
	}
	
	log.Info().Int("count", len(balances)).Msg("Processing payouts")
	
	// Batch payouts
	for i := 0; i < len(balances); i += e.maxBatchSize {
		end := i + e.maxBatchSize
		if end > len(balances) {
			end = len(balances)
		}
		
		batch := balances[i:end]
		if err := e.processBatch(batch); err != nil {
			log.Error().Err(err).Int("batch_start", i).Msg("Batch payout failed")
			continue
		}
	}
	
	return nil
}

func (e *Engine) processBatch(balances []storage.Balance) error {
	amounts := make(map[string]float64)
	
	for _, bal := range balances {
		amountPEARL := float64(bal.Balance) / 1e8
		amounts[bal.Address] = amountPEARL
	}
	
	txid, err := e.rpcClient.SendMany("", amounts)
	if err != nil {
		return fmt.Errorf("sendmany failed: %w", err)
	}
	
	for _, bal := range balances {
		if err := e.pgStore.RecordPayout(bal.Address, bal.Balance, txid); err != nil {
			log.Error().Err(err).Str("address", bal.Address).Msg("Failed to record payout")
		}
	}
	
	log.Info().Str("txid", txid).Int("recipients", len(balances)).Msg("Payout sent")
	return nil
}

func (e *Engine) StartScheduled(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := e.Run(); err != nil {
			log.Error().Err(err).Msg("Scheduled payout failed")
		}
	}
}
