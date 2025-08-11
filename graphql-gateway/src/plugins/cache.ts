import { ApolloServerPlugin } from '@apollo/server';
import { GraphQLError } from 'graphql';
import crypto from 'crypto';
import { Context, CacheOptions } from '../types';
import { RedisClient } from '../utils/redis';
import { logger, graphqlLogger } from '../utils/logger';
import { config } from '../config/config';

interface CacheEntry {
  data: any;
  timestamp: number;
  ttl: number;
  userId?: string;
  tags: string[];
}

interface CacheStats {
  hits: number;
  misses: number;
  totalQueries: number;
  hitRate: number;
}

export function createCachePlugin(redis: RedisClient): ApolloServerPlugin<Context> {
  const cache = new GraphQLResponseCache(redis);

  return {
    requestDidStart() {
      return {
        async willSendResponse(requestContext) {
          const { request, response, contextValue } = requestContext;
          
          // Only cache successful responses
          if (response.body.kind === 'single' && !response.body.singleResult.errors) {
            const cacheKey = cache.generateCacheKey(
              request.query || '',
              request.variables || {},
              contextValue.user?.id
            );

            const ttl = cache.calculateTTL(request.query || '', contextValue);
            const tags = cache.extractCacheTags(request.query || '', contextValue);

            await cache.set(cacheKey, response.body.singleResult, ttl, tags);
            
            graphqlLogger.logCache('SET', cacheKey, false);
          }
        },

        async didResolveOperation(requestContext) {
          const { request, contextValue } = requestContext;
          
          // Skip caching for mutations and subscriptions
          if (request.query?.includes('mutation') || request.query?.includes('subscription')) {
            return;
          }

          const cacheKey = cache.generateCacheKey(
            request.query || '',
            request.variables || {},
            contextValue.user?.id
          );

          const cachedResponse = await cache.get(cacheKey);
          
          if (cachedResponse) {
            // Return cached response
            requestContext.response.body = {
              kind: 'single',
              singleResult: cachedResponse,
            };
            
            // Add cache headers
            requestContext.response.http.headers = requestContext.response.http.headers || new Map();
            requestContext.response.http.headers.set('X-Cache', 'HIT');
            requestContext.response.http.headers.set('X-Cache-Key', cacheKey);
            
            graphqlLogger.logCache('GET', cacheKey, true);
            await cache.recordHit();
            
            // Skip execution
            return;
          }

          await cache.recordMiss();
          graphqlLogger.logCache('GET', cacheKey, false);
        },
      };
    },
  };
}

export class GraphQLResponseCache {
  private readonly keyPrefix = 'gql:cache:';
  private readonly statsKey = 'gql:cache:stats';
  private readonly tagsPrefix = 'gql:tags:';

  constructor(private redis: RedisClient) {}

  generateCacheKey(query: string, variables: any, userId?: string): string {
    const queryHash = crypto
      .createHash('sha256')
      .update(query.replace(/\s+/g, ' ').trim())
      .digest('hex')
      .substring(0, 16);

    const variablesHash = crypto
      .createHash('sha256')
      .update(JSON.stringify(variables || {}))
      .digest('hex')
      .substring(0, 8);

    const userPart = userId ? `u:${userId}` : 'anonymous';
    
    return `${this.keyPrefix}${queryHash}:${variablesHash}:${userPart}`;
  }

  async get(key: string): Promise<any | null> {
    try {
      const cached = await this.redis.get(key);
      if (!cached) return null;

      const entry: CacheEntry = JSON.parse(cached);
      
      // Check if entry has expired
      if (Date.now() - entry.timestamp > entry.ttl * 1000) {
        await this.redis.del(key);
        return null;
      }

      return entry.data;
    } catch (error) {
      logger.error('Cache get error', { error, key });
      return null;
    }
  }

  async set(key: string, data: any, ttl: number, tags: string[] = []): Promise<void> {
    try {
      const entry: CacheEntry = {
        data,
        timestamp: Date.now(),
        ttl,
        tags,
      };

      await this.redis.set(key, JSON.stringify(entry), ttl);

      // Store reverse mapping for tags
      for (const tag of tags) {
        const tagKey = `${this.tagsPrefix}${tag}`;
        await this.redis.sadd(tagKey, key);
        await this.redis.expire(tagKey, ttl + 300); // Extra 5 minutes
      }
    } catch (error) {
      logger.error('Cache set error', { error, key });
    }
  }

  async invalidate(pattern: string): Promise<number> {
    try {
      const keys = await this.redis.keys(`${this.keyPrefix}${pattern}`);
      if (keys.length === 0) return 0;

      await Promise.all(keys.map(key => this.redis.del(key)));
      
      logger.info('Cache invalidated', { pattern, count: keys.length });
      return keys.length;
    } catch (error) {
      logger.error('Cache invalidation error', { error, pattern });
      return 0;
    }
  }

  async invalidateByTags(tags: string[]): Promise<number> {
    try {
      let totalInvalidated = 0;

      for (const tag of tags) {
        const tagKey = `${this.tagsPrefix}${tag}`;
        const keys = await this.redis.smembers(tagKey);
        
        if (keys.length > 0) {
          await Promise.all(keys.map(key => this.redis.del(key)));
          await this.redis.del(tagKey);
          totalInvalidated += keys.length;
        }
      }

      logger.info('Cache invalidated by tags', { tags, count: totalInvalidated });
      return totalInvalidated;
    } catch (error) {
      logger.error('Cache tag invalidation error', { error, tags });
      return 0;
    }
  }

