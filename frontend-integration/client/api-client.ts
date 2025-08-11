/**
 * Comprehensive API Client Factory for PyAirtable Backend Services
 * Provides type-safe, caching, retry logic, and offline support
 */

import { 
  APIResponse, 
  ErrorResponse, 
  APIClientConfig, 
  ServiceEndpoint,
  CacheEntry,
  CacheStrategy
} from '../types/api';

// ============================================================================
// HTTP Client Configuration
// ============================================================================

export interface RequestConfig {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  url: string;
  data?: unknown;
  params?: Record<string, string | number | boolean>;
  headers?: Record<string, string>;
  timeout?: number;
  cache?: CacheStrategy;
  skipAuth?: boolean;
  retryable?: boolean;
}

export interface RequestContext {
  requestId: string;
  timestamp: number;
  userId?: string;
  tenantId?: string;
  traceId?: string;
}

// ============================================================================
// Service Registry
// ============================================================================

export class ServiceRegistry {
  private services: Map<string, ServiceEndpoint> = new Map();
  private healthChecks: Map<string, { healthy: boolean; lastCheck: number }> = new Map();

  constructor() {
    this.initializeServices();
  }

  private initializeServices() {
    const services: ServiceEndpoint[] = [
      { name: 'auth', url: 'http://localhost:8081', port: 8081, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'user', url: 'http://localhost:8082', port: 8082, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'workspace', url: 'http://localhost:8083', port: 8083, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'base', url: 'http://localhost:8084', port: 8084, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'table', url: 'http://localhost:8085', port: 8085, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'view', url: 'http://localhost:8086', port: 8086, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'record', url: 'http://localhost:8087', port: 8087, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'field', url: 'http://localhost:8088', port: 8088, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'formula', url: 'http://localhost:8089', port: 8089, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'file', url: 'http://localhost:8092', port: 8092, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
      { name: 'webhook', url: 'http://localhost:8096', port: 8096, healthCheck: '/health', timeout: 30000, retryCount: 3, weight: 1 },
    ];

    services.forEach(service => {
      this.services.set(service.name, service);
      this.healthChecks.set(service.name, { healthy: true, lastCheck: 0 });
    });
  }

  getService(name: string): ServiceEndpoint | undefined {
    return this.services.get(name);
  }

  isServiceHealthy(name: string): boolean {
    const health = this.healthChecks.get(name);
    return health?.healthy ?? false;
  }

  async checkServiceHealth(name: string): Promise<boolean> {
    const service = this.services.get(name);
    if (!service) return false;

    try {
      const response = await fetch(`${service.url}${service.healthCheck}`, {
        method: 'GET',
        timeout: 5000,
      });
      
      const healthy = response.ok;
      this.healthChecks.set(name, { healthy, lastCheck: Date.now() });
      return healthy;
    } catch (error) {
      this.healthChecks.set(name, { healthy: false, lastCheck: Date.now() });
      return false;
    }
  }

  getAllServices(): ServiceEndpoint[] {
    return Array.from(this.services.values());
  }
}

// ============================================================================
// Cache Manager
// ============================================================================

export class CacheManager {
  private cache: Map<string, CacheEntry<unknown>> = new Map();
  private readonly maxSize: number = 1000;

  constructor() {
    // Clean up expired entries every 5 minutes
    setInterval(() => this.cleanup(), 5 * 60 * 1000);
  }

  set<T>(key: string, data: T, ttl: number = 5 * 60 * 1000): void {
    const entry: CacheEntry<T> = {
      data,
      timestamp: Date.now(),
      expiration: Date.now() + ttl,
    };

    this.cache.set(key, entry as CacheEntry<unknown>);

    // Ensure cache doesn't grow too large
    if (this.cache.size > this.maxSize) {
      const oldestKey = this.cache.keys().next().value;
      this.cache.delete(oldestKey);
    }
  }

