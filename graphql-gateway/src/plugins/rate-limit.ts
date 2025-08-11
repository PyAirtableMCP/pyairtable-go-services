import { ApolloServerPlugin } from '@apollo/server';
import { GraphQLError } from 'graphql';
import { Context, RateLimitOptions } from '../types';
import { RedisClient } from '../utils/redis';
import { logger, graphqlLogger } from '../utils/logger';
import { config } from '../config/config';

interface RateLimitRule {
  operation: string;
  maxRequests: number;
  windowMs: number;
  message?: string;
  skip?: (context: Context, args: any) => boolean;
}

interface RateLimitData {
  count: number;
  resetTime: number;
  firstRequest: number;
}

export function createRateLimitPlugin(redis: RedisClient): ApolloServerPlugin<Context> {
  const rateLimiter = new GraphQLRateLimiter(redis);

  return {
    requestDidStart() {
      return {
        async didResolveOperation(requestContext) {
          const { request, contextValue } = requestContext;
          
          if (!request.query) return;

          // Skip rate limiting for introspection queries
          if (request.query.includes('__schema') || request.query.includes('__type')) {
            return;
          }

          const operationType = this.getOperationType(request.query);
          const operationName = request.operationName || 'anonymous';
          
          // Apply rate limiting
          const rateLimitResult = await rateLimiter.checkRateLimit(
            contextValue,
            operationType,
            operationName,
            request.variables || {}
          );

          if (!rateLimitResult.allowed) {
            graphqlLogger.logRateLimit(
              contextValue.user?.id || 'anonymous',
              operationName,
              rateLimitResult.limit
            );

            throw new GraphQLError(
              rateLimitResult.message || 'Rate limit exceeded',
              {
                extensions: {
                  code: 'RATE_LIMITED',
                  retryAfter: rateLimitResult.retryAfter,
                  limit: rateLimitResult.limit,
                  remaining: rateLimitResult.remaining,
                  resetTime: rateLimitResult.resetTime,
                },
              }
            );
          }

          // Add rate limit headers to response
          requestContext.response.http.headers = requestContext.response.http.headers || new Map();
          requestContext.response.http.headers.set('X-Rate-Limit', rateLimitResult.limit.toString());
          requestContext.response.http.headers.set('X-Rate-Limit-Remaining', rateLimitResult.remaining.toString());
          requestContext.response.http.headers.set('X-Rate-Limit-Reset', rateLimitResult.resetTime.toString());
        },
      };
    },

    getOperationType(query: string): string {
      if (query.includes('mutation')) return 'mutation';
      if (query.includes('subscription')) return 'subscription';
      return 'query';
    },
  };
}

export class GraphQLRateLimiter {
  private rules: Map<string, RateLimitRule> = new Map();
  private readonly keyPrefix = 'rl:';

  constructor(private redis: RedisClient) {
    this.setupDefaultRules();
  }

  private setupDefaultRules(): void {
    // General query rate limits
    this.addRule('query:general', {
      operation: 'query',
      maxRequests: 1000,
      windowMs: 900000, // 15 minutes
      message: 'Too many queries. Please slow down.',
    });

    // Mutation rate limits (more restrictive)
    this.addRule('mutation:general', {
      operation: 'mutation',
      maxRequests: 100,
      windowMs: 900000, // 15 minutes
      message: 'Too many mutations. Please slow down.',
    });

    // Subscription rate limits
    this.addRule('subscription:general', {
      operation: 'subscription',
      maxRequests: 50,
      windowMs: 900000, // 15 minutes
      message: 'Too many subscriptions. Please slow down.',
    });

    // Specific operation limits
    this.addRule('mutation:createAirtableRecord', {
      operation: 'createAirtableRecord',
      maxRequests: 100,
      windowMs: 3600000, // 1 hour
      message: 'Too many record creations. Please wait before creating more records.',
    });

    this.addRule('mutation:uploadFile', {
      operation: 'uploadFile',
      maxRequests: 50,
      windowMs: 3600000, // 1 hour
      message: 'Too many file uploads. Please wait before uploading more files.',
    });

    this.addRule('query:airtableRecords', {
      operation: 'airtableRecords',
      maxRequests: 500,
      windowMs: 900000, // 15 minutes
      message: 'Too many Airtable queries. Please implement pagination or reduce query frequency.',
    });

    this.addRule('mutation:generateContent', {
      operation: 'generateContent',
      maxRequests: 20,
      windowMs: 3600000, // 1 hour
      message: 'AI content generation limit exceeded. Please upgrade your plan for higher limits.',
    });

    this.addRule('mutation:analyzeData', {
      operation: 'analyzeData',
      maxRequests: 10,
      windowMs: 3600000, // 1 hour
      message: 'AI data analysis limit exceeded. Please upgrade your plan for higher limits.',
    });
  }

