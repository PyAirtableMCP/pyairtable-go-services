/**
 * Real-time Event Client for PyAirtable
 * Handles WebSocket and SSE connections to backend services
 */

import { RealtimeEvents } from '../types/api';

// ============================================================================
// Event Client Configuration
// ============================================================================

export interface RealtimeConfig {
  websocketUrl: string;
  sseUrl: string;
  reconnectAttempts: number;
  reconnectDelay: number;
  heartbeatInterval: number;
  enableSSEFallback: boolean;
  subscriptionTimeout: number;
}

export interface Subscription {
  id: string;
  channel: string;
  filters?: Record<string, unknown>;
  handler: EventHandler;
  active: boolean;
}

export type EventHandler = (event: RealtimeEvents.DomainEvent) => void;

// ============================================================================
// WebSocket Client
// ============================================================================

export class WebSocketClient {
  private ws: WebSocket | null = null;
  private subscriptions: Map<string, Subscription> = new Map();
  private reconnectAttempts = 0;
  private heartbeatTimer: NodeJS.Timeout | null = null;
  private isConnecting = false;
  private messageQueue: string[] = [];

  constructor(
    private config: RealtimeConfig,
    private onConnectionChange?: (connected: boolean) => void
  ) {}

  async connect(accessToken: string): Promise<void> {
    if (this.isConnecting || this.isConnected()) {
      return;
    }

    this.isConnecting = true;

    try {
      const wsUrl = `${this.config.websocketUrl}?token=${encodeURIComponent(accessToken)}`;
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        console.log('WebSocket connected');
        this.isConnecting = false;
        this.reconnectAttempts = 0;
        this.onConnectionChange?.(true);
        this.startHeartbeat();
        this.flushMessageQueue();
        this.resubscribeAll();
      };

      this.ws.onmessage = (event) => {
        this.handleMessage(event.data);
      };

      this.ws.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason);
        this.isConnecting = false;
        this.onConnectionChange?.(false);
        this.stopHeartbeat();
        