  get<T>(key: string): T | null {
    const entry = this.cache.get(key) as CacheEntry<T> | undefined;
    
    if (!entry) return null;
    
    if (Date.now() > entry.expiration) {
      this.cache.delete(key);
      return null;
    }

    return entry.data;
  }

  invalidate(pattern: string): void {
    const regex = new RegExp(pattern);
    for (const key of this.cache.keys()) {
      if (regex.test(key)) {
        this.cache.delete(key);
      }
    }
  }

  clear(): void {
    this.cache.clear();
  }

  private cleanup(): void {
    const now = Date.now();
    for (const [key, entry] of this.cache.entries()) {
      if (now > entry.expiration) {
        this.cache.delete(key);
      }
    }
  }

  generateCacheKey(method: string, url: string, params?: Record<string, unknown>): string {
    const paramsStr = params ? JSON.stringify(params) : '';
    return `${method}:${url}:${paramsStr}`;
  }
}

// ============================================================================
// Offline Queue Manager
// ============================================================================

export interface QueuedRequest extends RequestConfig {
  id: string;
  timestamp: number;
  attempts: number;
  maxAttempts: number;
}

export class OfflineQueueManager {
  private queue: QueuedRequest[] = [];
  private isProcessing = false;
  private readonly storageKey = 'pyairtable_offline_queue';

  constructor() {
    this.loadFromStorage();
    window.addEventListener('online', () => this.processQueue());
  }

  addRequest(config: RequestConfig): string {
    const id = `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
    const queuedRequest: QueuedRequest = {
      ...config,
      id,
      timestamp: Date.now(),
      attempts: 0,
      maxAttempts: 3,
    };

    this.queue.push(queuedRequest);
    this.saveToStorage();

    if (navigator.onLine) {
      this.processQueue();
    }

    return id;
  }

  async processQueue(): Promise<void> {
    if (this.isProcessing || this.queue.length === 0) return;

    this.isProcessing = true;

    try {
      const pendingRequests = [...this.queue];
      
      for (const request of pendingRequests) {
        try {
          await this.executeRequest(request);
          this.removeFromQueue(request.id);
        } catch (error) {
          request.attempts++;
          if (request.attempts >= request.maxAttempts) {
            this.removeFromQueue(request.id);
            console.error('Request failed after max attempts:', error);
          }
        }
      }
    } finally {
      this.isProcessing = false;
      this.saveToStorage();
    }
  }

  private async executeRequest(request: QueuedRequest): Promise<void> {
    // This would integrate with the main HTTP client
    // For now, just a placeholder
    const response = await fetch(request.url, {
      method: request.method,
      headers: request.headers,
      body: request.data ? JSON.stringify(request.data) : undefined,
    });

    if (!response.ok) {
      throw new Error(`Request failed: ${response.status}`);
    }
  }

  private removeFromQueue(id: string): void {
    const index = this.queue.findIndex(req => req.id === id);
    if (index !== -1) {
      this.queue.splice(index, 1);
    }
  }

  private saveToStorage(): void {
    try {
      localStorage.setItem(this.storageKey, JSON.stringify(this.queue));
    } catch (error) {
      console.warn('Failed to save offline queue to storage:', error);
    }
  }

  private loadFromStorage(): void {
    try {
      const stored = localStorage.getItem(this.storageKey);
      if (stored) {
        this.queue = JSON.parse(stored);
      }
    } catch (error) {
      console.warn('Failed to load offline queue from storage:', error);
      this.queue = [];
    }
  }

  getQueueSize(): number {
    return this.queue.length;
  }

  clearQueue(): void {
    this.queue = [];
    this.saveToStorage();
  }
}

// ============================================================================
// HTTP Client
// ============================================================================

export class HTTPClient {
  private readonly serviceRegistry: ServiceRegistry;
  private readonly cacheManager: CacheManager;
  private readonly offlineQueue: OfflineQueueManager;
  private authToken: string | null = null;
  private refreshToken: string | null = null;

  constructor(
    private config: APIClientConfig,
    private onTokenRefresh?: (tokens: { accessToken: string; refreshToken: string }) => void,
    private onAuthError?: () => void
  ) {
    this.serviceRegistry = new ServiceRegistry();
    this.cacheManager = new CacheManager();
    this.offlineQueue = new OfflineQueueManager();
  }

  setAuthTokens(accessToken: string, refreshToken: string): void {
    this.authToken = accessToken;
    this.refreshToken = refreshToken;
  }

  clearAuthTokens(): void {
    this.authToken = null;
    this.refreshToken = null;
  }

  async request<T>(serviceName: string, config: RequestConfig): Promise<APIResponse<T>> {
    const service = this.serviceRegistry.getService(serviceName);
    if (!service) {
      throw new Error(`Service '${serviceName}' not found`);
    }

    const url = `${service.url}${config.url}`;
    const fullConfig: RequestConfig = {
      ...config,
      url,
      timeout: config.timeout || service.timeout,
    };

    // Check cache first for GET requests
    if (config.method === 'GET' && this.config.enableCaching) {
      const cacheKey = this.cacheManager.generateCacheKey(config.method, url, config.params);
      const cached = this.cacheManager.get<T>(cacheKey);
      if (cached) {
        return { data: cached, status: 200 };
      }
    }

    // If offline and request is queueable, add to queue
    if (!navigator.onLine && this.config.enableOfflineQueue && this.isQueueableRequest(config)) {
      const requestId = this.offlineQueue.addRequest(fullConfig);
      throw new Error(`Request queued for offline processing: ${requestId}`);
    }

    // Execute request with retries
    return this.executeWithRetry(fullConfig);
  }

  private async executeWithRetry<T>(config: RequestConfig): Promise<APIResponse<T>> {
    let lastError: Error;
    const maxAttempts = config.retryable !== false ? this.config.retryAttempts : 1;

    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        return await this.executeRequest<T>(config);
      } catch (error) {
        lastError = error as Error;
        
        // Don't retry on auth errors or client errors
        if (error instanceof AuthError || (error as any)?.status < 500) {
          throw error;
        }

        if (attempt < maxAttempts) {
          await this.delay(this.config.retryDelay * Math.pow(2, attempt - 1));
        }
      }
    }

    throw lastError!;
  }

  private async executeRequest<T>(config: RequestConfig): Promise<APIResponse<T>> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Request-ID': `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      ...config.headers,
    };

