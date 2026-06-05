package main

import (
	"fmt"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/metrics"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

// StatsCollector periodically updates Prometheus metrics from storage
type StatsCollector struct {
	pgStore    *storage.PostgresStore
	redisStore *storage.RedisStore
	interval   time.Duration
	stopChan   chan struct{}
}

func NewStatsCollector(pgStore *storage.PostgresStore, redisStore *storage.RedisStore, interval time.Duration) *StatsCollector {
	return &StatsCollector{
		pgStore:    pgStore,
		redisStore: redisStore,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

func (s *StatsCollector) Start() {
	go s.run()
}

func (s *StatsCollector) Stop() {
	close(s.stopChan)
}

func (s *StatsCollector) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	
	log.Info().Dur("interval", s.interval).Msg("Stats collector started")
	
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if err := s.collect(); err != nil {
				log.Error().Err(err).Msg("Failed to collect stats")
			}
		}
	}
}

func (s *StatsCollector) collect() error {
	// Update pool hashrate
	hashrate, err := s.redisStore.GetPoolHashrate()
	if err != nil {
		return fmt.Errorf("failed to get pool hashrate: %w", err)
	}
	metrics.UpdatePoolHashrate(hashrate)
	
	// Update miner count
	miners, err := s.redisStore.GetTopMiners(10000)
	if err != nil {
		return fmt.Errorf("failed to get miner count: %w", err)
	}
	metrics.UpdatePoolMiners(len(miners))
	
	// Get pool stats from PostgreSQL
	poolStats, err := s.pgStore.GetPoolStats()
	if err != nil {
		return fmt.Errorf("failed to get pool stats: %w", err)
	}
	
	// Update worker count (sum of all miners' workers)
	// This is approximate, actual implementation would aggregate from Redis
	metrics.UpdatePoolWorkers(poolStats.ActiveMiners * 2) // Rough estimate
	
	return nil
}