        if (!event.wasClean && this.shouldReconnect()) {
          this.scheduleReconnect(accessToken);
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        this.isConnecting = false;
      };

    } catch (error) {
      this.isConnecting = false;
      throw error;
    }
  }

  disconnect(): void {
    this.stopHeartbeat();
    
    if (this.ws) {
      this.ws.close(1000, 'Client disconnect');
      this.ws = null;
    }
    
    this.reconnectAttempts = this.config.reconnectAttempts; // Prevent reconnection
    this.onConnectionChange?.(false);
  }

  subscribe(channel: string, handler: EventHandler, filters?: Record<string, unknown>): string {
    const subscription: Subscription = {
      id: `sub_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      channel,
      filters,
      handler,
      active: false,
    };

    this.subscriptions.set(subscription.id, subscription);

    if (this.isConnected()) {
      this.sendSubscribe(subscription);
    }

    return subscription.id;
  }

  unsubscribe(subscriptionId: string): void {
    const subscription = this.subscriptions.get(subscriptionId);
    if (!subscription) return;

    if (this.isConnected() && subscription.active) {
      this.sendUnsubscribe(subscription);
    }

    this.subscriptions.delete(subscriptionId);
  }

  private isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private shouldReconnect(): boolean {
    return this.reconnectAttempts < this.config.reconnectAttempts;
  }

  private scheduleReconnect(accessToken: string): void {
    if (!this.shouldReconnect()) return;

    const delay = this.config.reconnectDelay * Math.pow(2, this.reconnectAttempts);
    this.reconnectAttempts++;

    console.log(`Scheduling WebSocket reconnection attempt ${this.reconnectAttempts} in ${delay}ms`);

    setTimeout(() => {
      this.connect(accessToken);
    }, delay);
  }

  private startHeartbeat(): void {
    this.heartbeatTimer = setInterval(() => {
      if (this.isConnected()) {
        this.send({ type: 'ping', timestamp: Date.now() });
      }
    }, this.config.heartbeatInterval);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private flushMessageQueue(): void {
    while (this.messageQueue.length > 0 && this.isConnected()) {
      const message = this.messageQueue.shift()!;
      this.ws!.send(message);
    }
  }

  private resubscribeAll(): void {
    this.subscriptions.forEach(subscription => {
      this.sendSubscribe(subscription);
    });
  }

  private sendSubscribe(subscription: Subscription): void {
    const message = {
      type: 'subscribe',
      subscription_id: subscription.id,
      channel: subscription.channel,
      filters: subscription.filters,
    };

    this.send(message);
  }

  private sendUnsubscribe(subscription: Subscription): void {
    const message = {
      type: 'unsubscribe',
      subscription_id: subscription.id,
    };

    this.send(message);
  }

  private send(message: unknown): void {
    const messageStr = JSON.stringify(message);

    if (this.isConnected()) {
      this.ws!.send(messageStr);
    } else {
      this.messageQueue.push(messageStr);
    }
  }

  private handleMessage(data: string): void {
    try {
      const message = JSON.parse(data);

      switch (message.type) {
        case 'pong':
          // Heartbeat response
          break;

        case 'subscription_confirmed':
          this.handleSubscriptionConfirmed(message);
          break;

        case 'subscription_error':
          this.handleSubscriptionError(message);
          break;

        case 'event':
          this.handleEvent(message);
          break;

        default:
          console.warn('Unknown WebSocket message type:', message.type);
      }
    } catch (error) {
      console.error('Error parsing WebSocket message:', error);
    }
  }

  private handleSubscriptionConfirmed(message: any): void {
    const subscription = this.subscriptions.get(message.subscription_id);
    if (subscription) {
      subscription.active = true;
      console.log(`Subscription confirmed: ${subscription.channel}`);
    }
  }

  private handleSubscriptionError(message: any): void {
    const subscription = this.subscriptions.get(message.subscription_id);
    if (subscription) {
      console.error(`Subscription error for ${subscription.channel}:`, message.error);
    }
  }

  private handleEvent(message: any): void {
    const subscription = this.subscriptions.get(message.subscription_id);
    if (subscription && subscription.active) {
      subscription.handler(message.event);
    }
  }

  getActiveSubscriptions(): Subscription[] {
    return Array.from(this.subscriptions.values()).filter(sub => sub.active);
  }

  getConnectionStatus(): {
    connected: boolean;
    reconnectAttempts: number;
    subscriptions: number;
  } {
    return {
      connected: this.isConnected(),
      reconnectAttempts: this.reconnectAttempts,
      subscriptions: this.subscriptions.size,
    };
  }
}

// ============================================================================
// Server-Sent Events Client
// ============================================================================

export class SSEClient {
  private eventSource: EventSource | null = null;
  private subscriptions: Map<string, Subscription> = new Map();
  private reconnectTimer: NodeJS.Timeout | null = null;

  constructor(
    private config: RealtimeConfig,
    private onConnectionChange?: (connected: boolean) => void
  ) {}

  async connect(accessToken: string): Promise<void> {
    if (this.eventSource) {
      this.disconnect();
    }

    const sseUrl = `${this.config.sseUrl}?token=${encodeURIComponent(accessToken)}`;
    this.eventSource = new EventSource(sseUrl);

    this.eventSource.onopen = () => {
      console.log('SSE connected');
      this.onConnectionChange?.(true);
      this.clearReconnectTimer();
    };

    this.eventSource.onmessage = (event) => {
      this.handleMessage(event.data);
    };

    this.eventSource.onerror = (error) => {
      console.error('SSE error:', error);
      this.onConnectionChange?.(false);
      this.scheduleReconnect(accessToken);
    };
  }

  disconnect(): void {
    this.clearReconnectTimer();
    
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    
    this.onConnectionChange?.(false);
  }

  subscribe(channel: string, handler: EventHandler, filters?: Record<string, unknown>): string {
    const subscription: Subscription = {
      id: `sse_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      channel,
      filters,
      handler,
      active: true, // SSE subscriptions are always active once created
    };

    this.subscriptions.set(subscription.id, subscription);

    // For SSE, we might need to make an additional HTTP request to set up the subscription
    // This depends on the backend implementation
    this.setupSSESubscription(subscription);

    return subscription.id;
  }

  unsubscribe(subscriptionId: string): void {
    const subscription = this.subscriptions.get(subscriptionId);
    if (!subscription) return;

    // Make HTTP request to remove subscription
    this.removeSSESubscription(subscription);
    this.subscriptions.delete(subscriptionId);
  }

  private async setupSSESubscription(subscription: Subscription): Promise<void> {
    try {
      const response = await fetch(`${this.config.sseUrl}/subscribe`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          subscription_id: subscription.id,
          channel: subscription.channel,
          filters: subscription.filters,
        }),
      });

      if (!response.ok) {
        console.error('Failed to setup SSE subscription:', response.statusText);
      }
    } catch (error) {
      console.error('Error setting up SSE subscription:', error);
    }
  }

  private async removeSSESubscription(subscription: Subscription): Promise<void> {
    try {
      await fetch(`${this.config.sseUrl}/unsubscribe`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          subscription_id: subscription.id,
        }),
      });
    } catch (error) {
      console.error('Error removing SSE subscription:', error);
    }
  }

  private scheduleReconnect(accessToken: string): void {
    this.clearReconnectTimer();
    
    this.reconnectTimer = setTimeout(() => {
      this.connect(accessToken);
    }, this.config.reconnectDelay);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private handleMessage(data: string): void {
    try {
      const event = JSON.parse(data);
      
      // Route event to appropriate subscribers
      this.subscriptions.forEach(subscription => {
        if (this.eventMatchesSubscription(event, subscription)) {
          subscription.handler(event);
        }
      });
    } catch (error) {
      console.error('Error parsing SSE message:', error);
    }
  }

  private eventMatchesSubscription(event: RealtimeEvents.DomainEvent, subscription: Subscription): boolean {
    // Check if event matches subscription channel
    if (!event.type.startsWith(subscription.channel)) {
      return false;
    }

    // Check filters
    if (subscription.filters) {
      for (const [key, value] of Object.entries(subscription.filters)) {
        if ((event as any)[key] !== value) {
          return false;
        }
      }
    }

    return true;
  }

  isConnected(): boolean {
    return this.eventSource?.readyState === EventSource.OPEN;
  }
}