    // Add auth token if available and not skipped
    if (this.authToken && !config.skipAuth) {
      headers.Authorization = `Bearer ${this.authToken}`;
    }

    // Build URL with query parameters
    const url = config.params ? this.buildUrlWithParams(config.url, config.params) : config.url;

    const fetchConfig: RequestInit = {
      method: config.method,
      headers,
      body: config.data ? JSON.stringify(config.data) : undefined,
      signal: AbortSignal.timeout(config.timeout || this.config.timeout),
    };

    const response = await fetch(url, fetchConfig);

    // Handle auth errors
    if (response.status === 401 && !config.skipAuth) {
      if (this.refreshToken) {
        try {
          await this.refreshAuthToken();
          // Retry the original request with new token
          headers.Authorization = `Bearer ${this.authToken}`;
          const retryResponse = await fetch(url, { ...fetchConfig, headers });
          return this.processResponse<T>(retryResponse, config);
        } catch (refreshError) {
          this.onAuthError?.();
          throw new AuthError('Authentication failed');
        }
      } else {
        this.onAuthError?.();
        throw new AuthError('Authentication required');
      }
    }

    return this.processResponse<T>(response, config);
  }

  private async processResponse<T>(response: Response, config: RequestConfig): Promise<APIResponse<T>> {
    const responseData: APIResponse<T> = {
      status: response.status,
      timestamp: Date.now(),
    };

    try {
      if (response.headers.get('content-type')?.includes('application/json')) {
        const jsonData = await response.json();
        if (response.ok) {
          responseData.data = jsonData;
          
          // Cache successful GET requests
          if (config.method === 'GET' && this.config.enableCaching) {
            const cacheKey = this.cacheManager.generateCacheKey(config.method, config.url, config.params);
            this.cacheManager.set(cacheKey, jsonData, config.cache?.ttl || this.config.cacheTimeout);
          }
        } else {
          responseData.error = jsonData.error || jsonData.message || 'Request failed';
        }
      } else {
        if (!response.ok) {
          responseData.error = `HTTP ${response.status}: ${response.statusText}`;
        }
      }
    } catch (parseError) {
      if (!response.ok) {
        responseData.error = `HTTP ${response.status}: ${response.statusText}`;
      }
    }

    if (!response.ok) {
      throw new APIError(responseData as APIResponse<T> & { error: string });
    }

    return responseData;
  }

