/**
 * Optimistic Updates Pattern for PyAirtable
 * Provides immediate UI updates with rollback capabilities
 */

import { RecordService, FieldService, WorkspaceService } from '../types/api';
import { ServiceError } from '../client/service-clients';
import { AdvancedCacheManager } from '../cache/cache-strategies';

// ============================================================================
// Optimistic Update Types
// ============================================================================

export interface OptimisticUpdate<T = unknown> {
  id: string;
  type: OptimisticUpdateType;
  resourceId: string;
  resourceType: string;
  operation: 'create' | 'update' | 'delete';
  optimisticData: T;
  previousData?: T;
  timestamp: number;
  retryCount: number;
  maxRetries: number;
  status: 'pending' | 'confirmed' | 'failed' | 'rolled_back';
}

export type OptimisticUpdateType = 
  | 'record_create'
  | 'record_update' 
  | 'record_delete'
  | 'field_create'
  | 'field_update'
  | 'field_delete'
  | 'workspace_create'
  | 'workspace_update'
  | 'workspace_member_add'
  | 'workspace_member_remove';

export interface OptimisticContext {
  userId: string;
  tenantId: string;
  sessionId: string;
}

// ============================================================================
// Optimistic Update Manager
// ============================================================================

export class OptimisticUpdateManager {
  private updates: Map<string, OptimisticUpdate> = new Map();
  private rollbackHandlers: Map<OptimisticUpdateType, RollbackHandler> = new Map();
  private confirmHandlers: Map<OptimisticUpdateType, ConfirmHandler> = new Map();

  constructor(
    private cacheManager: AdvancedCacheManager,
    private context: OptimisticContext
  ) {
    this.setupDefaultHandlers();
  }

  // ============================================================================
  // Core Optimistic Update Methods
  // ============================================================================

  async createOptimisticUpdate<T>(
    type: OptimisticUpdateType,
    resourceId: string,
    resourceType: string,
    operation: 'create' | 'update' | 'delete',
    optimisticData: T,
    previousData?: T,
    maxRetries: number = 3
  ): Promise<string> {
    const update: OptimisticUpdate<T> = {
      id: `opt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      type,
      resourceId,
      resourceType,
      operation,
      optimisticData,
      previousData,
      timestamp: Date.now(),
      retryCount: 0,
      maxRetries,
      status: 'pending',
    };

    this.updates.set(update.id, update);

    // Apply optimistic update to cache
    this.applyOptimisticUpdate(update);

    return update.id;
  }

  confirmUpdate(updateId: string, confirmedData?: unknown): void {
    const update = this.updates.get(updateId);
    if (!update) return;

    update.status = 'confirmed';

    // Update cache with confirmed data if provided
    if (confirmedData) {
      this.updateCacheWithConfirmedData(update, confirmedData);
    }

    // Run confirm handler
    const handler = this.confirmHandlers.get(update.type);
    if (handler) {
      handler(update, confirmedData);
    }

    // Remove from pending updates after a delay
    setTimeout(() => {
      this.updates.delete(updateId);
    }, 5000);
  }

  rollbackUpdate(updateId: string, error?: ServiceError): void {
    const update = this.updates.get(updateId);
    if (!update) return;

    update.status = 'rolled_back';

    // Run rollback handler
    const handler = this.rollbackHandlers.get(update.type);
    if (handler) {
      handler(update, error);
    }

    // Restore previous state in cache
    this.rollbackCacheChanges(update);

    this.updates.delete(updateId);
  }

  retryUpdate(updateId: string): boolean {
    const update = this.updates.get(updateId);
    if (!update || update.retryCount >= update.maxRetries) {
      return false;
    }

    update.retryCount++;
    update.status = 'pending';
    update.timestamp = Date.now();

    return true;
  }

  // ============================================================================
  // Record-Specific Optimistic Updates
  // ============================================================================

  async optimisticCreateRecord(
    tableId: string,
    fields: Record<string, any>
  ): Promise<{ updateId: string; optimisticRecord: RecordService.Record }> {
    const optimisticRecord: RecordService.Record = {
      id: `temp_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      table_id: tableId,
      fields,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      created_by: this.context.userId,
      updated_by: this.context.userId,
    };

    const updateId = await this.createOptimisticUpdate(
      'record_create',
      optimisticRecord.id,
      'record',
      'create',
      optimisticRecord
    );

    return { updateId, optimisticRecord };
  }