  calculateTTL(query: string, context: Context): number {
    const baseTTL = config.cache.ttl;
    
    // Different TTL based on query type
    if (query.includes('user') && context.user) {
      return 300; // 5 minutes for user data
    }
    
    if (query.includes('workspace')) {
      return 600; // 10 minutes for workspace data
    }
    
    if (query.includes('airtable')) {
      return 120; // 2 minutes for Airtable data (changes frequently)
    }
    
    if (query.includes('file')) {
      return 1800; // 30 minutes for file metadata
    }
    
    if (query.includes('permission')) {
      return 900; // 15 minutes for permissions
    }
    
    if (query.includes('notification')) {
      return 60; // 1 minute for notifications
    }

    return baseTTL;
  }

  extractCacheTags(query: string, context: Context): string[] {
    const tags: string[] = [];
    
    if (context.user) {
      tags.push(`user:${context.user.id}`);
    }
    
    // Extract entity types from query
    const entityTypes = ['user', 'workspace', 'airtable', 'file', 'permission', 'notification'];
    
    for (const entityType of entityTypes) {
      if (query.includes(entityType)) {
        tags.push(`entity:${entityType}`);
      }
    }
    
    // Extract specific resource IDs from variables
    const resourceIdPatterns = [
      /workspaceId:\s*"([^"]+)"/g,
      /userId:\s*"([^"]+)"/g,
      /baseId:\s*"([^"]+)"/g,
      /fileId:\s*"([^"]+)"/g,
    ];
    
    for (const pattern of resourceIdPatterns) {
      const matches = query.matchAll(pattern);
      for (const match of matches) {
        tags.push(`resource:${match[1]}`);
      }
    }
    
    return tags;
  }

  async recordHit(): Promise<void> {
    try {
      await this.redis.hset(this.statsKey, 'hits', 
        ((await this.redis.hget(this.statsKey, 'hits')) || '0') + 1
      );
    } catch (error) {
      logger.error('Error recording cache hit', { error });
    }
  }

  async recordMiss(): Promise<void> {
    try {
      await this.redis.hset(this.statsKey, 'misses', 
        ((await this.redis.hget(this.statsKey, 'misses')) || '0') + 1
      );
    } catch (error) {
      logger.error('Error recording cache miss', { error });
    }
  }

  async getStats(): Promise<CacheStats> {
    try {
      const stats = await this.redis.hgetall(this.statsKey);
      const hits = parseInt(stats.hits || '0', 10);
      const misses = parseInt(stats.misses || '0', 10);
      const totalQueries = hits + misses;
      const hitRate = totalQueries > 0 ? (hits / totalQueries) * 100 : 0;

      return {
        hits,
        misses,
        totalQueries,
        hitRate: Math.round(hitRate * 100) / 100,
      };
    } catch (error) {
      logger.error('Error getting cache stats', { error });
      return { hits: 0, misses: 0, totalQueries: 0, hitRate: 0 };
    }
  }

  async clearStats(): Promise<void> {
    try {
      await this.redis.del(this.statsKey);
    } catch (error) {
      logger.error('Error clearing cache stats', { error });
    }
  }

  async warmupCache(queries: Array<{ query: string; variables?: any }>): Promise<void> {
    logger.info('Starting cache warmup', { queryCount: queries.length });
    
    for (const { query, variables } of queries) {
      try {
        // This would typically make actual GraphQL requests to populate cache
        // For now, we'll just log the intent
        logger.debug('Warming up cache for query', { query: query.substring(0, 100) });
      } catch (error) {
        logger.error('Cache warmup error', { error, query });
      }
    }
    
    logger.info('Cache warmup completed');
  }

  async getCacheSize(): Promise<{ keys: number; memory: string }> {
    try {
      const keys = await this.redis.keys(`${this.keyPrefix}*`);
      const memory = await this.redis.getClient().memory('usage', keys[0] || 'dummy');
      
      return {
        keys: keys.length,
        memory: `${Math.round((memory || 0) / 1024 / 1024 * 100) / 100} MB`,
      };
    } catch (error) {
      logger.error('Error getting cache size', { error });
      return { keys: 0, memory: '0 MB' };
    }
  }
}

// Cache invalidation strategies
export const cacheInvalidationStrategies = {
  // Invalidate user-related cache when user is updated
  onUserUpdate: async (cache: GraphQLResponseCache, userId: string) => {
    await cache.invalidateByTags([`user:${userId}`, 'entity:user']);
  },

  // Invalidate workspace-related cache when workspace is updated
  onWorkspaceUpdate: async (cache: GraphQLResponseCache, workspaceId: string) => {
    await cache.invalidateByTags([`resource:${workspaceId}`, 'entity:workspace']);
  },

  // Invalidate Airtable cache when records change
  onAirtableUpdate: async (cache: GraphQLResponseCache, baseId: string) => {
    await cache.invalidateByTags([`resource:${baseId}`, 'entity:airtable']);
  },

  // Invalidate file cache when file is updated
  onFileUpdate: async (cache: GraphQLResponseCache, fileId: string) => {
    await cache.invalidateByTags([`resource:${fileId}`, 'entity:file']);
  },

  // Invalidate permission cache when permissions change
  onPermissionUpdate: async (cache: GraphQLResponseCache, userId: string) => {
    await cache.invalidateByTags([`user:${userId}`, 'entity:permission']);
  },
};