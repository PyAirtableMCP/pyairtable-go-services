import DataLoader from 'dataloader';
import { Context } from '../types';
import { logger } from '../utils/logger';
import { tracingUtils } from '../plugins/tracing';

// Generic DataLoader factory
export function createDataLoader<K, V>(
  batchLoadFn: (keys: readonly K[]) => Promise<(V | Error)[]>,
  options?: DataLoader.Options<K, V>
): DataLoader<K, V> {
  return new DataLoader(batchLoadFn, {
    cache: true,
    maxBatchSize: 100,
    cacheKeyFn: (key: K) => JSON.stringify(key),
    ...options,
  });
}

// User DataLoaders
export function createUserDataLoaders(context: Context) {
  const userLoader = createDataLoader<string, any>(
    async (userIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'user-service',
        'getUsersByIds',
        async () => {
          logger.debug('Batch loading users', { count: userIds.length });
          
          // Make batch request to user service
          const response = await fetch(`${process.env.USERS_SUBGRAPH_URL}/batch/users`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ ids: userIds }),
          });

          if (!response.ok) {
            throw new Error(`User service error: ${response.statusText}`);
          }

          const users = await response.json();
          
          // Ensure order matches input order
          return userIds.map(id => 
            users.find((user: any) => user.id === id) || 
            new Error(`User not found: ${id}`)
          );
        }
      );
    }
  );

  const userPermissionsLoader = createDataLoader<string, any[]>(
    async (userIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'permission-service',
        'getUserPermissions',
        async () => {
          logger.debug('Batch loading user permissions', { count: userIds.length });

          const response = await fetch(`${process.env.PERMISSIONS_SUBGRAPH_URL}/batch/permissions`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ userIds }),
          });

          if (!response.ok) {
            throw new Error(`Permission service error: ${response.statusText}`);
          }

          const permissions = await response.json();
          
          return userIds.map(id => 
            permissions[id] || []
          );
        }
      );
    }
  );

  return {
    userLoader,
    userPermissionsLoader,
  };
}

// Workspace DataLoaders
export function createWorkspaceDataLoaders(context: Context) {
  const workspaceLoader = createDataLoader<string, any>(
    async (workspaceIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'workspace-service',
        'getWorkspacesByIds',
        async () => {
          logger.debug('Batch loading workspaces', { count: workspaceIds.length });

          const response = await fetch(`${process.env.WORKSPACES_SUBGRAPH_URL}/batch/workspaces`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ ids: workspaceIds }),
          });

          if (!response.ok) {
            throw new Error(`Workspace service error: ${response.statusText}`);
          }

          const workspaces = await response.json();
          
          return workspaceIds.map(id => 
            workspaces.find((workspace: any) => workspace.id === id) || 
            new Error(`Workspace not found: ${id}`)
          );
        }
      );
    }
  );

  const workspaceMembersLoader = createDataLoader<string, any[]>(
    async (workspaceIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'workspace-service',
        'getWorkspaceMembers',
        async () => {
          logger.debug('Batch loading workspace members', { count: workspaceIds.length });

          const response = await fetch(`${process.env.WORKSPACES_SUBGRAPH_URL}/batch/members`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ workspaceIds }),
          });

          if (!response.ok) {
            throw new Error(`Workspace service error: ${response.statusText}`);
          }

          const members = await response.json();
          
          return workspaceIds.map(id => members[id] || []);
        }
      );
    }
  );

  const workspaceProjectsLoader = createDataLoader<string, any[]>(
    async (workspaceIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'workspace-service',
        'getWorkspaceProjects',
        async () => {
          logger.debug('Batch loading workspace projects', { count: workspaceIds.length });

          const response = await fetch(`${process.env.WORKSPACES_SUBGRAPH_URL}/batch/projects`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ workspaceIds }),
          });

          if (!response.ok) {
            throw new Error(`Workspace service error: ${response.statusText}`);
          }

          const projects = await response.json();
          
          return workspaceIds.map(id => projects[id] || []);
        }
      );
    }
  );

  return {
    workspaceLoader,
    workspaceMembersLoader,
    workspaceProjectsLoader,
  };
}

// Airtable DataLoaders
export function createAirtableDataLoaders(context: Context) {
  const airtableBaseLoader = createDataLoader<string, any>(
    async (baseIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'airtable-service',
        'getBasesByIds',
        async () => {
          logger.debug('Batch loading Airtable bases', { count: baseIds.length });

          const response = await fetch(`${process.env.AIRTABLE_SUBGRAPH_URL}/batch/bases`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ ids: baseIds }),
          });

          if (!response.ok) {
            throw new Error(`Airtable service error: ${response.statusText}`);
          }

          const bases = await response.json();
          
          return baseIds.map(id => 
            bases.find((base: any) => base.id === id) || 
            new Error(`Airtable base not found: ${id}`)
          );
        }
      );
    }
  );

  const airtableRecordsLoader = createDataLoader<{ baseId: string; tableId: string }, any[]>(
    async (queries) => {
      return tracingUtils.traceExternalCall(
        context,
        'airtable-service',
        'getRecordsBatch',
        async () => {
          logger.debug('Batch loading Airtable records', { count: queries.length });

          const response = await fetch(`${process.env.AIRTABLE_SUBGRAPH_URL}/batch/records`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ queries }),
          });

          if (!response.ok) {
            throw new Error(`Airtable service error: ${response.statusText}`);
          }

          const recordsBatch = await response.json();
          
          return queries.map((query, index) => 
            recordsBatch[index] || []
          );
        }
      );
    },
    {
      cacheKeyFn: (key) => `${key.baseId}:${key.tableId}`,
    }
  );

  const airtableSchemaLoader = createDataLoader<string, any>(
    async (baseIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'airtable-service',
        'getSchemasByIds',
        async () => {
          logger.debug('Batch loading Airtable schemas', { count: baseIds.length });

          const response = await fetch(`${process.env.AIRTABLE_SUBGRAPH_URL}/batch/schemas`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ baseIds }),
          });

          if (!response.ok) {
            throw new Error(`Airtable service error: ${response.statusText}`);
          }

          const schemas = await response.json();
          
          return baseIds.map(id => 
            schemas[id] || new Error(`Schema not found for base: ${id}`)
          );
        }
      );
    }
  );

  return {
    airtableBaseLoader,
    airtableRecordsLoader,
    airtableSchemaLoader,
  };
}

