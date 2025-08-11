import { PubSub } from 'graphql-subscriptions';
import { RedisPubSub } from 'graphql-redis-subscriptions';
import { withFilter } from 'graphql-subscriptions';
import { RedisClient } from '../utils/redis';
import { logger } from '../utils/logger';
import { Context, SubscriptionType, SubscriptionFilter, PubSubMessage } from '../types';

export function createSubscriptionHandler(redis: RedisClient) {
  return new GraphQLSubscriptionHandler(redis);
}

export class GraphQLSubscriptionHandler {
  private pubsub: PubSub;
  private subscribers: Map<string, Set<string>> = new Map();

  constructor(private redis: RedisClient) {
    // Use Redis PubSub for production, in-memory for development
    if (process.env.NODE_ENV === 'production') {
      this.pubsub = new RedisPubSub({
        publisher: redis.getClient(),
        subscriber: redis.createSubscriber(),
      });
    } else {
      this.pubsub = new PubSub();
    }

    this.setupEventHandlers();
  }

  private setupEventHandlers(): void {
    // Handle Redis connection events
    if (this.pubsub instanceof RedisPubSub) {
      this.pubsub.getPublisher().on('error', (error) => {
        logger.error('Redis PubSub publisher error', { error });
      });

      this.pubsub.getSubscriber().on('error', (error) => {
        logger.error('Redis PubSub subscriber error', { error });
      });
    }
  }

  // Publish event to subscribers
  async publish(channel: string, payload: PubSubMessage): Promise<void> {
    try {
      await this.pubsub.publish(channel, payload);
      logger.debug('Event published', { channel, payload });
    } catch (error) {
      logger.error('Failed to publish event', { error, channel, payload });
    }
  }

  // Subscribe to events with filtering
  subscribe(
    channel: string,
    filter?: (payload: PubSubMessage, variables: any, context: Context) => boolean
  ) {
    if (filter) {
      return withFilter(
        () => this.pubsub.asyncIterator(channel),
        filter
      )();
    }

    return this.pubsub.asyncIterator(channel);
  }

  // User-related subscriptions
  userUpdatedSubscription() {
    return this.subscribe(
      SubscriptionType.USER_UPDATED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Users can only subscribe to their own updates or admins can see all
        return context.user?.role === 'admin' || 
               payload.userId === context.user?.id;
      }
    );
  }

  // Workspace-related subscriptions
  workspaceUpdatedSubscription() {
    return this.subscribe(
      SubscriptionType.WORKSPACE_UPDATED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Check if user has access to the workspace
        return this.userHasWorkspaceAccess(context.user?.id, payload.workspaceId);
      }
    );
  }

  // Airtable record change subscriptions
  airtableRecordChangedSubscription() {
    return this.subscribe(
      SubscriptionType.AIRTABLE_RECORD_CHANGED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Check if user has access to the Airtable base
        return this.userHasAirtableAccess(context.user?.id, payload.workspaceId);
      }
    );
  }

  // File upload subscriptions
  fileUploadedSubscription() {
    return this.subscribe(
      SubscriptionType.FILE_UPLOADED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Check if user has access to the workspace where file was uploaded
        return this.userHasWorkspaceAccess(context.user?.id, payload.workspaceId);
      }
    );
  }

  // Notification subscriptions
  notificationReceivedSubscription() {
    return this.subscribe(
      SubscriptionType.NOTIFICATION_RECEIVED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Users can only subscribe to their own notifications
        return payload.userId === context.user?.id;
      }
    );
  }

  // Permission change subscriptions
  permissionChangedSubscription() {
    return this.subscribe(
      SubscriptionType.PERMISSION_CHANGED,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Users can subscribe to their own permission changes
        return payload.userId === context.user?.id;
      }
    );
  }

  // AI processing complete subscriptions
  aiProcessingCompleteSubscription() {
    return this.subscribe(
      SubscriptionType.AI_PROCESSING_COMPLETE,
      (payload: PubSubMessage, variables: any, context: Context) => {
        // Check if user initiated the AI processing
        return payload.userId === context.user?.id;
      }
    );
  }

  // Helper methods for access control
  private async userHasWorkspaceAccess(userId?: string, workspaceId?: string): Promise<boolean> {
    if (!userId || !workspaceId) return false;
    
    try {
      // This would typically query the database to check workspace membership
      // For now, return true as placeholder
      const key = `workspace:${workspaceId}:members`;
      return await this.redis.sismember(key, userId);
    } catch (error) {
      logger.error('Error checking workspace access', { error, userId, workspaceId });
      return false;
    }
  }

  private async userHasAirtableAccess(userId?: string, workspaceId?: string): Promise<boolean> {
    if (!userId || !workspaceId) return false;
    
    try {
      // Check if user has access to the workspace (which includes Airtable access)
      return await this.userHasWorkspaceAccess(userId, workspaceId);
    } catch (error) {
      logger.error('Error checking Airtable access', { error, userId, workspaceId });
      return false;
    }
  }

  // Subscription management
  async addSubscriber(channel: string, subscriberId: string): Promise<void> {
    if (!this.subscribers.has(channel)) {
      this.subscribers.set(channel, new Set());
    }
    
    this.subscribers.get(channel)!.add(subscriberId);
    logger.debug('Subscriber added', { channel, subscriberId });
  }

  async removeSubscriber(channel: string, subscriberId: string): Promise<void> {
    const channelSubscribers = this.subscribers.get(channel);
    if (channelSubscribers) {
      channelSubscribers.delete(subscriberId);
      
      if (channelSubscribers.size === 0) {
        this.subscribers.delete(channel);
      }
    }
    
    logger.debug('Subscriber removed', { channel, subscriberId });
  }

  // Get subscription statistics
  getSubscriptionStats(): {
    totalChannels: number;
    totalSubscribers: number;
    channelStats: Record<string, number>;
  } {
    const channelStats: Record<string, number> = {};
    let totalSubscribers = 0;

    for (const [channel, subscribers] of this.subscribers.entries()) {
      channelStats[channel] = subscribers.size;
      totalSubscribers += subscribers.size;
    }

    return {
      totalChannels: this.subscribers.size,
      totalSubscribers,
      channelStats,
    };
  }

  // Cleanup inactive subscriptions
  async cleanupInactiveSubscriptions(): Promise<void> {
    const cutoffTime = Date.now() - (30 * 60 * 1000); // 30 minutes ago
    
    for (const [channel, subscribers] of this.subscribers.entries()) {
      const activeSubscribers = new Set<string>();
      
      for (const subscriberId of subscribers) {
        // Check if subscriber is still active
        const lastSeen = await this.redis.get(`subscriber:${subscriberId}:lastSeen`);
        
        if (lastSeen && parseInt(lastSeen, 10) > cutoffTime) {
          activeSubscribers.add(subscriberId);
        }
      }
      
      if (activeSubscribers.size === 0) {
        this.subscribers.delete(channel);
      } else {
        this.subscribers.set(channel, activeSubscribers);
      }
    }
    
    logger.info('Cleaned up inactive subscriptions');
  }

  // Health check for subscription system
  async healthCheck(): Promise<{
    status: 'healthy' | 'unhealthy';
    details: {
      pubsubConnected: boolean;
      redisConnected: boolean;
      activeSubscriptions: number;
    };
  }> {
    const pubsubConnected = this.pubsub !== null;
    const redisConnected = this.redis.isHealthy();
    const activeSubscriptions = this.subscribers.size;

    const status = pubsubConnected && redisConnected ? 'healthy' : 'unhealthy';

    return {
      status,
      details: {
        pubsubConnected,
        redisConnected,
        activeSubscriptions,
      },
    };
  }
}

