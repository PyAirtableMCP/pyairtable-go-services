package routing

import (
	"errors"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNoHealthyInstances = errors.New("no healthy service instances available")
	ErrUnknownAlgorithm   = errors.New("unknown load balancing algorithm")
)

// LoadBalancer interface defines the contract for load balancing algorithms
type LoadBalancer interface {
	SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error)
	Name() string
}

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	counter uint64
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (rb *RoundRobinBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	index := atomic.AddUint64(&rb.counter, 1) % uint64(len(instances))
	return instances[index], nil
}

func (rb *RoundRobinBalancer) Name() string {
	return "round_robin"
}

// WeightedRoundRobinBalancer implements weighted round-robin load balancing
type WeightedRoundRobinBalancer struct {
	mu      sync.RWMutex
	weights map[string]int // instance ID -> current weight
}

func NewWeightedRoundRobinBalancer() *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		weights: make(map[string]int),
	}
}

func (wrb *WeightedRoundRobinBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	wrb.mu.Lock()
	defer wrb.mu.Unlock()

	// Initialize weights if needed
	for _, instance := range instances {
		if _, exists := wrb.weights[instance.ID]; !exists {
			wrb.weights[instance.ID] = 0
		}
	}

	// Find instance with highest current weight
	var selected *ServiceInstance
	maxCurrentWeight := -1

	for _, instance := range instances {
		currentWeight := wrb.weights[instance.ID]
		currentWeight += instance.Weight

		wrb.weights[instance.ID] = currentWeight

		if selected == nil || currentWeight > maxCurrentWeight {
			selected = instance
			maxCurrentWeight = currentWeight
		}
	}

	// Reduce the selected instance's weight by the total weight
	if selected != nil {
		totalWeight := 0
		for _, instance := range instances {
			totalWeight += instance.Weight
		}
		wrb.weights[selected.ID] -= totalWeight
	}

	return selected, nil
}

func (wrb *WeightedRoundRobinBalancer) Name() string {
	return "weighted_round_robin"
}

// LeastConnectionsBalancer selects the instance with the fewest active connections
type LeastConnectionsBalancer struct{}

func NewLeastConnectionsBalancer() *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{}
}

func (lcb *LeastConnectionsBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	var selected *ServiceInstance
	minConnections := int64(-1)

	for _, instance := range instances {
		// Use request count as a proxy for active connections
		connections := atomic.LoadInt64(&instance.RequestCount)
		
		if selected == nil || connections < minConnections {
			selected = instance
			minConnections = connections
		}
	}

	return selected, nil
}

func (lcb *LeastConnectionsBalancer) Name() string {
	return "least_connections"
}

// RandomBalancer selects a random instance
type RandomBalancer struct {
	rand *rand.Rand
	mu   sync.Mutex
}

func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (rb *RandomBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	index := rb.rand.Intn(len(instances))
	return instances[index], nil
}

func (rb *RandomBalancer) Name() string {
	return "random"
}

// LatencyBasedBalancer selects the instance with the lowest response time
type LatencyBasedBalancer struct{}

func NewLatencyBasedBalancer() *LatencyBasedBalancer {
	return &LatencyBasedBalancer{}
}

func (lbb *LatencyBasedBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	var selected *ServiceInstance
	minLatency := int64(-1)

	for _, instance := range instances {
		latency := atomic.LoadInt64(&instance.ResponseTime)
		
		if selected == nil || (latency > 0 && (minLatency == -1 || latency < minLatency)) {
			selected = instance
			minLatency = latency
		}
	}

	// If no instance has recorded latency yet, fall back to round-robin
	if selected == nil || minLatency == -1 {
		return instances[0], nil
	}

	return selected, nil
}

func (lbb *LatencyBasedBalancer) Name() string {
	return "latency"
}

// HealthScoreBalancer considers both latency and error rate
type HealthScoreBalancer struct{}

func NewHealthScoreBalancer() *HealthScoreBalancer {
	return &HealthScoreBalancer{}
}

func (hsb *HealthScoreBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	type instanceScore struct {
		instance *ServiceInstance
		score    float64
	}

	scores := make([]instanceScore, 0, len(instances))

	for _, instance := range instances {
		// Calculate health score (lower is better)
		// Score = (normalized_latency * 0.7) + (error_rate * 0.3)
		
		latency := float64(atomic.LoadInt64(&instance.ResponseTime))
		errorRate := instance.GetErrorRate()
		
		// Normalize latency (assume max reasonable latency is 1000ms)
		normalizedLatency := latency / 1000.0
		if normalizedLatency > 1.0 {
			normalizedLatency = 1.0
		}
		
		score := (normalizedLatency * 0.7) + (errorRate * 0.3)
		
		scores = append(scores, instanceScore{
			instance: instance,
			score:    score,
		})
	}

	// Sort by score (ascending - lower score is better)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score < scores[j].score
	})

	return scores[0].instance, nil
}

func (hsb *HealthScoreBalancer) Name() string {
	return "health_score"
}

// LoadBalancerFactory creates load balancers based on algorithm name
func NewLoadBalancer(algorithm string) (LoadBalancer, error) {
	switch algorithm {
	case "round_robin":
		return NewRoundRobinBalancer(), nil
	case "weighted_round_robin", "weighted":
		return NewWeightedRoundRobinBalancer(), nil
	case "least_connections":
		return NewLeastConnectionsBalancer(), nil
	case "random":
		return NewRandomBalancer(), nil
	case "latency":
		return NewLatencyBasedBalancer(), nil
	case "health_score":
		return NewHealthScoreBalancer(), nil
	default:
		return nil, ErrUnknownAlgorithm
	}
}

// MultiAlgorithmBalancer can switch between different algorithms based on conditions
type MultiAlgorithmBalancer struct {
	primary   LoadBalancer
	fallback  LoadBalancer
	threshold int // minimum number of instances to use primary algorithm
}

func NewMultiAlgorithmBalancer(primary, fallback LoadBalancer, threshold int) *MultiAlgorithmBalancer {
	return &MultiAlgorithmBalancer{
		primary:   primary,
		fallback:  fallback,
		threshold: threshold,
	}
}

func (mab *MultiAlgorithmBalancer) SelectInstance(instances []*ServiceInstance) (*ServiceInstance, error) {
	if len(instances) == 0 {
		return nil, ErrNoHealthyInstances
	}

	// Use fallback algorithm if we don't have enough instances
	if len(instances) < mab.threshold {
		return mab.fallback.SelectInstance(instances)
	}

	return mab.primary.SelectInstance(instances)
}

func (mab *MultiAlgorithmBalancer) Name() string {
	return "multi_algorithm"
}