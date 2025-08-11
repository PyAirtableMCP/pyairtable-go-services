import { ApolloServerPlugin, GraphQLRequestListener } from '@apollo/server';
import { GraphQLError } from 'graphql';
import {
  getComplexity,
  fieldExtensionsEstimator,
  simpleEstimator,
  createComplexityLimitRule,
} from 'graphql-query-complexity';
import { Context, ComplexityConfig } from '../types';
import { RedisClient } from '../utils/redis';
import { logger, graphqlLogger } from '../utils/logger';
import { config } from '../config/config';

interface ComplexityMetrics {
  userId?: string;
  query: string;
  complexity: number;
  timestamp: number;
  operationName?: string;
}

export function createComplexityPlugin(redis: RedisClient): ApolloServerPlugin<Context> {
  return {
    requestDidStart() {
      return {
        didResolveOperation(requestContext) {
          const { request, document, schema, contextValue } = requestContext;
          
          if (!document || !schema) return;

          const complexity = getComplexity({
            estimators: [
              fieldExtensionsEstimator(),
              simpleEstimator({ defaultComplexity: 1 }),
            ],
            schema,
            query: document,
            variables: request.variables,
            introspection: true,
          });

          const maxComplexity = config.rateLimit.maxComplexity;
          
          // Log high complexity queries
          if (complexity > maxComplexity * 0.8) {
            graphqlLogger.logComplexity(
              complexity,
              maxComplexity,
              request.query || ''
            );
          }

          // Block queries that exceed maximum complexity
          if (complexity > maxComplexity) {
            throw new GraphQLError(
              `Query complexity limit of ${maxComplexity} exceeded, found ${complexity}`,
              {
                extensions: {
                  code: 'QUERY_COMPLEXITY_TOO_HIGH',
                  complexity,
                  maxComplexity,
                },
              }
            );
          }

          // Rate limiting based on complexity
          if (contextValue.user) {
            this.trackComplexityUsage(
              redis,
              contextValue.user.id,
              complexity,
              request.operationName
            );
          }

          // Store complexity in context for later use
          (requestContext as any).complexity = complexity;
        },

        willSendResponse(requestContext) {
          const complexity = (requestContext as any).complexity;
          if (complexity && requestContext.contextValue.user) {
            // Log complexity metrics
            const metrics: ComplexityMetrics = {
              userId: requestContext.contextValue.user.id,
              query: requestContext.request.query || '',
              complexity,
              timestamp: Date.now(),
              operationName: requestContext.request.operationName || undefined,
            };

            // Store metrics in Redis for analysis
            this.storeComplexityMetrics(redis, metrics);
          }
        },
      };
    },

    async trackComplexityUsage(
      redis: RedisClient,
      userId: string,
      complexity: number,
      operationName?: string
    ): Promise<void> {
      try {
        const windowMs = config.rateLimit.windowMs;
        const window = Math.floor(Date.now() / windowMs);
        const key = `complexity:${userId}:${window}`;
        
        const currentComplexity = await redis.incr(key);
        await redis.expire(key, Math.ceil(windowMs / 1000));

        const maxComplexityPerWindow = config.rateLimit.maxComplexity * 10; // Allow 10x normal complexity per window
        
        if (currentComplexity > maxComplexityPerWindow) {
          graphqlLogger.logRateLimit(userId, operationName || 'unknown', maxComplexityPerWindow);
          
          throw new GraphQLError(
            'Query complexity rate limit exceeded. Please wait before making more complex queries.',
            {
              extensions: {
                code: 'COMPLEXITY_RATE_LIMIT_EXCEEDED',
                userId,
                currentComplexity,
                maxComplexityPerWindow,
                retryAfter: Math.ceil(windowMs / 1000),
              },
            }
          );
        }
      } catch (error) {
        logger.error('Error tracking complexity usage', { error, userId, complexity });
      }
    },

    async storeComplexityMetrics(redis: RedisClient, metrics: ComplexityMetrics): Promise<void> {
      try {
        const key = `metrics:complexity:${metrics.userId}:${Date.now()}`;
        await redis.set(key, JSON.stringify(metrics), 86400); // Store for 24 hours
        
        // Also store in a sorted set for easy querying
        const sortedSetKey = `complexity_history:${metrics.userId}`;
        await redis.getClient().zadd(sortedSetKey, metrics.timestamp, key);
        
        // Keep only the last 1000 entries per user
        await redis.getClient().zremrangebyrank(sortedSetKey, 0, -1001);
      } catch (error) {
        logger.error('Error storing complexity metrics', { error, metrics });
      }
    },
  };
}