  private async refreshAuthToken(): Promise<void> {
    if (!this.refreshToken) {
      throw new Error('No refresh token available');
    }

    const response = await fetch('http://localhost:8081/auth/refresh', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: this.refreshToken }),
    });

    if (!response.ok) {
      throw new Error('Token refresh failed');
    }

    const tokenData = await response.json();
    this.authToken = tokenData.access_token;
    this.refreshToken = tokenData.refresh_token;

    this.onTokenRefresh?.({
      accessToken: tokenData.access_token,
      refreshToken: tokenData.refresh_token,
    });
  }

  private buildUrlWithParams(url: string, params: Record<string, string | number | boolean>): string {
    const urlObj = new URL(url);
    Object.entries(params).forEach(([key, value]) => {
      urlObj.searchParams.set(key, String(value));
    });
    return urlObj.toString();
  }

  private isQueueableRequest(config: RequestConfig): boolean {
    // Only queue non-GET requests that don't require immediate response
    return config.method !== 'GET' && !config.url.includes('/auth/');
  }

  private delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  // Cache management methods
  invalidateCache(pattern: string): void {
    this.cacheManager.invalidate(pattern);
  }

  clearCache(): void {
    this.cacheManager.clear();
  }

  // Service health methods
  async checkServiceHealth(serviceName: string): Promise<boolean> {
    return this.serviceRegistry.checkServiceHealth(serviceName);
  }

  getServiceHealth(): Record<string, boolean> {
    const services = this.serviceRegistry.getAllServices();
    const health: Record<string, boolean> = {};
    
    services.forEach(service => {
      health[service.name] = this.serviceRegistry.isServiceHealthy(service.name);
    });

    return health;
  }

  // Offline queue methods
  getOfflineQueueSize(): number {
    return this.offlineQueue.getQueueSize();
  }

  processOfflineQueue(): Promise<void> {
    return this.offlineQueue.processQueue();
  }

  clearOfflineQueue(): void {
    this.offlineQueue.clearQueue();
  }
}

// ============================================================================
// Custom Errors
// ============================================================================

export class APIError extends Error {
  constructor(public response: APIResponse<unknown> & { error: string }) {
    super(response.error);
    this.name = 'APIError';
  }
}

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'AuthError';
  }
}

export class NetworkError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'NetworkError';
  }
}

// ============================================================================
// API Client Factory
// ============================================================================

export class APIClientFactory {
  private static instance: HTTPClient | null = null;

  static create(config: APIClientConfig): HTTPClient {
    if (!this.instance) {
      this.instance = new HTTPClient(config);
    }
    return this.instance;
  }

  static getInstance(): HTTPClient {
    if (!this.instance) {
      throw new Error('API Client not initialized. Call create() first.');
    }
    return this.instance;
  }

  static createDefault(): HTTPClient {
    const defaultConfig: APIClientConfig = {
      baseURL: 'http://localhost:8080/api/v1',
      timeout: 30000,
      retryAttempts: 3,
      retryDelay: 1000,
      enableCaching: true,
      cacheTimeout: 5 * 60 * 1000, // 5 minutes
      enableOptimisticUpdates: true,
      enableOfflineQueue: true,
    };

    return this.create(defaultConfig);
  }
}