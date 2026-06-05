package stratum

import (
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Connection struct {
	ID               string
	netConn          net.Conn
	mu               sync.Mutex
	address          string
	worker           string
	authorized       bool
	extraNonce1      string
	extraNonce2Size  int
	difficulty       float64
	isSolo           bool
	sharesSubmitted  int64
	sharesAccepted   int64
	sharesRejected   int64
	lastActivity     time.Time
	connectedAt      time.Time
}

func NewConnection(netConn net.Conn, difficulty float64) *Connection {
	return &Connection{
		ID:           uuid.New().String(),
		netConn:      netConn,
		difficulty:   difficulty,
		connectedAt:  time.Now(),
		lastActivity: time.Now(),
	}
}

func (c *Connection) RemoteAddr() string {
	return c.netConn.RemoteAddr().String()
}

func (c *Connection) Close() error {
	return c.netConn.Close()
}

func (c *Connection) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastActivity = time.Now()
}
