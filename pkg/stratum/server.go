package stratum

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Server struct {
	port       int
	listener   net.Listener
	conns      map[string]*Connection
	connsMu    sync.RWMutex
	jobManager *JobManager
	difficulty float64
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
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
	}
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
	defer func() {
		conn.Close()
		s.connsMu.Lock()
		delete(s.conns, conn.ID)
		s.connsMu.Unlock()
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
	jobID, ok := msg.Params[1].(string)
	if !ok {
		s.sendError(conn, msg.ID, 24, "Invalid job_id")
		return
	}
	
	job, exists := s.jobManager.GetJob(jobID)
	if !exists {
		s.sendError(conn, msg.ID, 21, "Job not found")
		return
	}
	
	// TODO: Validate share (check nonce, extranonce2, compute hash)
	// TODO: Check if meets pool difficulty
	// TODO: Check if meets network difficulty (block found)
	// TODO: Submit to accounting system
	
	s.sendResult(conn, msg.ID, true)
	
	conn.sharesSubmitted++
	log.Debug().Str("conn_id", conn.ID).Str("job_id", jobID).Msg("Share submitted")
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
