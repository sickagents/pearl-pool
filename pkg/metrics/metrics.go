package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Pool-wide metrics
	PoolHashrate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pearl_pool_hashrate",
		Help: "Current pool hashrate in H/s",
	})
	
	PoolMiners = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pearl_pool_miners",
		Help: "Number of active miners",
	})
	
	PoolWorkers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pearl_pool_workers",
		Help: "Number of active workers",
	})
	
	BlocksFound = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_blocks_found_total",
		Help: "Total blocks found by the pool",
	})
	
	BlocksConfirmed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_blocks_confirmed_total",
		Help: "Total blocks confirmed",
	})
	
	BlocksOrphaned = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_blocks_orphaned_total",
		Help: "Total blocks orphaned",
	})
	
	// Share metrics
	SharesAccepted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_shares_accepted_total",
		Help: "Total accepted shares",
	}, []string{"port"})
	
	SharesRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_shares_rejected_total",
		Help: "Total rejected shares",
	}, []string{"port", "reason"})
	
	SharesStale = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_shares_stale_total",
		Help: "Total stale shares",
	}, []string{"port"})
	
	SharesDuplicate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_shares_duplicate_total",
		Help: "Total duplicate shares",
	}, []string{"port"})
	
	// Connection metrics
	ConnectionsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pearl_pool_connections_active",
		Help: "Number of active connections",
	}, []string{"port"})
	
	ConnectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_connections_total",
		Help: "Total connections",
	}, []string{"port"})
	
	// Job metrics
	JobsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_jobs_created_total",
		Help: "Total jobs created",
	})
	
	JobsBroadcast = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_jobs_broadcast_total",
		Help: "Total jobs broadcast to miners",
	})
	
	// Payout metrics
	PayoutsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_payouts_processed_total",
		Help: "Total payouts processed",
	})
	
	PayoutAmount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_payout_amount_total",
		Help: "Total amount paid out in satoshis",
	})
	
	PayoutsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pearl_pool_payouts_failed_total",
		Help: "Total failed payouts",
	})
	
	// RPC metrics
	RPCCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_rpc_calls_total",
		Help: "Total RPC calls to PEARL node",
	}, []string{"method"})
	
	RPCErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pearl_pool_rpc_errors_total",
		Help: "Total RPC errors",
	}, []string{"method"})
	
	RPCDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pearl_pool_rpc_duration_seconds",
		Help:    "RPC call duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
)

// RecordShareAccepted increments accepted share counter
func RecordShareAccepted(port string) {
	SharesAccepted.WithLabelValues(port).Inc()
}

// RecordShareRejected increments rejected share counter
func RecordShareRejected(port, reason string) {
	SharesRejected.WithLabelValues(port, reason).Inc()
}

// RecordShareStale increments stale share counter
func RecordShareStale(port string) {
	SharesStale.WithLabelValues(port).Inc()
}

// RecordShareDuplicate increments duplicate share counter
func RecordShareDuplicate(port string) {
	SharesDuplicate.WithLabelValues(port).Inc()
}

// RecordConnection increments connection counter
func RecordConnection(port string) {
	ConnectionsTotal.WithLabelValues(port).Inc()
}

// UpdateActiveConnections sets active connection gauge
func UpdateActiveConnections(port string, count int) {
	ConnectionsActive.WithLabelValues(port).Set(float64(count))
}

// RecordBlockFound increments block found counter
func RecordBlockFound() {
	BlocksFound.Inc()
}

// RecordBlockConfirmed increments confirmed block counter
func RecordBlockConfirmed() {
	BlocksConfirmed.Inc()
}

// RecordBlockOrphaned increments orphaned block counter
func RecordBlockOrphaned() {
	BlocksOrphaned.Inc()
}

// RecordJob increments job counter
func RecordJob() {
	JobsCreated.Inc()
}

// RecordJobBroadcast increments broadcast counter
func RecordJobBroadcast() {
	JobsBroadcast.Inc()
}

// RecordPayout increments payout counter and amount
func RecordPayout(amountSatoshis int64) {
	PayoutsProcessed.Inc()
	PayoutAmount.Add(float64(amountSatoshis))
}

// RecordPayoutFailed increments failed payout counter
func RecordPayoutFailed() {
	PayoutsFailed.Inc()
}

// RecordRPCCall records an RPC call
func RecordRPCCall(method string, duration float64) {
	RPCCalls.WithLabelValues(method).Inc()
	RPCDuration.WithLabelValues(method).Observe(duration)
}

// RecordRPCError records an RPC error
func RecordRPCError(method string) {
	RPCErrors.WithLabelValues(method).Inc()
}

// UpdatePoolHashrate sets pool hashrate gauge
func UpdatePoolHashrate(hashrate float64) {
	PoolHashrate.Set(hashrate)
}

// UpdatePoolMiners sets pool miners gauge
func UpdatePoolMiners(count int) {
	PoolMiners.Set(float64(count))
}

// UpdatePoolWorkers sets pool workers gauge
func UpdatePoolWorkers(count int) {
	PoolWorkers.Set(float64(count))
}
