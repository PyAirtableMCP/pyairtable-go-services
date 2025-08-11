import { ApolloServer } from '@apollo/server';
import { expressMiddleware } from '@apollo/server/express4';
import { ApolloServerPluginDrainHttpServer } from '@apollo/server/plugin/drainHttpServer';
import { ApolloServerPluginLandingPageLocalDefault } from '@apollo/server-plugin-landing-page-local-default';
import { ApolloGateway, IntrospectAndCompose, RemoteGraphQLDataSource } from '@apollo/gateway';
import express from 'express';
import http from 'http';
import cors from 'cors';
import helmet from 'helmet';
import compression from 'compression';
import rateLimit from 'express-rate-limit';
import { WebSocketServer } from 'ws';
import { useServer } from 'graphql-ws/lib/use/ws';
import jwt from 'jsonwebtoken';
// import { createProxyMiddleware } from 'http-proxy-middleware'; // Not needed for federation

import { logger } from './utils/logger';
import { createTracing } from './plugins/tracing';
import { createComplexityPlugin } from './plugins/complexity';
import { createDepthLimitPlugin } from './plugins/depth-limit';
import { createCachePlugin } from './plugins/cache';
import { createAuthPlugin } from './plugins/auth';
import { createRateLimitPlugin } from './plugins/rate-limit';
import { createSubscriptionHandler } from './subscriptions/handler';
import { RedisClient } from './utils/redis';
import { config } from './config/config';
import { Context, AuthenticatedUser } from './types';
import { createDataLoaders } from './dataloaders';

class AuthenticatedDataSource extends RemoteGraphQLDataSource {
  willSendRequest({ request, context }: { request: any; context: Context }) {
    // Forward authentication headers to subgraphs
    if (context.user) {
      request.http.headers.set('Authorization', `Bearer ${context.token}`);
      request.http.headers.set('X-User-ID', context.user.id);
      request.http.headers.set('X-User-Role', context.user.role);
    }
    
    // Forward request ID for tracing
    if (context.requestId) {
      request.http.headers.set('X-Request-ID', context.requestId);
    }
  }
}