// File DataLoaders
export function createFileDataLoaders(context: Context) {
  const fileLoader = createDataLoader<string, any>(
    async (fileIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'file-service',
        'getFilesByIds',
        async () => {
          logger.debug('Batch loading files', { count: fileIds.length });

          const response = await fetch(`${process.env.FILES_SUBGRAPH_URL}/batch/files`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ ids: fileIds }),
          });

          if (!response.ok) {
            throw new Error(`File service error: ${response.statusText}`);
          }

          const files = await response.json();
          
          return fileIds.map(id => 
            files.find((file: any) => file.id === id) || 
            new Error(`File not found: ${id}`)
          );
        }
      );
    }
  );

  const fileMetadataLoader = createDataLoader<string, any>(
    async (fileIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'file-service',
        'getFileMetadata',
        async () => {
          logger.debug('Batch loading file metadata', { count: fileIds.length });

          const response = await fetch(`${process.env.FILES_SUBGRAPH_URL}/batch/metadata`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ fileIds }),
          });

          if (!response.ok) {
            throw new Error(`File service error: ${response.statusText}`);
          }

          const metadata = await response.json();
          
          return fileIds.map(id => metadata[id] || {});
        }
      );
    }
  );

  return {
    fileLoader,
    fileMetadataLoader,
  };
}

// Notification DataLoaders
export function createNotificationDataLoaders(context: Context) {
  const notificationsLoader = createDataLoader<string, any[]>(
    async (userIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'notification-service',
        'getUserNotifications',
        async () => {
          logger.debug('Batch loading user notifications', { count: userIds.length });

          const response = await fetch(`${process.env.NOTIFICATIONS_SUBGRAPH_URL}/batch/notifications`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ userIds }),
          });

          if (!response.ok) {
            throw new Error(`Notification service error: ${response.statusText}`);
          }

          const notifications = await response.json();
          
          return userIds.map(id => notifications[id] || []);
        }
      );
    }
  );

  return {
    notificationsLoader,
  };
}

// AI Service DataLoaders
export function createAIDataLoaders(context: Context) {
  const aiProcessingStatusLoader = createDataLoader<string, any>(
    async (jobIds) => {
      return tracingUtils.traceExternalCall(
        context,
        'ai-service',
        'getProcessingStatus',
        async () => {
          logger.debug('Batch loading AI processing status', { count: jobIds.length });

          const response = await fetch(`${process.env.AI_SUBGRAPH_URL}/batch/status`, {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${context.token}`,
            },
            body: JSON.stringify({ jobIds }),
          });

          if (!response.ok) {
            throw new Error(`AI service error: ${response.statusText}`);
          }

          const statuses = await response.json();
          
          return jobIds.map(id => 
            statuses[id] || new Error(`AI job not found: ${id}`)
          );
        }
      );
    }
  );

  return {
    aiProcessingStatusLoader,
  };
}

// Main DataLoader factory function
export function createDataLoaders(context: Context) {
  return {
    ...createUserDataLoaders(context),
    ...createWorkspaceDataLoaders(context),
    ...createAirtableDataLoaders(context),
    ...createFileDataLoaders(context),
    ...createNotificationDataLoaders(context),
    ...createAIDataLoaders(context),
  };
}

// DataLoader utilities
export const dataLoaderUtils = {
  // Prime cache with known data
  primeCache<K, V>(loader: DataLoader<K, V>, key: K, value: V): void {
    loader.prime(key, value);
  },

  // Clear cache for specific key
  clearCache<K, V>(loader: DataLoader<K, V>, key: K): void {
    loader.clear(key);
  },

  // Clear all caches
  clearAllCaches(dataloaders: ReturnType<typeof createDataLoaders>): void {
    Object.values(dataloaders).forEach(loader => {
      if (loader && typeof loader.clearAll === 'function') {
        loader.clearAll();
      }
    });
  },

  // Get cache statistics
  getCacheStats(dataloaders: ReturnType<typeof createDataLoaders>): Record<string, any> {
    const stats: Record<string, any> = {};
    
    Object.entries(dataloaders).forEach(([name, loader]) => {
      if (loader) {
        stats[name] = {
          cacheSize: (loader as any)._cache?.size || 0,
          // Add more stats as needed
        };
      }
    });
    
    return stats;
  },
};

// Error handling for DataLoaders
export function handleDataLoaderError(error: Error, operation: string): Error {
  logger.error('DataLoader error', { error: error.message, operation });
  
  // Return a more user-friendly error
  return new Error(`Failed to load ${operation}. Please try again.`);
}