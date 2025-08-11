/**
 * Advanced Caching Strategies for PyAirtable Frontend
 * Provides intelligent caching with invalidation patterns
 */

import { CacheEntry, CacheStrategy } from '../types/api';

// ============================================================================
// Cache Configuration
// ============================================================================

export interface CacheConfig {
  defaultTTL: number;
  maxSize: number;
  cleanupInterval: number;
  enablePersistence: boolean;
  persistenceKey: string;
}

export interface CacheOptions {
  ttl?: number;
  staleWhileRevalidate?: boolean;
  skipCache?: boolean;
  cacheKey?: string;
  tags?: string[];
}

// ============================================================================
// Advanced Cache Manager
// ============================================================================

export class AdvancedCacheManager {
  private cache: Map<string, CacheEntry<unknown>> = new Map();
  private tags: Map<string, Set<string>> = new Map(); // tag -> cache keys
  private keyTags: Map<string, Set<string>> = new Map(); // cache key -> tags
  private cleanupTimer: NodeJS.Timeout | null = null;

  constructor(private config: CacheConfig) {
    this.startCleanup();
    
    if (config.enablePersistence) {
      this.loadFromPersistence();
      window.addEventListener('beforeunload', () => this.saveToPersistence());
    }
  }

  set<T>(key: string, data: T, options: CacheOptions = {}): void {
    const ttl = options.ttl || this.config.defaultTTL;
    const tags = options.tags || [];

    const entry: CacheEntry<T> = {
      data,
      timestamp: Date.now(),
      expiration: Date.now() + ttl,
    };

    this.cache.set(key, entry as CacheEntry<unknown>);

    // Handle tags
    if (tags.length > 0) {
      this.keyTags.set(key, new Set(tags));
      tags.forEach(tag => {
        if (!this.tags.has(tag)) {
          this.tags.set(tag, new Set());
        }
        this.tags.get(tag)!.add(key);
      });
    }

    // Ensure cache doesn't grow too large
    this.enforceMaxSize();
  }

  get<T>(key: string, options: CacheOptions = {}): T | null {
    if (options.skipCache) return null;

    const entry = this.cache.get(key) as CacheEntry<T> | undefined;
    
    if (!entry) return null;
    
    const now = Date.now();
    
    // Handle stale-while-revalidate
    if (options.staleWhileRevalidate && now > entry.expiration) {
      // Return stale data but trigger background refresh
      this.triggerBackgroundRefresh(key, options);
      return entry.data;
    }
    
    // Standard expiration check
    if (now > entry.expiration) {
      this.delete(key);
      return null;
    }

    return entry.data;
  }

  delete(key: string): boolean {
    const deleted = this.cache.delete(key);
    
    // Remove from tags
    const tags = this.keyTags.get(key);
    if (tags) {
      tags.forEach(tag => {
        const tagKeys = this.tags.get(tag);
        if (tagKeys) {
          tagKeys.delete(key);
          if (tagKeys.size === 0) {
            this.tags.delete(tag);
          }
        }
      });
      this.keyTags.delete(key);
    }

    return deleted;
  }

  invalidateByTag(tag: string): number {
    const keys = this.tags.get(tag);
    if (!keys) return 0;

    let count = 0;
    keys.forEach(key => {
      if (this.delete(key)) count++;
    });

    return count;
  }

  invalidateByPattern(pattern: string): number {
    const regex = new RegExp(pattern);
    let count = 0;

    for (const key of this.cache.keys()) {
      if (regex.test(key)) {
        if (this.delete(key)) count++;
      }
    }

    return count;
  }

  clear(): void {
    this.cache.clear();
    this.tags.clear();
    this.keyTags.clear();
  }

  getStats(): {
    size: number;
    maxSize: number;
    hitRate: number;
    tags: number;
  } {
    return {
      size: this.cache.size,
      maxSize: this.config.maxSize,
      hitRate: 0, // Would need to track hits/misses
      tags: this.tags.size,
    };
  }

  private enforceMaxSize(): void {
    while (this.cache.size > this.config.maxSize) {
      // Remove oldest entries
      const oldestKey = this.cache.keys().next().value;
      if (oldestKey) {
        this.delete(oldestKey);
      }
    }
  }

  private startCleanup(): void {
    this.cleanupTimer = setInterval(() => {
      this.cleanup();
    }, this.config.cleanupInterval);
  }

  private cleanup(): void {
    const now = Date.now();
    const expiredKeys: string[] = [];

    for (const [key, entry] of this.cache.entries()) {
      if (now > entry.expiration) {
        expiredKeys.push(key);
      }
    }

    expiredKeys.forEach(key => this.delete(key));
  }

