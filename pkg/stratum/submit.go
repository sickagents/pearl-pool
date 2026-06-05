package stratum

import (
	"encoding/json"
	"fmt"
	
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

// UpdateHandleSubmit replaces the TODO in server.go
func (s *Server) handleSubmitV2(conn *Connection, msg *StratumMessage) {
	if !conn.authorized {
		s.sendError(conn, msg.ID, 24, "Not authorized")
		return
	}
	
	if len(msg.Params) < 5 {
		s.sendError(conn, msg.ID, 24, "Invalid params")
		return
	}
	
	// Params: [worker_name, job_id, extranonce2, ntime, nonce]
	workerName, _ := msg.Params[0].(string)
	jobID, ok := msg.Params[1].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid job_id")
		return
	}
	
	extraNonce2, ok := msg.Params[2].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid extranonce2")
		return
	}
	
	nTime, ok := msg.Params[3].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid ntime")
		return
	}
	
	nonce, ok := msg.Params[4].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid nonce")
		return
	}
	
	// Get job
	job, exists := s.jobManager.GetJob(jobID)
	if !exists {
		s.sendError(conn, msg.ID, 21, "Job not found")
		conn.sharesRejected++
		return
	}
	
	// Check if stale
	if s.isStaleJob(job) {
		s.sendError(conn, msg.ID, 21, "Stale share")
		conn.sharesRejected++
		return
	}
	
	// Check duplicate
	if s.isDuplicateShare(conn, jobID, extraNonce2, nonce) {
		s.sendError(conn, msg.ID, 22, "Duplicate share")
		conn.sharesRejected++
		return
	}
	
	// Validate share
	result, err := s.ValidateShare(conn, job, extraNonce2, nTime, nonce)
	if err != nil {
		log.Error().Err(err).Str("conn_id", conn.ID).Msg("Share validation error")
		s.sendError(conn, msg.ID, 20, fmt.Sprintf("Validation failed: %s", err.Error()))
		conn.sharesRejected++
		return
	}
	
	if !result.Valid {
		s.sendError(conn, msg.ID, 23, "Invalid share (low difficulty)")
		conn.sharesRejected++
		return
	}
	
	// Accept share
	s.sendResult(conn, msg.ID, true)
	conn.sharesSubmitted++
	conn.sharesAccepted++
	conn.UpdateActivity()
	
	// Record to storage
	if s.pgStore != nil {
		worker := workerName
		if worker == "" {
			worker = "default"
		}
		
		err := s.pgStore.RecordShare(
			conn.address,
			worker,
			conn.difficulty,
			job.Height,
			result.IsBlock,
			result.BlockHash,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to record share to database")
		}
	}
	
	// Update Redis stats
	if s.redisStore != nil {
		s.redisStore.IncrShareCount(conn.address)
		s.redisStore.SetWorkerOnline(conn.address, workerName)
	}
	
	// If block found, submit to node
	if result.IsBlock {
		log.Info().
			Str("address", conn.address).
			Str("block_hash", result.BlockHash).
			Int64("height", job.Height).
			Msg("BLOCK FOUND!")
		
		// Submit block via RPC
		go s.submitBlock(result.BlockHash, job)
	}
	
	log.Debug().
		Str("conn_id", conn.ID).
		Str("address", conn.address).
		Str("job_id", jobID).
		Float64("difficulty", conn.difficulty).
		Bool("is_block", result.IsBlock).
		Msg("Share accepted")
}

func (s *Server) submitBlock(blockHex string, job *Job) {
	if s.rpcClient == nil {
		log.Error().Msg("RPC client not configured, cannot submit block")
		return
	}
	
	err := s.rpcClient.SubmitBlock(blockHex)
	if err != nil {
		log.Error().Err(err).Str("block_hash", blockHex).Msg("Block submission failed")
		
		// Update block status to rejected
		if s.pgStore != nil {
			// We need block hash from the hex, for now skip
			// TODO: Parse block hash from header
		}
		return
	}
	
	log.Info().Str("block_hash", blockHex).Int64("height", job.Height).Msg("Block accepted by network")
	
	// Record block in database
	if s.pgStore != nil {
		err := s.pgStore.RecordBlock(blockHex, job.Height, job.CoinbaseValue, "finder_address")
		if err != nil {
			log.Error().Err(err).Msg("Failed to record block")
		}
	}
}

// Add storage references to Server struct
type ServerWithStorage struct {
	*Server
	pgStore    *storage.PostgresStore
	redisStore *storage.RedisStore
	rpcClient  interface{} // TODO: proper type
}