  async optimisticUpdateRecord(
    recordId: string,
    fields: Record<string, any>,
    previousRecord: RecordService.Record
  ): Promise<{ updateId: string; optimisticRecord: RecordService.Record }> {
    const optimisticRecord: RecordService.Record = {
      ...previousRecord,
      fields: { ...previousRecord.fields, ...fields },
      updated_at: new Date().toISOString(),
      updated_by: this.context.userId,
    };

    const updateId = await this.createOptimisticUpdate(
      'record_update',
      recordId,
      'record',
      'update',
      optimisticRecord,
      previousRecord
    );

    return { updateId, optimisticRecord };
  }

  async optimisticDeleteRecord(
    recordId: string,
    previousRecord: RecordService.Record
  ): Promise<string> {
    return this.createOptimisticUpdate(
      'record_delete',
      recordId,
      'record',
      'delete',
      null,
      previousRecord
    );
  }

  // ============================================================================
  // Field-Specific Optimistic Updates
  // ============================================================================

  async optimisticCreateField(
    tableId: string,
    fieldData: FieldService.CreateFieldRequest
  ): Promise<{ updateId: string; optimisticField: FieldService.Field }> {
    const optimisticFields = this.getOptimisticFields(tableId);
    const maxOrder = Math.max(...optimisticFields.map(f => f.order), 0);

    const optimisticField: FieldService.Field = {
      id: `temp_field_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      name: fieldData.name,
      type: fieldData.type,
      table_id: tableId,
      is_primary: false,
      required: fieldData.required || false,
      order: maxOrder + 1,
      options: fieldData.options,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    const updateId = await this.createOptimisticUpdate(
      'field_create',
      optimisticField.id,
      'field',
      'create',
      optimisticField
    );

    return { updateId, optimisticField };
  }

  // ============================================================================
  // Workspace-Specific Optimistic Updates
  // ============================================================================

  async optimisticCreateWorkspace(
    workspaceData: WorkspaceService.CreateWorkspaceRequest
  ): Promise<{ updateId: string; optimisticWorkspace: WorkspaceService.Workspace }> {
    const optimisticWorkspace: WorkspaceService.Workspace = {
      id: `temp_ws_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
      name: workspaceData.name,
      description: workspaceData.description,
      owner_id: this.context.userId,
      tenant_id: this.context.tenantId,
      is_active: true,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      member_count: 1,
      base_count: 0,
    };

    const updateId = await this.createOptimisticUpdate(
      'workspace_create',
      optimisticWorkspace.id,
      'workspace',
      'create',
      optimisticWorkspace
    );

    return { updateId, optimisticWorkspace };
  }

  async optimisticAddWorkspaceMember(
    workspaceId: string,
    memberData: WorkspaceService.AddMemberRequest
  ): Promise<{ updateId: string; optimisticMember: WorkspaceService.WorkspaceMember }> {
    const optimisticMember: WorkspaceService.WorkspaceMember = {
      user_id: memberData.user_id,
      workspace_id: workspaceId,
      role: memberData.role,
      added_at: new Date().toISOString(),
      added_by: this.context.userId,
    };

    const updateId = await this.createOptimisticUpdate(
      'workspace_member_add',
      `${workspaceId}:${memberData.user_id}`,
      'workspace_member',
      'create',
      optimisticMember
    );

    return { updateId, optimisticMember };
  }

  // ============================================================================
  // Cache Integration
  // ============================================================================