  private triggerBackgroundRefresh(key: string, options: CacheOptions): void {
    // This would trigger a background refresh of the data
    // Implementation depends on the specific use case
    console.log(`Triggering background refresh for key: ${key}`);
  }

  private loadFromPersistence(): void {
    try {
      const stored = localStorage.getItem(this.config.persistenceKey);
      if (stored) {
        const data = JSON.parse(stored);
        this.cache = new Map(data.cache);
        this.tags = new Map(data.tags.map(([k, v]: [string, string[]]) => [k, new Set(v)]));
        this.keyTags = new Map(data.keyTags.map(([k, v]: [string, string[]]) => [k, new Set(v)]));
      }
    } catch (error) {
      console.warn('Failed to load cache from persistence:', error);
    }
  }

  private saveToPersistence(): void {
    try {
      const data = {
        cache: Array.from(this.cache.entries()),
        tags: Array.from(this.tags.entries()).map(([k, v]) => [k, Array.from(v)]),
        keyTags: Array.from(this.keyTags.entries()).map(([k, v]) => [k, Array.from(v)]),
      };
      localStorage.setItem(this.config.persistenceKey, JSON.stringify(data));
    } catch (error) {
      console.warn('Failed to save cache to persistence:', error);
    }
  }

  destroy(): void {
    if (this.cleanupTimer) {
      clearInterval(this.cleanupTimer);
    }
    this.clear();
  }
}

// ============================================================================
// Cache Strategies
// ============================================================================

export class CacheStrategies {
  static readonly USER_PROFILE: CacheStrategy = {
    ttl: 15 * 60 * 1000, // 15 minutes
    staleWhileRevalidate: true,
    maxAge: 60 * 60 * 1000, // 1 hour
    cacheKey: (userId: string) => `user:profile:${userId}`,
  };

  static readonly WORKSPACE_LIST: CacheStrategy = {
    ttl: 5 * 60 * 1000, // 5 minutes
    staleWhileRevalidate: true,
    maxAge: 30 * 60 * 1000, // 30 minutes
    cacheKey: (userId: string) => `user:${userId}:workspaces`,
  };

  static readonly BASE_LIST: CacheStrategy = {
    ttl: 5 * 60 * 1000, // 5 minutes
    staleWhileRevalidate: true,
    maxAge: 30 * 60 * 1000, // 30 minutes
    cacheKey: (workspaceId: string) => `workspace:${workspaceId}:bases`,
  };

  static readonly TABLE_SCHEMA: CacheStrategy = {
    ttl: 30 * 60 * 1000, // 30 minutes
    staleWhileRevalidate: true,
    maxAge: 2 * 60 * 60 * 1000, // 2 hours
    cacheKey: (tableId: string) => `table:${tableId}:schema`,
  };

  static readonly RECORD_LIST: CacheStrategy = {
    ttl: 2 * 60 * 1000, // 2 minutes
    staleWhileRevalidate: true,
    maxAge: 10 * 60 * 1000, // 10 minutes
    cacheKey: (params: any) => {
      const { table_id, view_id, ...rest } = params;
      const sortedParams = Object.keys(rest).sort().map(k => `${k}=${rest[k]}`).join('&');
      return `records:${table_id}:${view_id || 'default'}:${btoa(sortedParams)}`;
    },
  };

  static readonly SINGLE_RECORD: CacheStrategy = {
    ttl: 5 * 60 * 1000, // 5 minutes
    staleWhileRevalidate: true,
    maxAge: 30 * 60 * 1000, // 30 minutes
    cacheKey: (recordId: string) => `record:${recordId}`,
  };
}

// ============================================================================
// Cache Invalidation Rules
// ============================================================================

export class CacheInvalidationManager {
  constructor(private cacheManager: AdvancedCacheManager) {}

  // Invalidate cache based on data mutations
  onRecordCreated(tableId: string, recordId: string): void {
    // Invalidate record lists for this table
    this.cacheManager.invalidateByPattern(`records:${tableId}:`);
    
    // Invalidate related views
    this.cacheManager.invalidateByTag(`table:${tableId}`);
  }

  onRecordUpdated(tableId: string, recordId: string, changes: Record<string, any>): void {
    // Invalidate the specific record
    this.cacheManager.delete(`record:${recordId}`);
    
    // Invalidate record lists that might include this record
    this.cacheManager.invalidateByPattern(`records:${tableId}:`);
    
    // If key fields changed, invalidate more broadly
    const keyFields = ['name', 'title', 'status'];
    const hasKeyFieldChanges = Object.keys(changes).some(field => keyFields.includes(field));
    
    if (hasKeyFieldChanges) {
      this.cacheManager.invalidateByTag(`table:${tableId}`);
    }
  }

