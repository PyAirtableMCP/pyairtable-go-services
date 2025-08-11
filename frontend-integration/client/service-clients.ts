/**
 * Service-specific API clients with comprehensive error handling
 * Provides typed interfaces for each microservice
 */

import {
  APIResponse,
  PaginatedResponse,
  AuthService,
  UserService,
  WorkspaceService,
  BaseService,
  TableService,
  FieldService,
  RecordService,
  ViewService,
  FileService,
  WebhookService,
} from '../types/api';
import { HTTPClient, APIClientFactory, APIError, AuthError, NetworkError } from './api-client';

// ============================================================================
// Error Handling Utilities
// ============================================================================

export class ErrorHandler {
  static handle(error: unknown): never {
    if (error instanceof APIError) {
      throw new ServiceError(error.response.error, error.response.status, 'API_ERROR');
    }
    
    if (error instanceof AuthError) {
      throw new ServiceError(error.message, 401, 'AUTH_ERROR');
    }
    
    if (error instanceof NetworkError) {
      throw new ServiceError(error.message, 0, 'NETWORK_ERROR');
    }
    
    if (error instanceof Error) {
      throw new ServiceError(error.message, 500, 'UNKNOWN_ERROR');
    }
    
    throw new ServiceError('An unexpected error occurred', 500, 'UNKNOWN_ERROR');
  }

  static isRetryable(error: ServiceError): boolean {
    return error.code >= 500 || error.type === 'NETWORK_ERROR';
  }

  static getErrorMessage(error: ServiceError): string {
    switch (error.type) {
      case 'AUTH_ERROR':
        return 'Authentication required. Please sign in again.';
      case 'NETWORK_ERROR':
        return 'Network connection error. Please check your internet connection.';
      case 'API_ERROR':
        return error.message || 'An error occurred while processing your request.';
      default:
        return 'An unexpected error occurred. Please try again.';
    }
  }
}

export class ServiceError extends Error {
  constructor(
    message: string,
    public code: number,
    public type: 'API_ERROR' | 'AUTH_ERROR' | 'NETWORK_ERROR' | 'UNKNOWN_ERROR',
    public details?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'ServiceError';
  }
}

// ============================================================================
// Base Service Client
// ============================================================================

abstract class BaseServiceClient {
  protected httpClient: HTTPClient;
  protected serviceName: string;

  constructor(serviceName: string) {
    this.httpClient = APIClientFactory.getInstance();
    this.serviceName = serviceName;
  }

  protected async request<T>(config: {
    method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
    url: string;
    data?: unknown;
    params?: Record<string, string | number | boolean>;
  }): Promise<T> {
    try {
      const response = await this.httpClient.request<T>(this.serviceName, config);
      if (response.data === undefined) {
        throw new ServiceError('No data received from server', 500, 'API_ERROR');
      }
      return response.data;
    } catch (error) {
      return ErrorHandler.handle(error);
    }
  }

  protected async requestWithPagination<T>(config: {
    method: 'GET';
    url: string;
    params?: Record<string, string | number | boolean>;
  }): Promise<PaginatedResponse<T>> {
    return this.request<PaginatedResponse<T>>(config);
  }
}

// ============================================================================
// Authentication Service Client
// ============================================================================

export class AuthServiceClient extends BaseServiceClient {
  constructor() {
    super('auth');
  }

  async login(credentials: AuthService.LoginRequest): Promise<AuthService.TokenResponse> {
    return this.request({
      method: 'POST',
      url: '/auth/login',
      data: credentials,
    });
  }

  async register(data: AuthService.RegisterRequest): Promise<AuthService.User> {
    return this.request({
      method: 'POST',
      url: '/auth/register',
      data,
    });
  }

  async refreshToken(refreshToken: string): Promise<AuthService.TokenResponse> {
    return this.request({
      method: 'POST',
      url: '/auth/refresh',
      data: { refresh_token: refreshToken },
    });
  }

  async logout(): Promise<void> {
    await this.request({
      method: 'POST',
      url: '/auth/logout',
    });
  }

  async getMe(): Promise<AuthService.User> {
    return this.request({
      method: 'GET',
      url: '/auth/me',
    });
  }

  async updateMe(data: Partial<AuthService.User>): Promise<AuthService.User> {
    return this.request({
      method: 'PUT',
      url: '/auth/me',
      data,
    });
  }

  async changePassword(data: AuthService.ChangePasswordRequest): Promise<void> {
    await this.request({
      method: 'POST',
      url: '/auth/change-password',
      data,
    });
  }

  async validateToken(token: string): Promise<{ valid: boolean }> {
    return this.request({
      method: 'POST',
      url: '/auth/validate',
      data: { token },
    });
  }
}