  addRule(key: string, rule: RateLimitRule): void {
    this.rules.set(key, rule);
  }

  removeRule(key: string): void {
    this.rules.delete(key);
  }

  async checkRateLimit(
    context: Context,
    operationType: string,
    operationName: string,
    variables: any
  ): Promise<{
    allowed: boolean;
    limit: number;
    remaining: number;
    resetTime: number;
    retryAfter?: number;
    message?: string;
  }> {
    const userId = context.user?.id || 'anonymous';
    const userRole = context.user?.role || 'anonymous';
    
    // Get applicable rate limit rules
    const applicableRules = this.getApplicableRules(operationType, operationName, userRole);
    
    for (const rule of applicableRules) {
      // Skip if rule has a skip condition that matches
      if (rule.skip && rule.skip(context, variables)) {
        continue;
      }

      const key = this.generateKey(userId, rule.operation);
      const result = await this.checkSingleRule(key, rule);
      
      if (!result.allowed) {
        return result;
      }
    }

    // If we get here, all rules passed
    const generalRule = this.rules.get(`${operationType}:general`);
    if (generalRule) {
      const key = this.generateKey(userId, operationType);
      return await this.checkSingleRule(key, generalRule);
    }

    // Default: allow request
    return {
      allowed: true,
      limit: 1000,
      remaining: 999,
      resetTime: Date.now() + 900000,
    };
  }

  private getApplicableRules(
    operationType: string,
    operationName: string,
    userRole: string
  ): RateLimitRule[] {
    const rules: RateLimitRule[] = [];
    
    // Add specific operation rule if it exists
    const specificRule = this.rules.get(`${operationType}:${operationName}`);
    if (specificRule) {
      rules.push(specificRule);
    }
    
    // Add general operation type rule
    const generalRule = this.rules.get(`${operationType}:general`);
    if (generalRule) {
      rules.push(generalRule);
    }

    // Adjust limits based on user role
    return rules.map(rule => ({
      ...rule,
      maxRequests: this.adjustLimitForRole(rule.maxRequests, userRole),
    }));
  }

  private adjustLimitForRole(baseLimit: number, userRole: string): number {
    const multipliers = {
      admin: 10,
      premium: 5,
      standard: 2,
      free: 1,
      anonymous: 0.1,
    };

    const multiplier = multipliers[userRole as keyof typeof multipliers] || 1;
    return Math.floor(baseLimit * multiplier);
  }

  private async checkSingleRule(
    key: string,
    rule: RateLimitRule
  ): Promise<{
    allowed: boolean;
    limit: number;
    remaining: number;
    resetTime: number;
    retryAfter?: number;
    message?: string;
  }> {
    try {
      const now = Date.now();
      const windowStart = now - rule.windowMs;
      
      // Get current rate limit data
      const data = await this.getRateLimitData(key);
      
      // Reset if window has passed
      if (data && data.firstRequest < windowStart) {
        await this.resetRateLimit(key);
        data.count = 0;
        data.firstRequest = now;
      }

      const currentCount = (data?.count || 0) + 1;
      const resetTime = (data?.firstRequest || now) + rule.windowMs;
      
      if (currentCount > rule.maxRequests) {
        return {
          allowed: false,
          limit: rule.maxRequests,
          remaining: 0,
          resetTime,
          retryAfter: Math.ceil((resetTime - now) / 1000),
          message: rule.message,
        };
      }

      // Update counter
      await this.incrementCounter(key, rule.windowMs, data?.firstRequest || now);
      
      return {
        allowed: true,
        limit: rule.maxRequests,
        remaining: rule.maxRequests - currentCount,
        resetTime,
      };
    } catch (error) {
      logger.error('Rate limit check error', { error, key });
      // On error, allow the request to proceed
      return {
        allowed: true,
        limit: rule.maxRequests,
        remaining: rule.maxRequests,
        resetTime: Date.now() + rule.windowMs,
      };
    }
  }

