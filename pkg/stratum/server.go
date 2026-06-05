package stratum

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/metrics"
	"github.com/rs/zerolog/log"
)

type Server struct {
	port        int
	listener    net.Listener
	conns       map[string]*Connection
	connsMu     sync.RWMutex
	jobManager  *JobManager
	difficulty  float64
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	dupChecker  *DuplicateShareChecker
	rpcClient   interface{} // Will be *rpc.Client
	pgStore     interface{} // Will be *storage.PostgresStore
	redisStore  interface{} // Will be *storage.RedisStore
}

func NewServer(port int, difficulty float64, jobManager *JobManager) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		port:       port,
		conns:      make(map[string]*Connection),
		jobManager: jobManager,
		difficulty: difficulty,
		ctx:        ctx,
		cancel:     cancel,
		dupChecker: NewDuplicateShareChecker(),
	}
}

// SetRPCClient sets the RPC client for block submission
func (s *Server) SetRPCClient(client interface{}) {
	s.rpcClient = client
}

// SetPgStore sets the PostgreSQL store
func (s *Server) SetPgStore(store interface{}) {
	s.pgStore = store
}

// SetRedisStore sets the Redis store
func (s *Server) SetRedisStore(store interface{}) {
	s.redisStore = store
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}
	s.listener = listener
	
	log.Info().Int("port", s.port).Float64("difficulty", s.difficulty).Msg("Stratum server started")
	
	s.wg.Add(1)
	go s.acceptLoop()
	
	return nil
}

func (s *Server) Stop() {
	s.cancel()
	if s.listener != nil {
		s.listener.Close()
	}
	
	s.connsMu.Lock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.connsMu.Unlock()
	
	s.wg.Wait()
	log.Info().Msg("Stratum server stopped")
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		
		netConn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Msg("Accept failed")
			continue
		}
		
		conn := NewConnection(netConn, s.difficulty)
		
		s.connsMu.Lock()
		s.conns[conn.ID] = conn
		s.connsMu.Unlock()
		
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn *Connection) {
	defer s.wg.Done()
	
	// Record connection
	metrics.RecordConnection(strconv.Itoa(s.port))
	
	defer func() {
		conn.Close()
		s.connsMu.Lock()
		delete(s.conns, conn.ID)
		activeCount := len(s.conns)
		s.connsMu.Unlock()
		
		// Update active connections
		metrics.UpdateActiveConnections(strconv.Itoa(s.port), activeCount)
	}()
	
	log.Info().Str("conn_id", conn.ID).Str("remote", conn.RemoteAddr()).Msg("New connection")
	
	scanner := bufio.NewScanner(conn.netConn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		
		var msg StratumMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Warn().Str("conn_id", conn.ID).Err(err).Str("line", line).Msg("Invalid JSON")
			continue
		}
		
		s.handleMessage(conn, &msg)
	}
	
	if err := scanner.Err(); err != nil {
		log.Error().Str("conn_id", conn.ID).Err(err).Msg("Scanner error")
	}
}

func (s *Server) handleMessage(conn *Connection, msg *StratumMessage) {
	switch msg.Method {
	case "mining.subscribe":
		s.handleSubscribe(conn, msg)
	case "mining.authorize":
		s.handleAuthorize(conn, msg)
	case "mining.submit":
		s.handleSubmit(conn, msg)
	case "mining.extranonce.subscribe":
		s.handleExtraNonceSubscribe(conn, msg)
	default:
		s.sendError(conn, msg.ID, 20, fmt.Sprintf("Unknown method: %s", msg.Method))
	}
}

func (s *Server) handleSubscribe(conn *Connection, msg *StratumMessage) {
	conn.extraNonce1 = generateExtraNonce1()
	conn.extraNonce2Size = 4
	
	result := []interface{}{
		[][]string{
			{"mining.notify", conn.ID},
		},
		conn.extraNonce1,
		conn.extraNonce2Size,
	}
	
	s.sendResult(conn, msg.ID, result)
	log.Info().Str("conn_id", conn.ID).Str("extranonce1", conn.extraNonce1).Msg("Subscribed")
}

func (s *Server) handleAuthorize(conn *Connection, msg *StratumMessage) {
	if len(msg.Params) < 1 {
		s.sendError(conn, msg.ID, 24, "Invalid params")
		return
	}
	
	username, ok := msg.Params[0].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Username must be string")
		return
	}
	
	address, isSolo := ParseAddress(username)
	if !ValidateAddress(address) {
		s.sendError(conn, msg.ID, 24, "Invalid address")
		return
	}
	
	conn.address = address
	conn.isSolo = isSolo
	conn.authorized = true
	
	s.sendResult(conn, msg.ID, true)
	
	// Send difficulty
	diffMsg := BuildSetDifficulty(conn.difficulty)
	s.sendMessage(conn, diffMsg)
	
	// Send current job
	job := s.jobManager.GetCurrentJob()
	if job != nil {
		notifyMsg := BuildMiningNotify(job, conn.extraNonce1)
		s.sendMessage(conn, notifyMsg)
	}
	
	log.Info().Str("conn_id", conn.ID).Str("address", address).Bool("solo", isSolo).Msg("Authorized")
}