// ============================================================================
// User Service Client
// ============================================================================

export class UserServiceClient extends BaseServiceClient {
  constructor() {
    super('user');
  }

  async getProfile(): Promise<UserService.UserProfile> {
    return this.request({
      method: 'GET',
      url: '/users/me',
    });
  }

  async updateProfile(data: UserService.UpdateUserRequest): Promise<UserService.UserProfile> {
    return this.request({
      method: 'PUT',
      url: '/users/me',
      data,
    });
  }

  async getAllUsers(params?: { page?: number; per_page?: number }): Promise<PaginatedResponse<UserService.UserProfile>> {
    return this.requestWithPagination({
      method: 'GET',
      url: '/users',
      params,
    });
  }

  async getUserById(id: string): Promise<UserService.UserProfile> {
    return this.request({
      method: 'GET',
      url: `/users/${id}`,
    });
  }
}

// ============================================================================
// Workspace Service Client
// ============================================================================

export class WorkspaceServiceClient extends BaseServiceClient {
  constructor() {
    super('workspace');
  }

  async getWorkspaces(params?: { page?: number; per_page?: number }): Promise<PaginatedResponse<WorkspaceService.Workspace>> {
    return this.requestWithPagination({
      method: 'GET',
      url: '/workspaces',
      params,
    });
  }

  async getWorkspace(id: string): Promise<WorkspaceService.Workspace> {
    return this.request({
      method: 'GET',
      url: `/workspaces/${id}`,
    });
  }

  async createWorkspace(data: WorkspaceService.CreateWorkspaceRequest): Promise<WorkspaceService.Workspace> {
    return this.request({
      method: 'POST',
      url: '/workspaces',
      data,
    });
  }

  async updateWorkspace(id: string, data: WorkspaceService.UpdateWorkspaceRequest): Promise<WorkspaceService.Workspace> {
    return this.request({
      method: 'PUT',
      url: `/workspaces/${id}`,
      data,
    });
  }

  async deleteWorkspace(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/workspaces/${id}`,
    });
  }

  async getWorkspaceMembers(workspaceId: string): Promise<WorkspaceService.WorkspaceMember[]> {
    return this.request({
      method: 'GET',
      url: `/workspaces/${workspaceId}/members`,
    });
  }

  async addWorkspaceMember(workspaceId: string, data: WorkspaceService.AddMemberRequest): Promise<WorkspaceService.WorkspaceMember> {
    return this.request({
      method: 'POST',
      url: `/workspaces/${workspaceId}/members`,
      data,
    });
  }

  async removeWorkspaceMember(workspaceId: string, userId: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/workspaces/${workspaceId}/members/${userId}`,
    });
  }

  async updateMemberRole(workspaceId: string, userId: string, role: WorkspaceService.WorkspaceRole): Promise<WorkspaceService.WorkspaceMember> {
    return this.request({
      method: 'PATCH',
      url: `/workspaces/${workspaceId}/members/${userId}`,
      data: { role },
    });
  }
}

// ============================================================================
// Base Service Client
// ============================================================================

export class BaseServiceClient extends BaseServiceClient {
  constructor() {
    super('base');
  }

  async getBases(params?: { workspace_id?: string; page?: number; per_page?: number }): Promise<PaginatedResponse<BaseService.Base>> {
    return this.requestWithPagination({
      method: 'GET',
      url: '/bases',
      params,
    });
  }

  async getBase(id: string): Promise<BaseService.Base> {
    return this.request({
      method: 'GET',
      url: `/bases/${id}`,
    });
  }

  async createBase(data: BaseService.CreateBaseRequest): Promise<BaseService.Base> {
    return this.request({
      method: 'POST',
      url: '/bases',
      data,
    });
  }

  async updateBase(id: string, data: BaseService.UpdateBaseRequest): Promise<BaseService.Base> {
    return this.request({
      method: 'PUT',
      url: `/bases/${id}`,
      data,
    });
  }

  async deleteBase(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/bases/${id}`,
    });
  }
}

// ============================================================================
// Table Service Client
// ============================================================================

export class TableServiceClient extends BaseServiceClient {
  constructor() {
    super('table');
  }

  async getTables(params?: { base_id?: string; page?: number; per_page?: number }): Promise<PaginatedResponse<TableService.Table>> {
    return this.requestWithPagination({
      method: 'GET',
      url: '/tables',
      params,
    });
  }

  async getTable(id: string): Promise<TableService.Table> {
    return this.request({
      method: 'GET',
      url: `/tables/${id}`,
    });
  }

  async createTable(data: TableService.CreateTableRequest): Promise<TableService.Table> {
    return this.request({
      method: 'POST',
      url: '/tables',
      data,
    });
  }

  async updateTable(id: string, data: TableService.UpdateTableRequest): Promise<TableService.Table> {
    return this.request({
      method: 'PUT',
      url: `/tables/${id}`,
      data,
    });
  }

  async deleteTable(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/tables/${id}`,
    });
  }
}