  private applyOptimisticUpdate(update: OptimisticUpdate): void {
    switch (update.type) {
      case 'record_create':
        this.applyOptimisticRecordCreate(update);
        break;
      case 'record_update':
        this.applyOptimisticRecordUpdate(update);
        break;
      case 'record_delete':
        this.applyOptimisticRecordDelete(update);
        break;
      case 'field_create':
        this.applyOptimisticFieldCreate(update);
        break;
      case 'workspace_create':
        this.applyOptimisticWorkspaceCreate(update);
        break;
      case 'workspace_member_add':
        this.applyOptimisticMemberAdd(update);
        break;
    }
  }

  private applyOptimisticRecordCreate(update: OptimisticUpdate<RecordService.Record>): void {
    const record = update.optimisticData;
    
    // Add to single record cache
    this.cacheManager.set(`record:${record.id}`, record, { tags: [`table:${record.table_id}`] });
    
    // Invalidate record lists to trigger refetch with optimistic data
    this.cacheManager.invalidateByPattern(`records:${record.table_id}:`);
    
    // Add optimistic record to any cached lists
    this.addOptimisticRecordToLists(record);
  }

  private applyOptimisticRecordUpdate(update: OptimisticUpdate<RecordService.Record>): void {
    const record = update.optimisticData;
    
    // Update single record cache
    this.cacheManager.set(`record:${record.id}`, record, { tags: [`table:${record.table_id}`] });
    
    // Update record in cached lists
    this.updateOptimisticRecordInLists(record);
  }

  private applyOptimisticRecordDelete(update: OptimisticUpdate): void {
    // Remove from single record cache
    this.cacheManager.delete(`record:${update.resourceId}`);
    
    // Remove from cached lists
    this.removeOptimisticRecordFromLists(update.resourceId);
  }

  private applyOptimisticFieldCreate(update: OptimisticUpdate<FieldService.Field>): void {
    const field = update.optimisticData;
    
    // Add to field cache
    this.cacheManager.set(`field:${field.id}`, field, { tags: [`table:${field.table_id}`] });
    
    // Invalidate table schema
    this.cacheManager.invalidateByPattern(`table:${field.table_id}:schema`);
  }

  private applyOptimisticWorkspaceCreate(update: OptimisticUpdate<WorkspaceService.Workspace>): void {
    const workspace = update.optimisticData;
    
    // Add to workspace cache
    this.cacheManager.set(`workspace:${workspace.id}`, workspace, { tags: [`user:${this.context.userId}`] });
    
    // Invalidate workspace lists
    this.cacheManager.invalidateByPattern(`user:${this.context.userId}:workspaces`);
  }

  private applyOptimisticMemberAdd(update: OptimisticUpdate<WorkspaceService.WorkspaceMember>): void {
    const member = update.optimisticData;
    
    // Invalidate member lists
    this.cacheManager.invalidateByPattern(`workspace:${member.workspace_id}:members`);
  }

  private rollbackCacheChanges(update: OptimisticUpdate): void {
    switch (update.operation) {
      case 'create':
        // Remove optimistically created items
        this.cacheManager.delete(`${update.resourceType}:${update.resourceId}`);
        break;
      case 'update':
        // Restore previous data if available
        if (update.previousData) {
          this.cacheManager.set(`${update.resourceType}:${update.resourceId}`, update.previousData);
        }
        break;
      case 'delete':
        // Restore deleted data if available
        if (update.previousData) {
          this.cacheManager.set(`${update.resourceType}:${update.resourceId}`, update.previousData);
        }
        break;
    }

    // Invalidate related caches to trigger refetch
    this.invalidateRelatedCaches(update);
  }

  private updateCacheWithConfirmedData(update: OptimisticUpdate, confirmedData: unknown): void {
    // Replace optimistic data with confirmed data
    this.cacheManager.set(`${update.resourceType}:${update.resourceId}`, confirmedData);
    
    // Update any cached lists with confirmed data
    this.updateConfirmedDataInLists(update, confirmedData);
  }

  // ============================================================================
  // Helper Methods
  // ============================================================================