// Subscription resolvers for GraphQL schema
export const subscriptionResolvers = {
  Subscription: {
    userUpdated: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.userUpdatedSubscription();
      },
    },

    workspaceUpdated: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.workspaceUpdatedSubscription();
      },
    },

    airtableRecordChanged: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.airtableRecordChangedSubscription();
      },
    },

    fileUploaded: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.fileUploadedSubscription();
      },
    },

    notificationReceived: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.notificationReceivedSubscription();
      },
    },

    permissionChanged: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.permissionChangedSubscription();
      },
    },

    aiProcessingComplete: {
      subscribe: (parent: any, args: any, context: Context) => {
        const handler = (context as any).subscriptionHandler as GraphQLSubscriptionHandler;
        return handler.aiProcessingCompleteSubscription();
      },
    },
  },
};

// Event publishers for different services
export const eventPublishers = {
  publishUserUpdated: async (handler: GraphQLSubscriptionHandler, userId: string, userData: any) => {
    await handler.publish(SubscriptionType.USER_UPDATED, {
      channel: SubscriptionType.USER_UPDATED,
      payload: { user: userData },
      userId,
    });
  },

  publishWorkspaceUpdated: async (
    handler: GraphQLSubscriptionHandler,
    workspaceId: string,
    workspaceData: any
  ) => {
    await handler.publish(SubscriptionType.WORKSPACE_UPDATED, {
      channel: SubscriptionType.WORKSPACE_UPDATED,
      payload: { workspace: workspaceData },
      workspaceId,
    });
  },

  publishAirtableRecordChanged: async (
    handler: GraphQLSubscriptionHandler,
    workspaceId: string,
    recordData: any
  ) => {
    await handler.publish(SubscriptionType.AIRTABLE_RECORD_CHANGED, {
      channel: SubscriptionType.AIRTABLE_RECORD_CHANGED,
      payload: { record: recordData },
      workspaceId,
    });
  },

  publishFileUploaded: async (
    handler: GraphQLSubscriptionHandler,
    userId: string,
    workspaceId: string,
    fileData: any
  ) => {
    await handler.publish(SubscriptionType.FILE_UPLOADED, {
      channel: SubscriptionType.FILE_UPLOADED,
      payload: { file: fileData },
      userId,
      workspaceId,
    });
  },

  publishNotificationReceived: async (
    handler: GraphQLSubscriptionHandler,
    userId: string,
    notificationData: any
  ) => {
    await handler.publish(SubscriptionType.NOTIFICATION_RECEIVED, {
      channel: SubscriptionType.NOTIFICATION_RECEIVED,
      payload: { notification: notificationData },
      userId,
    });
  },

  publishPermissionChanged: async (
    handler: GraphQLSubscriptionHandler,
    userId: string,
    permissionData: any
  ) => {
    await handler.publish(SubscriptionType.PERMISSION_CHANGED, {
      channel: SubscriptionType.PERMISSION_CHANGED,
      payload: { permission: permissionData },
      userId,
    });
  },

  publishAiProcessingComplete: async (
    handler: GraphQLSubscriptionHandler,
    userId: string,
    processingResult: any
  ) => {
    await handler.publish(SubscriptionType.AI_PROCESSING_COMPLETE, {
      channel: SubscriptionType.AI_PROCESSING_COMPLETE,
      payload: { result: processingResult },
      userId,
    });
  },
};