// ============================================================================
// Field Service Client
// ============================================================================

export class FieldServiceClient extends BaseServiceClient {
  constructor() {
    super('field');
  }

  async getFields(params?: { table_id?: string }): Promise<FieldService.Field[]> {
    return this.request({
      method: 'GET',
      url: '/fields',
      params,
    });
  }

  async getField(id: string): Promise<FieldService.Field> {
    return this.request({
      method: 'GET',
      url: `/fields/${id}`,
    });
  }

  async createField(data: FieldService.CreateFieldRequest): Promise<FieldService.Field> {
    return this.request({
      method: 'POST',
      url: '/fields',
      data,
    });
  }

  async updateField(id: string, data: Partial<FieldService.Field>): Promise<FieldService.Field> {
    return this.request({
      method: 'PUT',
      url: `/fields/${id}`,
      data,
    });
  }

  async deleteField(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/fields/${id}`,
    });
  }
}

// ============================================================================
// Record Service Client
// ============================================================================

export class RecordServiceClient extends BaseServiceClient {
  constructor() {
    super('record');
  }

  async getRecords(query: RecordService.ListRecordsQuery): Promise<PaginatedResponse<RecordService.Record>> {
    return this.requestWithPagination({
      method: 'GET',
      url: '/records',
      params: query as Record<string, string | number | boolean>,
    });
  }

  async getRecord(id: string): Promise<RecordService.Record> {
    return this.request({
      method: 'GET',
      url: `/records/${id}`,
    });
  }

  async createRecord(data: RecordService.CreateRecordRequest): Promise<RecordService.Record> {
    return this.request({
      method: 'POST',
      url: '/records',
      data,
    });
  }

  async updateRecord(id: string, data: RecordService.UpdateRecordRequest): Promise<RecordService.Record> {
    return this.request({
      method: 'PUT',
      url: `/records/${id}`,
      data,
    });
  }

  async deleteRecord(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/records/${id}`,
    });
  }

  async batchCreateRecords(records: RecordService.CreateRecordRequest[]): Promise<RecordService.Record[]> {
    return this.request({
      method: 'POST',
      url: '/records/batch',
      data: { records },
    });
  }

  async batchUpdateRecords(updates: Array<{ id: string; fields: Record<string, any> }>): Promise<RecordService.Record[]> {
    return this.request({
      method: 'PATCH',
      url: '/records/batch',
      data: { updates },
    });
  }

  async batchDeleteRecords(ids: string[]): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: '/records/batch',
      data: { ids },
    });
  }
}

// ============================================================================
// View Service Client
// ============================================================================

export class ViewServiceClient extends BaseServiceClient {
  constructor() {
    super('view');
  }

  async getViews(params?: { table_id?: string }): Promise<ViewService.View[]> {
    return this.request({
      method: 'GET',
      url: '/views',
      params,
    });
  }

  async getView(id: string): Promise<ViewService.View> {
    return this.request({
      method: 'GET',
      url: `/views/${id}`,
    });
  }

  async createView(data: Partial<ViewService.View>): Promise<ViewService.View> {
    return this.request({
      method: 'POST',
      url: '/views',
      data,
    });
  }

  async updateView(id: string, data: Partial<ViewService.View>): Promise<ViewService.View> {
    return this.request({
      method: 'PUT',
      url: `/views/${id}`,
      data,
    });
  }

  async deleteView(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/views/${id}`,
    });
  }
}

// ============================================================================
// File Service Client
// ============================================================================

export class FileServiceClient extends BaseServiceClient {
  constructor() {
    super('file');
  }

  async uploadFile(data: FileService.UploadRequest): Promise<FileService.UploadResponse> {
    const formData = new FormData();
    formData.append('file', data.file);
    
    if (data.table_id) formData.append('table_id', data.table_id);
    if (data.record_id) formData.append('record_id', data.record_id);
    if (data.field_id) formData.append('field_id', data.field_id);

    // Note: This bypasses the JSON request wrapper for file uploads
    try {
      const response = await this.httpClient.request<FileService.UploadResponse>('file', {
        method: 'POST',
        url: '/files/upload',
        data: formData,
      });
      return response.data!;
    } catch (error) {
      return ErrorHandler.handle(error);
    }
  }

  async getFile(id: string): Promise<FileService.FileRecord> {
    return this.request({
      method: 'GET',
      url: `/files/${id}`,
    });
  }

  async deleteFile(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/files/${id}`,
    });
  }

  async getFileUrl(id: string): Promise<{ url: string }> {
    return this.request({
      method: 'GET',
      url: `/files/${id}/url`,
    });
  }
}

