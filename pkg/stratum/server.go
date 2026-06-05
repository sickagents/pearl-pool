package stratum

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pearl-mining/pearl-pool/pkg/rpc"
	"github.com/pearl-mining/pearl-pool/pkg/storage"
	"github.com/rs/zerolog/log"
)

type Server struct {
	port       int
	listener   net.Listener
	conns      map[string]*Connection
	connsMu    sync.RWMutex
	jobManager *JobManager
	difficulty float64
	pgStore    *storage.PostgresStore
	rpcClient  *rpc.Client
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewServer(port int, difficulty float64, jobManager *JobManager, pgStore *storage.PostgresStore, rpcClient *rpc.Client) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		port:       port,
		conns:      make(map[string]*Connection),
		jobManager: jobManager,
		difficulty: difficulty,
		pgStore:    pgStore,
		rpcClient:  rpcClient,
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
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
	s.wg.Wait()
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
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Error().Err(err).Msg("Accept error")
				continue
			}
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
	defer func() {
		conn.netConn.Close()
		s.connsMu.Lock()
		delete(s.conns, conn.ID)
		s.connsMu.Unlock()
		log.Info().Str("conn_id", conn.ID).Msg("Connection closed")
	}()
	
	reader := bufio.NewReader(conn.netConn)
	
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		
		conn.netConn.SetReadDeadline(time.Now().Add(300 * time.Second))
		
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		
		var msg StratumMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Warn().Str("conn_id", conn.ID).Err(err).Msg("Invalid JSON")
			continue
		}
		
		s.handleMessage(conn, &msg)
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
		s.sendError(conn, msg.ID, 20, "Unknown method")
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
	
	nonce, ok := msg.Params[4].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid nonce")
		return
	}
	
	// Validate share
	validation, err := s.validateShare(conn, jobID, extraNonce2, nTime, nonce)
	if err != nil {
		s.sendError(conn, msg.ID, 20, fmt.Sprintf("Invalid share: %s", err.Error()))
		conn.sharesRejected++
		log.Warn().Str("conn_id", conn.ID).Str("address", conn.address).Err(err).Msg("Share rejected")
		return
	}
	
	if !validation.Valid {
		s.sendError(conn, msg.ID, 23, "Low difficulty share")
		conn.sharesRejected++
		return
	}
	
	// Accept share
	s.sendResult(conn, msg.ID, true)
	conn.sharesAccepted++
	conn.sharesSubmitted++
	
	// Record to storage
	if s.pgStore != nil {
		worker := workerName
		if worker == "" {
			worker = "default"
		}
		
		err := s.pgStore.RecordShare(
			conn.address,
			worker,
			validation.Difficulty,
			validation.BlockHeight,
			validation.IsBlock,
			validation.BlockHash,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to record share")
		}
	}
	
	// Block found
	if validation.IsBlock {
		log.Info().
			Str("address", conn.address).
			Str("block_hash", validation.BlockHash).
			Int64("height", validation.BlockHeight).
			Msg("BLOCK FOUND!")
		
		// Submit to node
		if s.rpcClient != nil {
			if err := s.rpcClient.SubmitBlock(validation.BlockHex); err != nil {
				log.Error().Err(err).Str("hash", validation.BlockHash).Msg("Block submission failed")
			} else {
				log.Info().Str("hash", validation.BlockHash).Msg("Block submitted to network")
			}
		}
		
		// Record block
		if s.pgStore != nil {
			// TODO: Get actual block reward from template or RPC
			reward := int64(100 * 1e8) // 100 PEARL placeholder
			err := s.pgStore.RecordBlock(validation.BlockHash, validation.BlockHeight, reward, conn.address)
			if err != nil {
				log.Error().Err(err).Msg("Failed to record block")
			}
		}
	}
	
	log.Debug().
		Str("conn_id", conn.ID).
		Str("address", conn.address).
		Str("job_id", jobID).
		Bool("is_block", validation.IsBlock).
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
	
	log.Info().Str("job_id", job.ID).Int64("height", job.Height).Msg("Broadcasted new job")
}