// ============================================================================
// Real-time Event Manager
// ============================================================================

export class RealtimeEventManager {
  private wsClient: WebSocketClient;
  private sseClient: SSEClient;
  private activeClient: 'websocket' | 'sse' | null = null;
  private accessToken: string | null = null;

  constructor(private config: RealtimeConfig) {
    this.wsClient = new WebSocketClient(config, (connected) => {
      if (!connected && this.activeClient === 'websocket' && config.enableSSEFallback) {
        this.fallbackToSSE();
      }
    });

    this.sseClient = new SSEClient(config, (connected) => {
      // SSE connection status handling
    });
  }

  async connect(accessToken: string): Promise<void> {
    this.accessToken = accessToken;

    try {
      await this.wsClient.connect(accessToken);
      this.activeClient = 'websocket';
    } catch (error) {
      console.warn('WebSocket connection failed, falling back to SSE:', error);
      if (this.config.enableSSEFallback) {
        await this.fallbackToSSE();
      } else {
        throw error;
      }
    }
  }

  async disconnect(): Promise<void> {
    this.wsClient.disconnect();
    this.sseClient.disconnect();
    this.activeClient = null;
    this.accessToken = null;
  }

  subscribe(channel: string, handler: EventHandler, filters?: Record<string, unknown>): string {
    if (!this.activeClient) {
      throw new Error('Not connected to real-time service');
    }

    if (this.activeClient === 'websocket') {
      return this.wsClient.subscribe(channel, handler, filters);
    } else {
      return this.sseClient.subscribe(channel, handler, filters);
    }
  }

  unsubscribe(subscriptionId: string): void {
    this.wsClient.unsubscribe(subscriptionId);
    this.sseClient.unsubscribe(subscriptionId);
  }

  private async fallbackToSSE(): Promise<void> {
    if (!this.accessToken) return;

    try {
      await this.sseClient.connect(this.accessToken);
      this.activeClient = 'sse';
    } catch (error) {
      console.error('SSE fallback failed:', error);
      throw error;
    }
  }

  getConnectionStatus(): {
    activeClient: 'websocket' | 'sse' | null;
    websocket: ReturnType<WebSocketClient['getConnectionStatus']>;
    sse: boolean;
  } {
    return {
      activeClient: this.activeClient,
      websocket: this.wsClient.getConnectionStatus(),
      sse: this.sseClient.isConnected(),
    };
  }

  // Convenience methods for common subscriptions
  subscribeToRecordChanges(tableId: string, handler: EventHandler): string {
    return this.subscribe('record', handler, { aggregate_type: 'record', table_id: tableId });
  }

  subscribeToWorkspaceChanges(workspaceId: string, handler: EventHandler): string {
    return this.subscribe('workspace', handler, { aggregate_id: workspaceId });
  }

  subscribeToUserEvents(userId: string, handler: EventHandler): string {
    return this.subscribe('user', handler, { aggregate_id: userId });
  }
}

// ============================================================================
// Event Hooks (for React)
// ============================================================================

export interface UseRealtimeOptions {
  channel: string;
  filters?: Record<string, unknown>;
  enabled?: boolean;
}

// These would be implemented in the React application
export function useRealtime(options: UseRealtimeOptions, handler: EventHandler): {
  connected: boolean;
  subscribed: boolean;
  error: Error | null;
} {
  throw new Error('useRealtime hook must be implemented in the React application');
}

export function useRealtimeEvent<T extends RealtimeEvents.DomainEvent>(
  eventType: T['type'],
  handler: (event: T) => void,
  dependencies?: unknown[]
): void {
  throw new Error('useRealtimeEvent hook must be implemented in the React application');
}

// ============================================================================
// Default Configuration
// ============================================================================

export const defaultRealtimeConfig: RealtimeConfig = {
  websocketUrl: 'ws://localhost:8080/ws',
  sseUrl: 'http://localhost:8080/events',
  reconnectAttempts: 5,
  reconnectDelay: 1000,
  heartbeatInterval: 30000,
  enableSSEFallback: true,
  subscriptionTimeout: 10000,
};

export function createRealtimeManager(config: Partial<RealtimeConfig> = {}): RealtimeEventManager {
  return new RealtimeEventManager({ ...defaultRealtimeConfig, ...config });
}