// ============================================================================
// Webhook Service Client
// ============================================================================

export class WebhookServiceClient extends BaseServiceClient {
  constructor() {
    super('webhook');
  }

  async getWebhooks(): Promise<WebhookService.Webhook[]> {
    return this.request({
      method: 'GET',
      url: '/webhooks',
    });
  }

  async getWebhook(id: string): Promise<WebhookService.Webhook> {
    return this.request({
      method: 'GET',
      url: `/webhooks/${id}`,
    });
  }

  async createWebhook(data: WebhookService.CreateWebhookRequest): Promise<WebhookService.Webhook> {
    return this.request({
      method: 'POST',
      url: '/webhooks',
      data,
    });
  }

  async updateWebhook(id: string, data: Partial<WebhookService.Webhook>): Promise<WebhookService.Webhook> {
    return this.request({
      method: 'PUT',
      url: `/webhooks/${id}`,
      data,
    });
  }

  async deleteWebhook(id: string): Promise<void> {
    await this.request({
      method: 'DELETE',
      url: `/webhooks/${id}`,
    });
  }

  async getWebhookDeliveries(webhookId: string): Promise<WebhookService.WebhookDelivery[]> {
    return this.request({
      method: 'GET',
      url: `/webhooks/${webhookId}/deliveries`,
    });
  }

  async retryWebhookDelivery(webhookId: string, deliveryId: string): Promise<void> {
    await this.request({
      method: 'POST',
      url: `/webhooks/${webhookId}/deliveries/${deliveryId}/retry`,
    });
  }
}

// ============================================================================
// Service Client Factory
// ============================================================================

export class ServiceClientFactory {
  private static instances: Map<string, BaseServiceClient> = new Map();

  static getAuthService(): AuthServiceClient {
    if (!this.instances.has('auth')) {
      this.instances.set('auth', new AuthServiceClient());
    }
    return this.instances.get('auth') as AuthServiceClient;
  }

  static getUserService(): UserServiceClient {
    if (!this.instances.has('user')) {
      this.instances.set('user', new UserServiceClient());
    }
    return this.instances.get('user') as UserServiceClient;
  }

  static getWorkspaceService(): WorkspaceServiceClient {
    if (!this.instances.has('workspace')) {
      this.instances.set('workspace', new WorkspaceServiceClient());
    }
    return this.instances.get('workspace') as WorkspaceServiceClient;
  }

  static getBaseService(): BaseServiceClient {
    if (!this.instances.has('base')) {
      this.instances.set('base', new BaseServiceClient());
    }
    return this.instances.get('base') as BaseServiceClient;
  }

  static getTableService(): TableServiceClient {
    if (!this.instances.has('table')) {
      this.instances.set('table', new TableServiceClient());
    }
    return this.instances.get('table') as TableServiceClient;
  }

  static getFieldService(): FieldServiceClient {
    if (!this.instances.has('field')) {
      this.instances.set('field', new FieldServiceClient());
    }
    return this.instances.get('field') as FieldServiceClient;
  }

  static getRecordService(): RecordServiceClient {
    if (!this.instances.has('record')) {
      this.instances.set('record', new RecordServiceClient());
    }
    return this.instances.get('record') as RecordServiceClient;
  }

  static getViewService(): ViewServiceClient {
    if (!this.instances.has('view')) {
      this.instances.set('view', new ViewServiceClient());
    }
    return this.instances.get('view') as ViewServiceClient;
  }

  static getFileService(): FileServiceClient {
    if (!this.instances.has('file')) {
      this.instances.set('file', new FileServiceClient());
    }
    return this.instances.get('file') as FileServiceClient;
  }

  static getWebhookService(): WebhookServiceClient {
    if (!this.instances.has('webhook')) {
      this.instances.set('webhook', new WebhookServiceClient());
    }
    return this.instances.get('webhook') as WebhookServiceClient;
  }

  static clearInstances(): void {
    this.instances.clear();
  }
}

// ============================================================================
// Convenience Export
// ============================================================================

export const apiClients = {
  auth: () => ServiceClientFactory.getAuthService(),
  user: () => ServiceClientFactory.getUserService(),
  workspace: () => ServiceClientFactory.getWorkspaceService(),
  base: () => ServiceClientFactory.getBaseService(),
  table: () => ServiceClientFactory.getTableService(),
  field: () => ServiceClientFactory.getFieldService(),
  record: () => ServiceClientFactory.getRecordService(),
  view: () => ServiceClientFactory.getViewService(),
  file: () => ServiceClientFactory.getFileService(),
  webhook: () => ServiceClientFactory.getWebhookService(),
};