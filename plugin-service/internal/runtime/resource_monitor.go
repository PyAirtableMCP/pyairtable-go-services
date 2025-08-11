package runtime

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pyairtable/go-services/plugin-service/internal/models"
)

// ResourceMonitor provides global resource monitoring
type ResourceMonitor struct {
	mu                    sync.RWMutex
	trackers              map[string]*ResourceTracker
	globalMemoryUsage     int64
	globalCPUUsage        int64
	globalNetworkRequests int64
	started               bool
	stopChan              chan struct{}
}

// ResourceTracker tracks resource usage for a specific plugin execution
type ResourceTracker struct {
	limits          models.ResourceLimits
	startTime       time.Time
	memoryUsage     int64
	cpuTime         time.Duration
	networkRequests int32
	storageUsage    int64
	violationChan   chan struct{}
	stopChan        chan struct{}
	stopped         int32
	mu              sync.RWMutex
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		trackers: make(map[string]*ResourceTracker),
		stopChan: make(chan struct{}),
	}
}

// Start starts the global resource monitor
func (rm *ResourceMonitor) Start() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.started {
		return
	}

	rm.started = true
	go rm.monitorGlobalResources()
}

// Stop stops the global resource monitor
func (rm *ResourceMonitor) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.started {
		return
	}

	close(rm.stopChan)
	rm.started = false
}

// NewResourceTracker creates a new resource tracker for a plugin execution
func NewResourceTracker(limits models.ResourceLimits) *ResourceTracker {
	return &ResourceTracker{
		limits:        limits,
		startTime:     time.Now(),
		violationChan: make(chan struct{}, 1),
		stopChan:      make(chan struct{}),
	}
}

// Start starts resource tracking
func (rt *ResourceTracker) Start() {
	go rt.monitorResources()
}

// Stop stops resource tracking
func (rt *ResourceTracker) Stop() {
	if atomic.CompareAndSwapInt32(&rt.stopped, 0, 1) {
		close(rt.stopChan)
	}
}

// GetUsage returns current resource usage
func (rt *ResourceTracker) GetUsage() ResourceUsage {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	return ResourceUsage{
		MemoryMB:        int32(rt.memoryUsage / (1024 * 1024)),
		CPUTimeMs:       int32(rt.cpuTime.Milliseconds()),
		NetworkRequests: rt.networkRequests,
		StorageMB:       int32(rt.storageUsage / (1024 * 1024)),
	}
}

// ViolationChan returns a channel that signals resource limit violations
func (rt *ResourceTracker) ViolationChan() <-chan struct{} {
	return rt.violationChan
}

// RecordNetworkRequest records a network request
func (rt *ResourceTracker) RecordNetworkRequest() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.networkRequests++
	if rt.networkRequests > rt.limits.MaxNetworkReqs {
		select {
		case rt.violationChan <- struct{}{}:
		default:
		}
		return fmt.Errorf("network request limit exceeded: %d/%d", 
			rt.networkRequests, rt.limits.MaxNetworkReqs)
	}

	return nil
}

// RecordStorageUsage records storage usage
func (rt *ResourceTracker) RecordStorageUsage(bytes int64) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.storageUsage += bytes
	storageMB := rt.storageUsage / (1024 * 1024)
	if storageMB > int64(rt.limits.MaxStorageMB) {
		select {
		case rt.violationChan <- struct{}{}:
		default:
		}
		return fmt.Errorf("storage limit exceeded: %dMB/%dMB", 
			storageMB, rt.limits.MaxStorageMB)
	}

	return nil
}

// monitorResources monitors resource usage in a goroutine
func (rt *ResourceTracker) monitorResources() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastCPUTime time.Duration
	startCPUTime := getCPUTime()

	for {
		select {
		case <-rt.stopChan:
			return
		case <-ticker.C:
			// Check execution time limit
			if elapsed := time.Since(rt.startTime); elapsed.Milliseconds() > int64(rt.limits.MaxExecutionMs) {
				select {
				case rt.violationChan <- struct{}{}:
				default:
				}
				return
			}

			// Check memory usage
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			rt.mu.Lock()
			rt.memoryUsage = int64(m.Alloc)
			memoryMB := rt.memoryUsage / (1024 * 1024)
			rt.mu.Unlock()

			if memoryMB > int64(rt.limits.MaxMemoryMB) {
				select {
				case rt.violationChan <- struct{}{}:
				default:
				}
				return
			}

			// Check CPU usage (simplified)
			currentCPUTime := getCPUTime()
			cpuDelta := currentCPUTime - startCPUTime
			if cpuDelta > lastCPUTime {
				rt.mu.Lock()
				rt.cpuTime = cpuDelta
				rt.mu.Unlock()
			}
			lastCPUTime = cpuDelta
		}
	}
}