  private getOptimisticFields(tableId: string): FieldService.Field[] {
    // Get fields from cache, including optimistic ones
    const cachedFields = this.cacheManager.get<FieldService.Field[]>(`table:${tableId}:fields`) || [];
    
    // Add pending optimistic field creates
    const optimisticFields = Array.from(this.updates.values())
      .filter(update => update.type === 'field_create' && update.status === 'pending')
      .map(update => update.optimisticData as FieldService.Field)
      .filter(field => field.table_id === tableId);
    
    return [...cachedFields, ...optimisticFields];
  }

  private addOptimisticRecordToLists(record: RecordService.Record): void {
    // This would add the optimistic record to any cached record lists
    // Implementation depends on how lists are cached
  }

  private updateOptimisticRecordInLists(record: RecordService.Record): void {
    // This would update the optimistic record in any cached record lists
  }

  private removeOptimisticRecordFromLists(recordId: string): void {
    // This would remove the optimistic record from any cached record lists
  }

  private updateConfirmedDataInLists(update: OptimisticUpdate, confirmedData: unknown): void {
    // This would update confirmed data in any cached lists
  }

  private invalidateRelatedCaches(update: OptimisticUpdate): void {
    switch (update.resourceType) {
      case 'record':
        this.cacheManager.invalidateByTag(`table:${(update.optimisticData as any)?.table_id}`);
        break;
      case 'field':
        this.cacheManager.invalidateByTag(`table:${(update.optimisticData as any)?.table_id}`);
        break;
      case 'workspace':
        this.cacheManager.invalidateByTag(`user:${this.context.userId}`);
        break;
    }
  }

  // ============================================================================
  // Handler Setup
  // ============================================================================

  private setupDefaultHandlers(): void {
    // Record handlers
    this.rollbackHandlers.set('record_create', (update, error) => {
      console.warn(`Rolling back record creation: ${update.resourceId}`, error);
    });

    this.rollbackHandlers.set('record_update', (update, error) => {
      console.warn(`Rolling back record update: ${update.resourceId}`, error);
    });

    this.rollbackHandlers.set('record_delete', (update, error) => {
      console.warn(`Rolling back record deletion: ${update.resourceId}`, error);
    });

    // Confirm handlers
    this.confirmHandlers.set('record_create', (update, confirmedData) => {
      console.log(`Confirmed record creation: ${update.resourceId}`);
    });

    this.confirmHandlers.set('record_update', (update, confirmedData) => {
      console.log(`Confirmed record update: ${update.resourceId}`);
    });
  }

  // ============================================================================
  // Public Interface
  // ============================================================================

  getPendingUpdates(): OptimisticUpdate[] {
    return Array.from(this.updates.values()).filter(update => update.status === 'pending');
  }

  hasOptimisticUpdates(): boolean {
    return this.getPendingUpdates().length > 0;
  }

  getUpdateById(updateId: string): OptimisticUpdate | undefined {
    return this.updates.get(updateId);
  }

  clearAllUpdates(): void {
    this.updates.clear();
  }

  setRollbackHandler(type: OptimisticUpdateType, handler: RollbackHandler): void {
    this.rollbackHandlers.set(type, handler);
  }

  setConfirmHandler(type: OptimisticUpdateType, handler: ConfirmHandler): void {
    this.confirmHandlers.set(type, handler);
  }
}

// ============================================================================
// Handler Types
// ============================================================================

export type RollbackHandler = (update: OptimisticUpdate, error?: ServiceError) => void;
export type ConfirmHandler = (update: OptimisticUpdate, confirmedData?: unknown) => void;

// ============================================================================
// React Hook Interfaces
// ============================================================================

export interface UseOptimisticOptions {
  maxRetries?: number;
  timeout?: number;
}

// These would be implemented in the React application
export function useOptimisticUpdate<T>(
  mutationFn: () => Promise<T>,
  options?: UseOptimisticOptions
): {
  mutate: () => Promise<T>;
  isOptimistic: boolean;
  rollback: () => void;
  error: ServiceError | null;
} {
  throw new Error('useOptimisticUpdate hook must be implemented in the React application');
}

export function useOptimisticState<T>(
  initialData: T,
  resourceId: string
): [T, (newData: T) => void, boolean] {
  throw new Error('useOptimisticState hook must be implemented in the React application');
}