func (s *Server) handleSubmit(conn *Connection, msg *StratumMessage) {
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
	
	nonceStr, ok := msg.Params[4].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid nonce")
		return
	}
	
	// Parse nonce
	var nonce uint32
	if _, err := fmt.Sscanf(nonceStr, "%x", &nonce); err != nil {
		s.sendError(conn, msg.ID, 24, "Invalid nonce format")
		return
	}
	
	job, exists := s.jobManager.GetJob(jobID)
	if !exists {
		s.sendError(conn, msg.ID, 21, "Job not found")
		return
	}
	
	// Check for duplicate share
	if s.dupChecker.Check(jobID, nonce, extraNonce2) {
		s.sendError(conn, msg.ID, 22, "Duplicate share")
		conn.sharesRejected++
		return
	}
	
	// Validate share
	validation, err := ValidateShare(job, nonce, extraNonce2, nTime)
	if err != nil {
		s.sendError(conn, msg.ID, 23, fmt.Sprintf("Invalid share: %v", err))
		conn.sharesRejected++
		log.Warn().Str("conn_id", conn.ID).Str("address", conn.address).Err(err).Msg("Share validation failed")
		return
	}
	
	if !validation.Valid {
		s.sendError(conn, msg.ID, 23, validation.ErrorReason)
		conn.sharesRejected++
		return
	}
	
	// Share is valid
	conn.sharesSubmitted++
	conn.sharesAccepted++
	
	// Record share to storage
	if s.pgStore != nil {
		err := s.pgStore.RecordShare(
			conn.address,
			workerName,
			conn.difficulty,
			job.Height,
			validation.MeetsDifficulty,
			validation.BlockHash,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to record share")
		}
	}
	
	// If meets network difficulty, we found a block!
	if validation.MeetsDifficulty {
		log.Info().
			Str("address", conn.address).
			Str("block_hash", validation.BlockHash).
			Int64("height", job.Height).
			Msg("BLOCK FOUND!")
		
		// Submit block to node (will be implemented in block submission handler)
		if s.rpcClient != nil {
			go s.submitBlock(job, validation)
		}
		
		// Record block in database
		if s.pgStore != nil {
			err := s.pgStore.RecordBlock(
				validation.BlockHash,
				job.Height,
				job.CoinbaseValue,
				conn.address,
			)
			if err != nil {
				log.Error().Err(err).Msg("Failed to record block")
			}
		}
	}
	
	// Update Redis stats
	if s.redisStore != nil {
		s.redisStore.IncrShareCount(conn.address)
		s.redisStore.SetWorkerOnline(conn.address, workerName)
	}
	
	s.sendResult(conn, msg.ID, true)
	
	log.Debug().
		Str("conn_id", conn.ID).
		Str("address", conn.address).
		Str("job_id", jobID).
		Bool("block", validation.MeetsDifficulty).
		Msg("Share accepted")
}

func (s *Server) handleExtraNonceSubscribe(conn *Connection, msg *StratumMessage) {
	s.sendResult(conn, msg.ID, true)
}

func (s *Server) sendResult(conn *Connection, id interface{}, result interface{}) {
	s.sendMessage(conn, &StratumMessage{
		ID:     id,
		Result: result,
		Error:  nil,
	})
}

func (s *Server) sendError(conn *Connection, id interface{}, code int, message string) {
	s.sendMessage(conn, &StratumMessage{
		ID:    id,
		Error: []interface{}{code, message, nil},
	})
}

func (s *Server) sendMessage(conn *Connection, msg *StratumMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal message")
		return
	}
	
	conn.mu.Lock()
	defer conn.mu.Unlock()
	
	_, err = conn.netConn.Write(append(data, '\n'))
	if err != nil {
		log.Error().Str("conn_id", conn.ID).Err(err).Msg("Failed to send message")
	}
}

func (s *Server) BroadcastJob(job *Job) {
	s.connsMu.RLock()
	defer s.connsMu.RUnlock()
	
	for _, conn := range s.conns {
		if !conn.authorized {
			continue
		}
		
		msg := BuildMiningNotify(job, conn.extraNonce1)
		s.sendMessage(conn, msg)
	}
	
	log.Info().Str("job_id", job.ID).Int("connections", len(s.conns)).Msg("Job broadcasted")
}

func generateExtraNonce1() string {
	return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
}