// monitorGlobalResources monitors global resource usage
func (rm *ResourceMonitor) monitorGlobalResources() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.collectGlobalMetrics()
		}
	}
}

// collectGlobalMetrics collects global resource metrics
func (rm *ResourceMonitor) collectGlobalMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	atomic.StoreInt64(&rm.globalMemoryUsage, int64(m.Alloc))

	// Update global CPU usage (simplified)
	cpuTime := getCPUTime()
	atomic.StoreInt64(&rm.globalCPUUsage, int64(cpuTime.Milliseconds()))
}

// GetGlobalMetrics returns global resource metrics
func (rm *ResourceMonitor) GetGlobalMetrics() GlobalResourceMetrics {
	return GlobalResourceMetrics{
		MemoryUsageMB:     atomic.LoadInt64(&rm.globalMemoryUsage) / (1024 * 1024),
		CPUUsageMs:        atomic.LoadInt64(&rm.globalCPUUsage),
		NetworkRequests:   atomic.LoadInt64(&rm.globalNetworkRequests),
		ActiveTrackers:    len(rm.trackers),
	}
}

// GlobalResourceMetrics contains global resource usage metrics
type GlobalResourceMetrics struct {
	MemoryUsageMB   int64 `json:"memory_usage_mb"`
	CPUUsageMs      int64 `json:"cpu_usage_ms"`
	NetworkRequests int64 `json:"network_requests"`
	ActiveTrackers  int   `json:"active_trackers"`
}

// getCPUTime returns the current CPU time (simplified implementation)
func getCPUTime() time.Duration {
	// This is a simplified version. In a real implementation,
	// you would use platform-specific code to get actual CPU time
	return time.Since(time.Now())
}

// MemoryPool provides memory management for plugins
type MemoryPool struct {
	mu        sync.RWMutex
	pools     map[int]*sync.Pool
	allocated map[uintptr]int
	maxSize   int64
	used      int64
}

// NewMemoryPool creates a new memory pool
func NewMemoryPool(maxSize int64) *MemoryPool {
	return &MemoryPool{
		pools:     make(map[int]*sync.Pool),
		allocated: make(map[uintptr]int),
		maxSize:   maxSize,
	}
}

// Allocate allocates memory from the pool
func (mp *MemoryPool) Allocate(size int) ([]byte, error) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.used+int64(size) > mp.maxSize {
		return nil, fmt.Errorf("memory pool exhausted: %d + %d > %d", mp.used, size, mp.maxSize)
	}

	// Round up to nearest power of 2
	poolSize := 1
	for poolSize < size {
		poolSize *= 2
	}

	pool, exists := mp.pools[poolSize]
	if !exists {
		pool = &sync.Pool{
			New: func() interface{} {
				return make([]byte, poolSize)
			},
		}
		mp.pools[poolSize] = pool
	}

	buffer := pool.Get().([]byte)
	ptr := uintptr(unsafe.Pointer(&buffer[0]))
	mp.allocated[ptr] = poolSize
	mp.used += int64(poolSize)

	return buffer[:size], nil
}

// Free returns memory to the pool
func (mp *MemoryPool) Free(buffer []byte) {
	if len(buffer) == 0 {
		return
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	ptr := uintptr(unsafe.Pointer(&buffer[0]))
	size, exists := mp.allocated[ptr]
	if !exists {
		return
	}

	delete(mp.allocated, ptr)
	mp.used -= int64(size)

	if pool, exists := mp.pools[size]; exists {
		// Reset buffer before returning to pool
		for i := range buffer {
			buffer[i] = 0
		}
		pool.Put(buffer)
	}
}

// GetStats returns memory pool statistics
func (mp *MemoryPool) GetStats() MemoryPoolStats {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	return MemoryPoolStats{
		MaxSize:     mp.maxSize,
		Used:        mp.used,
		Available:   mp.maxSize - mp.used,
		Allocations: len(mp.allocated),
		PoolCount:   len(mp.pools),
	}
}

// MemoryPoolStats contains memory pool statistics
type MemoryPoolStats struct {
	MaxSize     int64 `json:"max_size"`
	Used        int64 `json:"used"`
	Available   int64 `json:"available"`
	Allocations int   `json:"allocations"`
	PoolCount   int   `json:"pool_count"`
}

