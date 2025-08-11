import winston from 'winston';
import { config } from '../config/config';

// Custom log format
const logFormat = winston.format.combine(
  winston.format.timestamp(),
  winston.format.errors({ stack: true }),
  winston.format.json(),
  winston.format.printf(({ timestamp, level, message, ...meta }) => {
    return JSON.stringify({
      timestamp,
      level,
      message,
      service: 'graphql-gateway',
      ...meta,
    });
  })
);

// Create logger instance
export const logger = winston.createLogger({
  level: config.logging.level,
  format: logFormat,
  defaultMeta: {
    service: 'graphql-gateway',
    version: process.env.npm_package_version || '1.0.0',
  },
  transports: [
    // Console transport for development
    new winston.transports.Console({
      format: config.environment === 'development' 
        ? winston.format.combine(
            winston.format.colorize(),
            winston.format.simple()
          )
        : logFormat,
    }),
    
    // File transport for production
    ...(config.environment === 'production' ? [
      new winston.transports.File({
        filename: 'logs/error.log',
        level: 'error',
        maxsize: 10 * 1024 * 1024, // 10MB
        maxFiles: 5,
      }),
      new winston.transports.File({
        filename: 'logs/combined.log',
        maxsize: 10 * 1024 * 1024, // 10MB
        maxFiles: 10,
      }),
    ] : []),
  ],
  
  // Handle exceptions and rejections
  exceptionHandlers: [
    new winston.transports.Console(),
    ...(config.environment === 'production' ? [
      new winston.transports.File({ filename: 'logs/exceptions.log' })
    ] : []),
  ],
  
  rejectionHandlers: [
    new winston.transports.Console(),
    ...(config.environment === 'production' ? [
      new winston.transports.File({ filename: 'logs/rejections.log' })
    ] : []),
  ],
});

// GraphQL-specific logging utilities
export const graphqlLogger = {
  logQuery: (query: string, variables: any, context: any) => {
    logger.info('GraphQL Query Executed', {
      query: query.replace(/\s+/g, ' ').trim(),
      variables,
      userId: context.user?.id,
      requestId: context.requestId,
    });
  },

  logMutation: (mutation: string, variables: any, context: any) => {
    logger.info('GraphQL Mutation Executed', {
      mutation: mutation.replace(/\s+/g, ' ').trim(),
      variables,
      userId: context.user?.id,
      requestId: context.requestId,
    });
  },

  logSubscription: (subscription: string, variables: any, context: any) => {
    logger.info('GraphQL Subscription Started', {
      subscription: subscription.replace(/\s+/g, ' ').trim(),
      variables,
      userId: context.user?.id,
      requestId: context.requestId,
    });
  },

  logError: (error: Error, context: any, query?: string) => {
    logger.error('GraphQL Error', {
      error: error.message,
      stack: error.stack,
      query: query?.replace(/\s+/g, ' ').trim(),
      userId: context.user?.id,
      requestId: context.requestId,
    });
  },

  logPerformance: (operationName: string, duration: number, context: any) => {
    logger.info('GraphQL Performance', {
      operation: operationName,
      duration,
      userId: context.user?.id,
      requestId: context.requestId,
    });
  },

  logRateLimit: (userId: string, operation: string, limit: number) => {
    logger.warn('Rate Limit Exceeded', {
      userId,
      operation,
      limit,
    });
  },

  logComplexity: (complexity: number, maxComplexity: number, query: string) => {
    logger.warn('High Query Complexity', {
      complexity,
      maxComplexity,
      query: query.replace(/\s+/g, ' ').trim(),
    });
  },

  logCache: (operation: string, key: string, hit: boolean) => {
    logger.debug('Cache Operation', {
      operation,
      key,
      hit,
    });
  },

  logAuth: (operation: string, userId?: string, success: boolean = true) => {
    logger.info('Authentication Event', {
      operation,
      userId,
      success,
    });
  },

  logFederation: (subgraph: string, operation: string, duration: number, success: boolean) => {
    logger.info('Federation Operation', {
      subgraph,
      operation,
      duration,
      success,
    });
  },
};

// Performance monitoring
export const performanceLogger = {
  startTimer: (label: string) => {
    return {
      end: () => {
        const endTime = process.hrtime.bigint();
        logger.debug('Performance Timer', {
          label,
          duration: Number(endTime) / 1000000, // Convert to milliseconds
        });
      }
    };
  },

  logMemoryUsage: () => {
    const memUsage = process.memoryUsage();
    logger.debug('Memory Usage', {
      rss: Math.round(memUsage.rss / 1024 / 1024),
      heapTotal: Math.round(memUsage.heapTotal / 1024 / 1024),
      heapUsed: Math.round(memUsage.heapUsed / 1024 / 1024),
      external: Math.round(memUsage.external / 1024 / 1024),
    });
  },

  logResourceUsage: () => {
    const usage = process.cpuUsage();
    logger.debug('CPU Usage', {
      user: usage.user,
      system: usage.system,
    });
  },
};

// Export as default for backward compatibility
export default logger;