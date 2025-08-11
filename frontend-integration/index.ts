/**
 * PyAirtable Frontend Integration Layer
 * Complete integration solution for modern frontend with Go microservices backend
 */

// ============================================================================
// Core Exports
// ============================================================================

// Types
export * from './types/api';

// HTTP Client & Service Clients
export { 
  HTTPClient, 
  APIClientFactory,
  APIError,
  AuthError,
  NetworkError 
} from './client/api-client';

export {
  ServiceClientFactory,
  apiClients,
  ErrorHandler,
  ServiceError,
  AuthServiceClient,
  UserServiceClient,
  WorkspaceServiceClient,
  BaseServiceClient,
  TableServiceClient,
  FieldServiceClient,
  RecordServiceClient,
  ViewServiceClient,
  FileServiceClient,
  WebhookServiceClient
} from './client/service-clients';

// Authentication
export {
  authOptions,
  AuthServiceIntegration,
  useAuth,
  withAuth,
  getServerAuthSession
} from './auth/nextauth-provider';

// Caching
export {
  AdvancedCacheManager,
  CacheInvalidationManager,
  CacheStrategies,
  QueryCache,
  createCacheManager,
  defaultCacheConfig,
  useCacheContext
} from './cache/cache-strategies';

// Real-time Events
export {
  WebSocketClient,
  SSEClient,
  RealtimeEventManager,
  createRealtimeManager,
  defaultRealtimeConfig,
  useRealtime,
  useRealtimeEvent
} from './realtime/event-client';

// Optimistic Updates
export {
  OptimisticUpdateManager,
  useOptimisticUpdate,
  useOptimisticState
} from './optimistic/optimistic-updates';

// ============================================================================
// Main Integration Class
// ============================================================================

import { APIClientConfig } from './types/api';
import { APIClientFactory } from './client/api-client';
import { ServiceClientFactory } from './client/service-clients';
import { AuthServiceIntegration } from './auth/nextauth-provider';
import { createCacheManager, CacheInvalidationManager } from './cache/cache-strategies';
import { createRealtimeManager } from './realtime/event-client';
import { OptimisticUpdateManager } from './optimistic/optimistic-updates';

export interface PyAirtableIntegrationConfig extends APIClientConfig {
  // Cache configuration
  cacheConfig?: {
    defaultTTL?: number;
    maxSize?: number;
    enablePersistence?: boolean;
  };
  
  // Real-time configuration
  realtimeConfig?: {
    websocketUrl?: string;
    sseUrl?: string;
    enableSSEFallback?: boolean;
  };
  
  // Optimistic updates configuration
  optimisticConfig?: {
    maxRetries?: number;
    enabled?: boolean;
  };
  
  // Feature flags
  features?: {
    enableRealtime?: boolean;
    enableOptimisticUpdates?: boolean;
    enableOfflineSupport?: boolean;
  };
}

export class PyAirtableIntegration {
  private httpClient: ReturnType<typeof APIClientFactory.createDefault>;
  private cacheManager: ReturnType<typeof createCacheManager>;
  private invalidationManager: CacheInvalidationManager;
  private realtimeManager: ReturnType<typeof createRealtimeManager>;
  private optimisticManager: OptimisticUpdateManager | null = null;
  private authIntegration: AuthServiceIntegration;

  constructor(private config: PyAirtableIntegrationConfig) {
    // Initialize HTTP client
    this.httpClient = APIClientFactory.create(config);
    
    // Initialize cache manager
    this.cacheManager = createCacheManager(config.cacheConfig);
    this.invalidationManager = new CacheInvalidationManager(this.cacheManager);
    
    // Initialize real-time manager
    this.realtimeManager = createRealtimeManager(config.realtimeConfig);
    
    // Initialize auth integration
    this.authIntegration = new AuthServiceIntegration();
    
    this.setupEventListeners();
  }

  // ============================================================================
  // Initialization Methods
  // ============================================================================

  async initialize(session?: any): Promise<void> {
    // Initialize auth integration with session
    if (session) {
      this.authIntegration.initializeWithSession(session);
    }

    // Initialize optimistic updates if enabled
    if (this.config.features?.enableOptimisticUpdates && session) {
      this.optimisticManager = new OptimisticUpdateManager(
        this.cacheManager,
        {
          userId: session.user.id,
          tenantId: session.user.tenant_id,
          sessionId: `session_${Date.now()}`,
        }
      );
    }

    // Connect to real-time services if enabled
    if (this.config.features?.enableRealtime && session?.accessToken) {
      await this.realtimeManager.connect(session.accessToken);
    }
  }

  async destroy(): Promise<void> {
    await this.realtimeManager.disconnect();
    this.cacheManager.destroy();
    this.authIntegration.clearTokens();
  }

  // ============================================================================
  // Service Access Methods
  // ============================================================================

  get services() {
    return {
      auth: ServiceClientFactory.getAuthService(),
      user: ServiceClientFactory.getUserService(),
      workspace: ServiceClientFactory.getWorkspaceService(),
      base: ServiceClientFactory.getBaseService(),
      table: ServiceClientFactory.getTableService(),
      field: ServiceClientFactory.getFieldService(),
      record: ServiceClientFactory.getRecordService(),
      view: ServiceClientFactory.getViewService(),
      file: ServiceClientFactory.getFileService(),
      webhook: ServiceClientFactory.getWebhookService(),
    };
  }

  get cache() {
    return {
      manager: this.cacheManager,
      invalidation: this.invalidationManager,
      strategies: this.getCacheStrategies(),
    };
  }

