import { RedisClient } from '../utils/redis';

export interface AuthenticatedUser {
  id: string;
  email: string;
  role: string;
  permissions: string[];
}

export interface Context {
  user: AuthenticatedUser | null;
  token: string | null;
  requestId: string;
  redis: RedisClient;
}

export interface ComplexityConfig {
  maximumComplexity: number;
  variables?: Record<string, any>;
  createError?: (max: number, actual: number) => Error;
  onComplete?: (complexity: number) => void;
}

export interface CacheOptions {
  ttl?: number;
  cacheKeyFromObject?: (obj: any) => string | null;
  scope?: string;
}

export interface RateLimitOptions {
  identityArgs?: string[];
  scalarToken?: number;
  objectToken?: (obj: any) => number;
  introspection?: boolean;
  max?: number;
  window?: string;
  message?: string;
  skip?: (obj: any) => boolean;
}

export interface SubscriptionPayload {
  [key: string]: any;
}

export interface PubSubMessage {
  channel: string;
  payload: SubscriptionPayload;
  userId?: string;
  workspaceId?: string;
}

export interface DataLoaderKey {
  id: string;
  service: string;
  method: string;
  args?: Record<string, any>;
}

export interface ServiceEndpoint {
  name: string;
  url: string;
  healthCheck: string;
  version: string;
}

export interface MetricsData {
  requestCount: number;
  errorCount: number;
  avgResponseTime: number;
  complexity: number;
  depth: number;
  cacheHits: number;
  cacheMisses: number;
}

export interface TracingInfo {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  operation: string;
  startTime: number;
  endTime?: number;
  tags: Record<string, any>;
}

export interface FieldPermission {
  field: string;
  roles: string[];
  permissions: string[];
  condition?: (context: Context, args: any, info: any) => boolean;
}

export interface SchemaDirective {
  name: string;
  locations: string[];
  args: Record<string, any>;
}

export enum SubscriptionType {
  USER_UPDATED = 'USER_UPDATED',
  WORKSPACE_UPDATED = 'WORKSPACE_UPDATED',
  AIRTABLE_RECORD_CHANGED = 'AIRTABLE_RECORD_CHANGED',
  FILE_UPLOADED = 'FILE_UPLOADED',
  NOTIFICATION_RECEIVED = 'NOTIFICATION_RECEIVED',
  PERMISSION_CHANGED = 'PERMISSION_CHANGED',
  AI_PROCESSING_COMPLETE = 'AI_PROCESSING_COMPLETE',
}

export interface SubscriptionFilter {
  userId?: string;
  workspaceId?: string;
  resourceId?: string;
  eventType?: SubscriptionType;
  conditions?: Record<string, any>;
}