async function startServer() {
  const app = express();
  const httpServer = http.createServer(app);

  // Initialize Redis
  const redis = new RedisClient(config.redis);
  await redis.connect();

  // Security middleware
  app.use(helmet({
    contentSecurityPolicy: {
      directives: {
        'default-src': ["'self'"],
        'script-src': ["'self'", "'unsafe-inline'", "'unsafe-eval'"],
        'style-src': ["'self'", "'unsafe-inline'"],
        'img-src': ["'self'", "data:", "https:"],
      },
    },
    crossOriginEmbedderPolicy: false,
  }));

  app.use(compression());
  app.use(cors({
    origin: config.cors.allowedOrigins,
    credentials: true,
  }));

  // Rate limiting
  const limiter = rateLimit({
    windowMs: 15 * 60 * 1000, // 15 minutes
    max: 1000, // limit each IP to 1000 requests per windowMs
    message: 'Too many requests from this IP, please try again later.',
    standardHeaders: true,
    legacyHeaders: false,
  });
  app.use(limiter);

  // Apollo Gateway configuration
  const gateway = new ApolloGateway({
    supergraphSdl: new IntrospectAndCompose({
      subgraphs: [
        { name: 'users', url: config.subgraphs.users },
        { name: 'workspaces', url: config.subgraphs.workspaces },
        { name: 'airtable', url: config.subgraphs.airtable },
        { name: 'files', url: config.subgraphs.files },
        { name: 'permissions', url: config.subgraphs.permissions },
        { name: 'notifications', url: config.subgraphs.notifications },
        { name: 'analytics', url: config.subgraphs.analytics },
        { name: 'ai', url: config.subgraphs.ai },
      ],
    }),
    buildService({ name, url }) {
      return new AuthenticatedDataSource({ url });
    },
  });

  const server = new ApolloServer({
    gateway,
    plugins: [
      ApolloServerPluginDrainHttpServer({ httpServer }),
      ApolloServerPluginLandingPageLocalDefault({ embed: true }),
      createTracing(),
      createComplexityPlugin(redis),
      createDepthLimitPlugin(),
      createCachePlugin(redis),
      createAuthPlugin(),
      createRateLimitPlugin(redis),
    ],
    introspection: config.environment !== 'production',
    includeStacktraceInErrorResponses: config.environment !== 'production',
  });

  await server.start();

  // GraphQL endpoint
  app.use(
    '/graphql',
    expressMiddleware(server, {
      context: async ({ req }): Promise<Context> => {
        const token = req.headers.authorization?.replace('Bearer ', '');
        let user: AuthenticatedUser | null = null;
        
        if (token) {
          try {
            const decoded = jwt.verify(token, config.jwt.secret) as any;
            user = {
              id: decoded.sub,
              email: decoded.email,
              role: decoded.role,
              permissions: decoded.permissions || [],
            };
          } catch (error) {
            logger.warn('Invalid JWT token', { error: error.message });
          }
        }

        const context: Context = {
          user,
          token,
          requestId: req.headers['x-request-id'] as string || generateRequestId(),
          redis,
        };

        // Add DataLoaders to context
        (context as any).dataloaders = createDataLoaders(context);
        (context as any).subscriptionHandler = subscriptionHandler;

        return context;
      },
    })
  );

  // WebSocket server for subscriptions
  const wsServer = new WebSocketServer({
    server: httpServer,
    path: '/graphql',
  });

  const subscriptionHandler = createSubscriptionHandler(redis);
  const serverCleanup = useServer(
    {
      schema: gateway.schema,
      context: async (ctx, msg, args) => {
        const token = ctx.connectionParams?.authorization?.replace('Bearer ', '');
        let user: AuthenticatedUser | null = null;
        
        if (token) {
          try {
            const decoded = jwt.verify(token, config.jwt.secret) as any;
            user = {
              id: decoded.sub,
              email: decoded.email,
              role: decoded.role,
              permissions: decoded.permissions || [],
            };
          } catch (error) {
            logger.warn('Invalid JWT token in subscription', { error: error.message });
          }
        }

        const context: Context = {
          user,
          token,
          requestId: generateRequestId(),
          redis,
        };

        // Add DataLoaders to context for subscriptions
        (context as any).dataloaders = createDataLoaders(context);
        (context as any).subscriptionHandler = subscriptionHandler;

        return context;
      },
      onConnect: async (ctx) => {
        logger.info('WebSocket connection established', { 
          connectionParams: ctx.connectionParams 
        });
      },
      onDisconnect: (ctx) => {
        logger.info('WebSocket connection closed');
      },
    },
    wsServer
  );

  // Health check endpoint
  app.get('/health', (req, res) => {
    res.json({ 
      status: 'healthy', 
      timestamp: new Date().toISOString(),
      uptime: process.uptime(),
      version: process.env.npm_package_version || '1.0.0',
    });
  });

  // Schema introspection endpoint (development only)
  if (config.environment !== 'production') {
    app.get('/schema', async (req, res) => {
      try {
        const schema = await gateway.schema;
        res.json({ schema: schema.toString() });
      } catch (error) {
        res.status(500).json({ error: 'Failed to fetch schema' });
      }
    });
  }

  // Graceful shutdown
  process.on('SIGTERM', async () => {
    logger.info('Received SIGTERM, shutting down gracefully');
    serverCleanup.dispose();
    await server.stop();
    await redis.disconnect();
    httpServer.close(() => {
      logger.info('Server shutdown complete');
      process.exit(0);
    });
  });

  // Start server
  const port = config.port;
  httpServer.listen(port, () => {
    logger.info(`🚀 GraphQL Gateway ready at http://localhost:${port}/graphql`);
    logger.info(`🚀 GraphQL Subscriptions ready at ws://localhost:${port}/graphql`);
    logger.info(`🔍 Health check available at http://localhost:${port}/health`);
  });
}

function generateRequestId(): string {
  return `req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

// Start the server
startServer().catch((error) => {
  logger.error('Failed to start server', { error });
  process.exit(1);
});