// Custom complexity estimators for specific fields
export const customComplexityEstimators = {
  // Airtable operations are more expensive
  airtableRecords: ({ args }: { args: any }) => {
    const limit = args.limit || 100;
    return Math.ceil(limit / 10); // 1 complexity per 10 records
  },

  // Search operations are expensive
  searchRecords: ({ args }: { args: any }) => {
    const complexity = 10; // Base complexity
    if (args.filters && Object.keys(args.filters).length > 0) {
      return complexity + Object.keys(args.filters).length * 2;
    }
    return complexity;
  },

  // File operations
  uploadFile: () => 15,
  downloadFile: () => 5,
  
  // AI operations are very expensive
  generateContent: () => 50,
  analyzeData: () => 30,
  processImage: () => 25,

  // Workspace operations
  workspaceMembers: ({ args }: { args: any }) => {
    const limit = args.limit || 50;
    return Math.ceil(limit / 25); // 1 complexity per 25 members
  },

  // Notification operations
  notifications: ({ args }: { args: any }) => {
    const limit = args.limit || 20;
    return Math.ceil(limit / 10); // 1 complexity per 10 notifications
  },
};

// Complexity analysis for monitoring
export class ComplexityAnalyzer {
  constructor(private redis: RedisClient) {}

  async getTopComplexQueries(limit: number = 10): Promise<ComplexityMetrics[]> {
    try {
      const keys = await this.redis.keys('metrics:complexity:*');
      const metrics: ComplexityMetrics[] = [];
      
      for (const key of keys) {
        const data = await this.redis.get(key);
        if (data) {
          metrics.push(JSON.parse(data));
        }
      }

      return metrics
        .sort((a, b) => b.complexity - a.complexity)
        .slice(0, limit);
    } catch (error) {
      logger.error('Error getting top complex queries', { error });
      return [];
    }
  }

  async getUserComplexityHistory(
    userId: string,
    limit: number = 100
  ): Promise<ComplexityMetrics[]> {
    try {
      const sortedSetKey = `complexity_history:${userId}`;
      const keys = await this.redis.getClient().zrevrange(sortedSetKey, 0, limit - 1);
      const metrics: ComplexityMetrics[] = [];
      
      for (const key of keys) {
        const data = await this.redis.get(key);
        if (data) {
          metrics.push(JSON.parse(data));
        }
      }

      return metrics;
    } catch (error) {
      logger.error('Error getting user complexity history', { error, userId });
      return [];
    }
  }

  async getComplexityStats(): Promise<{
    avgComplexity: number;
    maxComplexity: number;
    totalQueries: number;
    topUsers: Array<{ userId: string; totalComplexity: number }>;
  }> {
    try {
      const keys = await this.redis.keys('metrics:complexity:*');
      let totalComplexity = 0;
      let maxComplexity = 0;
      const userComplexity: Record<string, number> = {};

      for (const key of keys) {
        const data = await this.redis.get(key);
        if (data) {
          const metrics: ComplexityMetrics = JSON.parse(data);
          totalComplexity += metrics.complexity;
          maxComplexity = Math.max(maxComplexity, metrics.complexity);
          
          if (metrics.userId) {
            userComplexity[metrics.userId] = (userComplexity[metrics.userId] || 0) + metrics.complexity;
          }
        }
      }

      const topUsers = Object.entries(userComplexity)
        .map(([userId, totalComplexity]) => ({ userId, totalComplexity }))
        .sort((a, b) => b.totalComplexity - a.totalComplexity)
        .slice(0, 10);

      return {
        avgComplexity: keys.length > 0 ? totalComplexity / keys.length : 0,
        maxComplexity,
        totalQueries: keys.length,
        topUsers,
      };
    } catch (error) {
      logger.error('Error getting complexity stats', { error });
      return {
        avgComplexity: 0,
        maxComplexity: 0,
        totalQueries: 0,
        topUsers: [],
      };
    }
  }
}