  private async getRateLimitData(key: string): Promise<RateLimitData | null> {
    const data = await this.redis.get(key);
    return data ? JSON.parse(data) : null;
  }

  private async incrementCounter(key: string, windowMs: number, firstRequest: number): Promise<void> {
    const data = await this.getRateLimitData(key);
    const newData: RateLimitData = {
      count: (data?.count || 0) + 1,
      resetTime: firstRequest + windowMs,
      firstRequest: data?.firstRequest || Date.now(),
    };

    await this.redis.set(key, JSON.stringify(newData), Math.ceil(windowMs / 1000));
  }

  private async resetRateLimit(key: string): Promise<void> {
    await this.redis.del(key);
  }

  private generateKey(userId: string, operation: string): string {
    return `${this.keyPrefix}${userId}:${operation}`;
  }

  // Get rate limit status for a user
  async getRateLimitStatus(userId: string): Promise<Record<string, any>> {
    const status: Record<string, any> = {};
    
    for (const [ruleKey, rule] of this.rules.entries()) {
      const key = this.generateKey(userId, rule.operation);
      const data = await this.getRateLimitData(key);
      
      if (data) {
        status[ruleKey] = {
          operation: rule.operation,
          current: data.count,
          limit: rule.maxRequests,
          remaining: Math.max(0, rule.maxRequests - data.count),
          resetTime: data.resetTime,
          windowMs: rule.windowMs,
        };
      }
    }
    
    return status;
  }

  // Clear rate limits for a user (admin function)
  async clearUserRateLimits(userId: string): Promise<void> {
    const keys = await this.redis.keys(`${this.keyPrefix}${userId}:*`);
    for (const key of keys) {
      await this.redis.del(key);
    }
    
    logger.info('Cleared rate limits for user', { userId, keysCleared: keys.length });
  }

  // Get rate limit statistics
  async getStatistics(): Promise<{
    totalUsers: number;
    activeUsers: number;
    topUsers: Array<{ userId: string; totalRequests: number }>;
    operationStats: Record<string, { total: number; unique: number }>;
  }> {
    try {
      const keys = await this.redis.keys(`${this.keyPrefix}*`);
      const userStats: Record<string, number> = {};
      const operationStats: Record<string, { total: number; unique: number }> = {};
      
      for (const key of keys) {
        const parts = key.replace(this.keyPrefix, '').split(':');
        const userId = parts[0];
        const operation = parts[1];
        
        const data = await this.getRateLimitData(key);
        if (data) {
          userStats[userId] = (userStats[userId] || 0) + data.count;
          
          if (!operationStats[operation]) {
            operationStats[operation] = { total: 0, unique: 0 };
          }
          operationStats[operation].total += data.count;
          operationStats[operation].unique += 1;
        }
      }
      
      const topUsers = Object.entries(userStats)
        .map(([userId, totalRequests]) => ({ userId, totalRequests }))
        .sort((a, b) => b.totalRequests - a.totalRequests)
        .slice(0, 10);
      
      return {
        totalUsers: Object.keys(userStats).length,
        activeUsers: Object.keys(userStats).filter(userId => userStats[userId] > 0).length,
        topUsers,
        operationStats,
      };
    } catch (error) {
      logger.error('Error getting rate limit statistics', { error });
      return {
        totalUsers: 0,
        activeUsers: 0,
        topUsers: [],
        operationStats: {},
      };
    }
  }
}