  get realtime() {
    return {
      manager: this.realtimeManager,
      subscribe: this.realtimeManager.subscribe.bind(this.realtimeManager),
      unsubscribe: this.realtimeManager.unsubscribe.bind(this.realtimeManager),
      subscribeToRecordChanges: this.realtimeManager.subscribeToRecordChanges.bind(this.realtimeManager),
      subscribeToWorkspaceChanges: this.realtimeManager.subscribeToWorkspaceChanges.bind(this.realtimeManager),
    };
  }

  get optimistic() {
    if (!this.optimisticManager) {
      throw new Error('Optimistic updates not enabled or not initialized');
    }
    return this.optimisticManager;
  }

  get auth() {
    return this.authIntegration;
  }

  // ============================================================================
  // High-Level API Methods
  // ============================================================================

  async withOptimisticUpdate<T>(
    operation: () => Promise<T>,
    optimisticUpdateFn?: () => void,
    rollbackFn?: () => void
  ): Promise<T> {
    if (!this.optimisticManager) {
      return operation();
    }

    try {
      // Apply optimistic update
      optimisticUpdateFn?.();
      
      // Execute actual operation
      const result = await operation();
      
      return result;
    } catch (error) {
      // Rollback optimistic changes
      rollbackFn?.();
      throw error;
    }
  }

  async invalidateCache(pattern: string): Promise<void> {
    this.cacheManager.invalidateByPattern(pattern);
  }

  async clearCache(): Promise<void> {
    this.cacheManager.clear();
  }

  getServiceHealth(): Record<string, boolean> {
    return this.httpClient.getServiceHealth();
  }

  getConnectionStatus() {
    return {
      http: {
        offlineQueueSize: this.httpClient.getOfflineQueueSize(),
      },
      realtime: this.realtimeManager.getConnectionStatus(),
      cache: this.cacheManager.getStats(),
      optimistic: this.optimisticManager ? {
        pendingUpdates: this.optimisticManager.getPendingUpdates().length,
        hasUpdates: this.optimisticManager.hasOptimisticUpdates(),
      } : null,
    };
  }

  // ============================================================================
  // Private Methods
  // ============================================================================

  private setupEventListeners(): void {
    // Set up real-time event listeners for cache invalidation
    if (this.config.features?.enableRealtime) {
      // Record events
      this.realtimeManager.subscribe('record', (event) => {
        switch (event.type) {
          case 'record.created':
            this.invalidationManager.onRecordCreated(
              event.payload.table_id,
              event.payload.record.id
            );
            break;
          case 'record.updated':
            this.invalidationManager.onRecordUpdated(
              event.payload.table_id,
              event.payload.record.id,
              event.payload.changes
            );
            break;
          case 'record.deleted':
            this.invalidationManager.onRecordDeleted(
              event.payload.table_id,
              event.payload.record_id
            );
            break;
        }
      });

      // Workspace events
      this.realtimeManager.subscribe('workspace', (event) => {
        switch (event.type) {
          case 'workspace.member_added':
            this.invalidationManager.onWorkspaceMembershipChanged(
              event.payload.workspace_id,
              event.payload.member.user_id
            );
            break;
        }
      });
    }
  }

  private getCacheStrategies() {
    return {
      userProfile: this.cacheManager.get.bind(this.cacheManager),
      workspaceList: this.cacheManager.get.bind(this.cacheManager),
      recordList: this.cacheManager.get.bind(this.cacheManager),
      invalidateByTag: this.cacheManager.invalidateByTag.bind(this.cacheManager),
      invalidateByPattern: this.cacheManager.invalidateByPattern.bind(this.cacheManager),
    };
  }
}

// ============================================================================
// Factory Function
// ============================================================================

export function createPyAirtableIntegration(config: Partial<PyAirtableIntegrationConfig> = {}): PyAirtableIntegration {
  const defaultConfig: PyAirtableIntegrationConfig = {
    baseURL: 'http://localhost:8080/api/v1',
    timeout: 30000,
    retryAttempts: 3,
    retryDelay: 1000,
    enableCaching: true,
    cacheTimeout: 5 * 60 * 1000,
    enableOptimisticUpdates: true,
    enableOfflineQueue: true,
    features: {
      enableRealtime: true,
      enableOptimisticUpdates: true,
      enableOfflineSupport: true,
    },
  };

  return new PyAirtableIntegration({ ...defaultConfig, ...config });
}

// ============================================================================
// Default Instance (Singleton)
// ============================================================================

let defaultInstance: PyAirtableIntegration | null = null;

export function getPyAirtableIntegration(): PyAirtableIntegration {
  if (!defaultInstance) {
    defaultInstance = createPyAirtableIntegration();
  }
  return defaultInstance;
}

export function initializePyAirtableIntegration(
  config: Partial<PyAirtableIntegrationConfig> = {},
  session?: any
): PyAirtableIntegration {
  defaultInstance = createPyAirtableIntegration(config);
  defaultInstance.initialize(session);
  return defaultInstance;
}

// ============================================================================
// React Context Provider Interface
// ============================================================================

export interface PyAirtableProviderProps {
  children: React.ReactNode;
  config?: Partial<PyAirtableIntegrationConfig>;
  session?: any;
}

// This would be implemented in the React application
export function PyAirtableProvider({ children, config, session }: PyAirtableProviderProps): JSX.Element {
  throw new Error('PyAirtableProvider must be implemented in the React application');
}

export function usePyAirtable(): PyAirtableIntegration {
  throw new Error('usePyAirtable hook must be implemented in the React application');
}

// ============================================================================
// Version Information
// ============================================================================

export const VERSION = '1.0.0';
export const INTEGRATION_NAME = 'PyAirtable Frontend Integration';

console.log(`${INTEGRATION_NAME} v${VERSION} loaded`);

export default PyAirtableIntegration;