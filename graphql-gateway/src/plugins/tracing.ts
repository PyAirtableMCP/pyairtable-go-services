import { ApolloServerPlugin } from '@apollo/server';
import { GraphQLError } from 'graphql';
import { Context, TracingInfo } from '../types';
import { logger, graphqlLogger, performanceLogger } from '../utils/logger';
import { config } from '../config/config';

interface SpanData {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  operationName: string;
  startTime: number;
  endTime?: number;
  duration?: number;
  tags: Record<string, any>;
  logs: Array<{ timestamp: number; fields: Record<string, any> }>;
  status: 'success' | 'error';
  error?: string;
}

interface TraceData {
  traceId: string;
  spans: SpanData[];
  startTime: number;
  endTime?: number;
  duration?: number;
  operationName: string;
  userId?: string;
  requestId: string;
}

export function createTracing(): ApolloServerPlugin<Context> {
  const tracer = new GraphQLTracer();

  return {
    requestDidStart() {
      return {
        async didResolveOperation(requestContext) {
          const { request, contextValue } = requestContext;
          
          const traceId = generateTraceId();
          const spanId = generateSpanId();
          const operationName = request.operationName || 'GraphQLOperation';
          
          // Start root span
          const rootSpan = tracer.startSpan({
            traceId,
            spanId,
            operationName,
            tags: {
              'graphql.operation.type': this.getOperationType(request.query || ''),
              'graphql.operation.name': operationName,
              'user.id': contextValue.user?.id,
              'request.id': contextValue.requestId,
            },
          });

          // Store trace info in context
          (contextValue as any).tracing = {
            traceId,
            spanId,
            tracer,
            rootSpan,
          };

          // Log operation start
          graphqlLogger.logPerformance(`${operationName} started`, 0, contextValue);
        },

        async willSendResponse(requestContext) {
          const tracing = (requestContext.contextValue as any).tracing;
          
          if (tracing) {
            const { tracer, rootSpan } = tracing;
            
            // End root span
            const duration = tracer.endSpan(rootSpan.spanId);
            
            // Log operation completion
            graphqlLogger.logPerformance(
              `${rootSpan.operationName} completed`,
              duration,
              requestContext.contextValue
            );

            // Add tracing headers
            requestContext.response.http.headers = requestContext.response.http.headers || new Map();
            requestContext.response.http.headers.set('X-Trace-ID', rootSpan.traceId);
            requestContext.response.http.headers.set('X-Span-ID', rootSpan.spanId);
            requestContext.response.http.headers.set('X-Duration', duration.toString());

            // Send trace data to monitoring system
            if (config.environment === 'production') {
              await tracer.exportTrace(rootSpan.traceId);
            }
          }
        },

        async didEncounterErrors(requestContext) {
          const tracing = (requestContext.contextValue as any).tracing;
          
          if (tracing) {
            const { tracer, rootSpan } = tracing;
            
            // Add error information to span
            tracer.addError(rootSpan.spanId, requestContext.errors[0]);
            
            // Log error with trace information
            graphqlLogger.logError(
              requestContext.errors[0],
              requestContext.contextValue,
              requestContext.request.query
            );
          }
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

export class GraphQLTracer {
  private spans: Map<string, SpanData> = new Map();
  private traces: Map<string, TraceData> = new Map();

  startSpan(options: {
    traceId: string;
    spanId: string;
    parentSpanId?: string;
    operationName: string;
    tags?: Record<string, any>;
  }): SpanData {
    const span: SpanData = {
      traceId: options.traceId,
      spanId: options.spanId,
      parentSpanId: options.parentSpanId,
      operationName: options.operationName,
      startTime: Date.now(),
      tags: options.tags || {},
      logs: [],
      status: 'success',
    };

    this.spans.set(options.spanId, span);

    // Initialize trace if it doesn't exist
    if (!this.traces.has(options.traceId)) {
      this.traces.set(options.traceId, {
        traceId: options.traceId,
        spans: [],
        startTime: Date.now(),
        operationName: options.operationName,
        requestId: options.tags?.['request.id'] || '',
        userId: options.tags?.['user.id'],
      });
    }

    const trace = this.traces.get(options.traceId)!;
    trace.spans.push(span);

    logger.debug('Span started', {
      traceId: options.traceId,
      spanId: options.spanId,
      operationName: options.operationName,
    });

    return span;
  }

  endSpan(spanId: string): number {
    const span = this.spans.get(spanId);
    if (!span) {
      logger.warn('Attempted to end non-existent span', { spanId });
      return 0;
    }

    const endTime = Date.now();
    const duration = endTime - span.startTime;

    span.endTime = endTime;
    span.duration = duration;

    // Update trace end time
    const trace = this.traces.get(span.traceId);
    if (trace && (!trace.endTime || endTime > trace.endTime)) {
      trace.endTime = endTime;
      trace.duration = endTime - trace.startTime;
    }

    logger.debug('Span ended', {
      traceId: span.traceId,
      spanId: span.spanId,
      duration,
    });

    return duration;
  }

  addTag(spanId: string, key: string, value: any): void {
    const span = this.spans.get(spanId);
    if (span) {
      span.tags[key] = value;
    }
  }

  addLog(spanId: string, fields: Record<string, any>): void {
    const span = this.spans.get(spanId);
    if (span) {
      span.logs.push({
        timestamp: Date.now(),
        fields,
      });
    }
  }

  addError(spanId: string, error: Error): void {
    const span = this.spans.get(spanId);
    if (span) {
      span.status = 'error';
      span.error = error.message;
      span.tags['error'] = true;
      span.tags['error.message'] = error.message;
      span.tags['error.stack'] = error.stack;
      
      this.addLog(spanId, {
        level: 'error',
        message: error.message,
        stack: error.stack,
      });
    }
  }

  getSpan(spanId: string): SpanData | undefined {
    return this.spans.get(spanId);
  }

  getTrace(traceId: string): TraceData | undefined {
    return this.traces.get(traceId);
  }

  async exportTrace(traceId: string): Promise<void> {
    const trace = this.traces.get(traceId);
    if (!trace) return;

    try {
      // In a real implementation, this would send to Jaeger, Zipkin, etc.
      // For now, we'll log the trace data
      logger.info('Trace completed', {
        traceId: trace.traceId,
        duration: trace.duration,
        spanCount: trace.spans.length,
        operationName: trace.operationName,
        userId: trace.userId,
        requestId: trace.requestId,
      });

      // Clean up completed trace
      setTimeout(() => {
        this.cleanupTrace(traceId);
      }, 60000); // Clean up after 1 minute
    } catch (error) {
      logger.error('Failed to export trace', { error, traceId });
    }
  }

  private cleanupTrace(traceId: string): void {
    const trace = this.traces.get(traceId);
    if (trace) {
      // Remove all spans for this trace
      for (const span of trace.spans) {
        this.spans.delete(span.spanId);
      }
      
      // Remove the trace
      this.traces.delete(traceId);
      
      logger.debug('Cleaned up trace', { traceId, spanCount: trace.spans.length });
    }
  }

  // Get performance metrics
  getMetrics(): {
    activeTraces: number;
    activeSpans: number;
    avgTraceDuration: number;
    errorRate: number;
    slowestOperations: Array<{ operation: string; avgDuration: number }>;
  } {
    const traces = Array.from(this.traces.values());
    const completedTraces = traces.filter(t => t.endTime);
    
    const totalDuration = completedTraces.reduce((sum, t) => sum + (t.duration || 0), 0);
    const avgTraceDuration = completedTraces.length > 0 ? totalDuration / completedTraces.length : 0;
    
    const errorTraces = completedTraces.filter(t => 
      t.spans.some(s => s.status === 'error')
    );
    const errorRate = completedTraces.length > 0 ? 
      (errorTraces.length / completedTraces.length) * 100 : 0;
    
    // Calculate slowest operations
    const operationDurations: Record<string, number[]> = {};
    completedTraces.forEach(trace => {
      if (!operationDurations[trace.operationName]) {
        operationDurations[trace.operationName] = [];
      }
      operationDurations[trace.operationName].push(trace.duration || 0);
    });
    
    const slowestOperations = Object.entries(operationDurations)
      .map(([operation, durations]) => ({
        operation,
        avgDuration: durations.reduce((a, b) => a + b, 0) / durations.length,
      }))
      .sort((a, b) => b.avgDuration - a.avgDuration)
      .slice(0, 10);

    return {
      activeTraces: this.traces.size,
      activeSpans: this.spans.size,
      avgTraceDuration: Math.round(avgTraceDuration),
      errorRate: Math.round(errorRate * 100) / 100,
      slowestOperations,
    };
  }

  // Create child span
  createChildSpan(
    parentSpanId: string,
    operationName: string,
    tags?: Record<string, any>
  ): SpanData | null {
    const parentSpan = this.spans.get(parentSpanId);
    if (!parentSpan) return null;

    const childSpanId = generateSpanId();
    return this.startSpan({
      traceId: parentSpan.traceId,
      spanId: childSpanId,
      parentSpanId: parentSpanId,
      operationName,
      tags,
    });
  }
}

// Utility functions
function generateTraceId(): string {
  return `trace_${Date.now()}_${Math.random().toString(36).substr(2, 16)}`;
}

function generateSpanId(): string {
  return `span_${Date.now()}_${Math.random().toString(36).substr(2, 12)}`;
}

// Tracing utilities for resolvers
export const tracingUtils = {
  // Wrap resolver with tracing
  traceResolver(operationName: string) {
    return function (target: any, propertyName: string, descriptor: PropertyDescriptor) {
      const originalMethod = descriptor.value;

      descriptor.value = async function (parent: any, args: any, context: Context, info: any) {
        const tracing = (context as any).tracing;
        
        if (!tracing) {
          return originalMethod.call(this, parent, args, context, info);
        }

        const { tracer, rootSpan } = tracing;
        const childSpan = tracer.createChildSpan(
          rootSpan.spanId,
          `${target.constructor.name}.${propertyName}`,
          {
            'resolver.field': info.fieldName,
            'resolver.type': info.parentType.name,
            'resolver.args': JSON.stringify(args),
          }
        );

        if (!childSpan) {
          return originalMethod.call(this, parent, args, context, info);
        }

        try {
          const result = await originalMethod.call(this, parent, args, context, info);
          tracer.addTag(childSpan.spanId, 'resolver.success', true);
          return result;
        } catch (error) {
          tracer.addError(childSpan.spanId, error as Error);
          throw error;
        } finally {
          tracer.endSpan(childSpan.spanId);
        }
      };

      return descriptor;
    };
  },

  // Trace external API calls
  async traceExternalCall<T>(
    context: Context,
    serviceName: string,
    operation: string,
    fn: () => Promise<T>
  ): Promise<T> {
    const tracing = (context as any).tracing;
    
    if (!tracing) {
      return fn();
    }

    const { tracer, rootSpan } = tracing;
    const span = tracer.createChildSpan(
      rootSpan.spanId,
      `external.${serviceName}.${operation}`,
      {
        'external.service': serviceName,
        'external.operation': operation,
      }
    );

    if (!span) {
      return fn();
    }

    try {
      const result = await fn();
      tracer.addTag(span.spanId, 'external.success', true);
      return result;
    } catch (error) {
      tracer.addError(span.spanId, error as Error);
      throw error;
    } finally {
      tracer.endSpan(span.spanId);
    }
  },

  // Add breadcrumb to current span
  addBreadcrumb(context: Context, message: string, data?: Record<string, any>): void {
    const tracing = (context as any).tracing;
    
    if (tracing) {
      const { tracer, rootSpan } = tracing;
      tracer.addLog(rootSpan.spanId, {
        level: 'info',
        message,
        ...data,
      });
    }
  },
};