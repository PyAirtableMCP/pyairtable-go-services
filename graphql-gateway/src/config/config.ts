import { config as dotenvConfig } from 'dotenv';

dotenvConfig();

export interface Config {
  environment: string;
  port: number;
  jwt: {
    secret: string;
    expiresIn: string;
  };
  redis: {
    host: string;
    port: number;
    password?: string;
    db: number;
  };
  cors: {
    allowedOrigins: string[];
  };
  subgraphs: {
    users: string;
    workspaces: string;
    airtable: string;
    files: string;
    permissions: string;
    notifications: string;
    analytics: string;
    ai: string;
  };
  tracing: {
    jaegerEndpoint: string;
    serviceName: string;
  };
  cache: {
    ttl: number;
    maxSize: number;
  };
  rateLimit: {
    windowMs: number;
    maxRequests: number;
    maxComplexity: number;
    maxDepth: number;
  };
  logging: {
    level: string;
    format: string;
  };
}

export const config: Config = {
  environment: process.env.NODE_ENV || 'development',
  port: parseInt(process.env.PORT || '4000', 10),
  
  jwt: {
    secret: process.env.JWT_SECRET || 'your-super-secret-jwt-key',
    expiresIn: process.env.JWT_EXPIRES_IN || '24h',
  },
  
  redis: {
    host: process.env.REDIS_HOST || 'localhost',
    port: parseInt(process.env.REDIS_PORT || '6379', 10),
    password: process.env.REDIS_PASSWORD,
    db: parseInt(process.env.REDIS_DB || '0', 10),
  },
  
  cors: {
    allowedOrigins: (process.env.CORS_ORIGINS || 'http://localhost:3000,http://localhost:3001').split(','),
  },
  
  subgraphs: {
    users: process.env.USERS_SUBGRAPH_URL || 'http://localhost:8001/graphql',
    workspaces: process.env.WORKSPACES_SUBGRAPH_URL || 'http://localhost:8002/graphql',
    airtable: process.env.AIRTABLE_SUBGRAPH_URL || 'http://localhost:8003/graphql',
    files: process.env.FILES_SUBGRAPH_URL || 'http://localhost:8004/graphql',
    permissions: process.env.PERMISSIONS_SUBGRAPH_URL || 'http://localhost:8005/graphql',
    notifications: process.env.NOTIFICATIONS_SUBGRAPH_URL || 'http://localhost:8006/graphql',
    analytics: process.env.ANALYTICS_SUBGRAPH_URL || 'http://localhost:8007/graphql',
    ai: process.env.AI_SUBGRAPH_URL || 'http://localhost:8008/graphql',
  },
  
  tracing: {
    jaegerEndpoint: process.env.JAEGER_ENDPOINT || 'http://localhost:14268/api/traces',
    serviceName: process.env.SERVICE_NAME || 'graphql-gateway',
  },
  
  cache: {
    ttl: parseInt(process.env.CACHE_TTL || '300', 10), // 5 minutes
    maxSize: parseInt(process.env.CACHE_MAX_SIZE || '1000', 10),
  },
  
  rateLimit: {
    windowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS || '900000', 10), // 15 minutes
    maxRequests: parseInt(process.env.RATE_LIMIT_MAX_REQUESTS || '1000', 10),
    maxComplexity: parseInt(process.env.MAX_QUERY_COMPLEXITY || '1000', 10),
    maxDepth: parseInt(process.env.MAX_QUERY_DEPTH || '10', 10),
  },
  
  logging: {
    level: process.env.LOG_LEVEL || 'info',
    format: process.env.LOG_FORMAT || 'json',
  },
};