  onRecordDeleted(tableId: string, recordId: string): void {
    // Invalidate the specific record
    this.cacheManager.delete(`record:${recordId}`);
    
    // Invalidate record lists
    this.cacheManager.invalidateByPattern(`records:${tableId}:`);
    
    // Invalidate related data
    this.cacheManager.invalidateByTag(`table:${tableId}`);
  }

  onTableSchemaChanged(tableId: string): void {
    // Invalidate table schema
    this.cacheManager.delete(`table:${tableId}:schema`);
    
    // Invalidate all records for this table since field definitions changed
    this.cacheManager.invalidateByPattern(`records:${tableId}:`);
    this.cacheManager.invalidateByTag(`table:${tableId}`);
  }

  onWorkspaceMembershipChanged(workspaceId: string, userId: string): void {
    // Invalidate workspace data for the user
    this.cacheManager.invalidateByPattern(`user:${userId}:workspaces`);
    
    // Invalidate workspace member lists
    this.cacheManager.invalidateByPattern(`workspace:${workspaceId}:members`);
  }

  onUserProfileUpdated(userId: string): void {
    // Invalidate user profile
    this.cacheManager.delete(`user:profile:${userId}`);
    
    // Invalidate any cached data that includes user info
    this.cacheManager.invalidateByTag(`user:${userId}`);
  }
}

// ============================================================================
// Query Cache
// ============================================================================

export interface QueryCacheEntry<T> extends CacheEntry<T> {
  query: string;
  variables?: Record<string, unknown>;
  dependencies: string[];
}

export class QueryCache {
  private cache: Map<string, QueryCacheEntry<unknown>> = new Map();
  private dependencies: Map<string, Set<string>> = new Map(); // dependency -> query keys

  constructor(private cacheManager: AdvancedCacheManager) {}

  set<T>(
    query: string,
    variables: Record<string, unknown> = {},
    data: T,
    dependencies: string[] = [],
    ttl: number = 5 * 60 * 1000
  ): void {
    const key = this.generateQueryKey(query, variables);
    
    const entry: QueryCacheEntry<T> = {
      data,
      timestamp: Date.now(),
      expiration: Date.now() + ttl,
      query,
      variables,
      dependencies,
    };

    this.cache.set(key, entry as QueryCacheEntry<unknown>);

    // Track dependencies
    dependencies.forEach(dep => {
      if (!this.dependencies.has(dep)) {
        this.dependencies.set(dep, new Set());
      }
      this.dependencies.get(dep)!.add(key);
    });
  }

  get<T>(query: string, variables: Record<string, unknown> = {}): T | null {
    const key = this.generateQueryKey(query, variables);
    const entry = this.cache.get(key) as QueryCacheEntry<T> | undefined;
    
    if (!entry) return null;
    
    if (Date.now() > entry.expiration) {
      this.delete(key);
      return null;
    }

    return entry.data;
  }

  invalidateByDependency(dependency: string): number {
    const queryKeys = this.dependencies.get(dependency);
    if (!queryKeys) return 0;

    let count = 0;
    queryKeys.forEach(key => {
      if (this.delete(key)) count++;
    });

    return count;
  }

  private delete(key: string): boolean {
    const entry = this.cache.get(key);
    if (!entry) return false;

    this.cache.delete(key);

    // Remove from dependencies
    entry.dependencies.forEach(dep => {
      const depKeys = this.dependencies.get(dep);
      if (depKeys) {
        depKeys.delete(key);
        if (depKeys.size === 0) {
          this.dependencies.delete(dep);
        }
      }
    });

    return true;
  }

  private generateQueryKey(query: string, variables: Record<string, unknown>): string {
    const variablesStr = Object.keys(variables)
      .sort()
      .map(key => `${key}:${JSON.stringify(variables[key])}`)
      .join('|');
    
    return `query:${btoa(query)}:${btoa(variablesStr)}`;
  }
}

// ============================================================================
// Cache Provider Hook (for React)
// ============================================================================

export interface CacheContextValue {
  cacheManager: AdvancedCacheManager;
  invalidationManager: CacheInvalidationManager;
  queryCache: QueryCache;
}

// This would be implemented in the React application
export function useCacheContext(): CacheContextValue {
  throw new Error('useCacheContext must be implemented in the React application');
}

// ============================================================================
// Default Cache Configuration
// ============================================================================

export const defaultCacheConfig: CacheConfig = {
  defaultTTL: 5 * 60 * 1000, // 5 minutes
  maxSize: 1000,
  cleanupInterval: 5 * 60 * 1000, // 5 minutes
  enablePersistence: true,
  persistenceKey: 'pyairtable_cache',
};

export function createCacheManager(config: Partial<CacheConfig> = {}): AdvancedCacheManager {
  return new AdvancedCacheManager({ ...defaultCacheConfig, ...config });
}