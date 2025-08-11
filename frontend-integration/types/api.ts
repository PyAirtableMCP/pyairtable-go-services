/**
 * Comprehensive TypeScript definitions for PyAirtable backend services
 * Auto-generated from Go service definitions - keep in sync with backend
 */

// ============================================================================
// Common Types
// ============================================================================

export interface APIResponse<T = unknown> {
  data?: T;
  error?: string;
  status: number;
  message?: string;
  timestamp?: number;
  path?: string;
  method?: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    page: number;
    per_page: number;
    total: number;
    total_pages: number;
    has_next: boolean;
    has_prev: boolean;
  };
}

export interface ErrorResponse {
  error: string;
  status: number;
  path?: string;
  method?: string;
  timestamp: number;
  details?: Record<string, unknown>;
}

// ============================================================================
// Authentication Service Types (Port 8081)
// ============================================================================

export namespace AuthService {
  export interface User {
    id: string;
    email: string;
    first_name: string;
    last_name: string;
    role: string;
    tenant_id: string;
    is_active: boolean;
    email_verified: boolean;
    created_at: string;
    updated_at: string;
    last_login_at?: string;
  }

  export interface LoginRequest {
    email: string;
    password: string;
  }

  export interface RegisterRequest {
    email: string;
    password: string;
    first_name: string;
    last_name: string;
    tenant_id?: string;
  }

  export interface TokenResponse {
    access_token: string;
    refresh_token: string;
    token_type: string;
    expires_in: number;
  }

  export interface RefreshRequest {
    refresh_token: string;
  }

  export interface ChangePasswordRequest {
    current_password: string;
    new_password: string;
  }

  export interface Claims {
    user_id: string;
    email: string;
    role: string;
    tenant_id: string;
    exp: number;
    iat: number;
  }
}

// ============================================================================
// User Service Types (Port 8082)
// ============================================================================

export namespace UserService {
  export interface UserProfile extends AuthService.User {
    preferences?: Record<string, unknown>;
    settings?: UserSettings;
  }

  export interface UserSettings {
    theme: 'light' | 'dark' | 'system';
    language: string;
    timezone: string;
    notifications: NotificationSettings;
  }

  export interface NotificationSettings {
    email: boolean;
    push: boolean;
    in_app: boolean;
    webhooks: boolean;
  }

  export interface UpdateUserRequest {
    first_name?: string;
    last_name?: string;
    preferences?: Record<string, unknown>;
    settings?: Partial<UserSettings>;
  }
}

// ============================================================================
// Workspace Service Types (Port 8083)
// ============================================================================

export namespace WorkspaceService {
  export interface Workspace {
    id: string;
    name: string;
    description?: string;
    owner_id: string;
    tenant_id: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
    member_count: number;
    base_count: number;
  }

  export interface WorkspaceMember {
    user_id: string;
    workspace_id: string;
    role: WorkspaceRole;
    added_at: string;
    added_by: string;
    user?: UserService.UserProfile;
  }

  export type WorkspaceRole = 'owner' | 'admin' | 'editor' | 'viewer';

  export interface CreateWorkspaceRequest {
    name: string;
    description?: string;
  }

  export interface UpdateWorkspaceRequest {
    name?: string;
    description?: string;
  }

  export interface AddMemberRequest {
    user_id: string;
    role: WorkspaceRole;
  }
}

// ============================================================================
// Base Service Types (Port 8084)
// ============================================================================

export namespace BaseService {
  export interface Base {
    id: string;
    name: string;
    description?: string;
    workspace_id: string;
    owner_id: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
    table_count: number;
  }

  export interface CreateBaseRequest {
    name: string;
    description?: string;
    workspace_id: string;
  }

  export interface UpdateBaseRequest {
    name?: string;
    description?: string;
  }
}

// ============================================================================
// Table Service Types (Port 8085)
// ============================================================================

export namespace TableService {
  export interface Table {
    id: string;
    name: string;
    description?: string;
    base_id: string;
    primary_field_id?: string;
    created_at: string;
    updated_at: string;
    record_count: number;
    field_count: number;
  }

  export interface CreateTableRequest {
    name: string;
    description?: string;
    base_id: string;
  }

  export interface UpdateTableRequest {
    name?: string;
    description?: string;
  }
}

// ============================================================================
// Field Service Types (Port 8088)
// ============================================================================

export namespace FieldService {
  export interface Field {
    id: string;
    name: string;
    type: FieldType;
    table_id: string;
    is_primary: boolean;
    required: boolean;
    order: number;
    options?: FieldOptions;
    created_at: string;
    updated_at: string;
  }

  export type FieldType = 
    | 'text'
    | 'number'
    | 'date'
    | 'datetime'
    | 'select'
    | 'multiselect'
    | 'checkbox'
    | 'email'
    | 'url' 
    | 'phone'
    | 'attachment'
    | 'formula'
    | 'lookup'
    | 'rollup'
    | 'reference';

  export interface FieldOptions {
    // Text options
    max_length?: number;
    
    // Number options
    format?: 'integer' | 'decimal' | 'currency' | 'percentage';
    precision?: number;
    
    // Select options
    choices?: SelectChoice[];
    
    // Date options
    date_format?: string;
    include_time?: boolean;
    
    // Formula options
    formula?: string;
    
    // Reference options
    referenced_table_id?: string;
    reference_field_id?: string;
  }

  export interface SelectChoice {
    id: string;
    name: string;
    color?: string;
  }

  export interface CreateFieldRequest {
    name: string;
    type: FieldType;
    table_id: string;
    required?: boolean;
    options?: FieldOptions;
  }
}

// ============================================================================
// Record Service Types (Port 8087)
// ============================================================================

export namespace RecordService {
  export interface Record {
    id: string;
    table_id: string;
    fields: Record<string, FieldValue>;
    created_at: string;
    updated_at: string;
    created_by: string;
    updated_by: string;
  }

  export type FieldValue = 
    | string 
    | number 
    | boolean 
    | Date 
    | string[] 
    | Attachment[] 
    | RecordReference[]
    | null;

  export interface Attachment {
    id: string;
    filename: string;
    size: number;
    type: string;
    url: string;
    thumbnails?: AttachmentThumbnail[];
  }

  export interface AttachmentThumbnail {
    type: 'small' | 'large';
    url: string;
    width: number;
    height: number;
  }

  export interface RecordReference {
    id: string;
    display_value?: string;
  }

  export interface CreateRecordRequest {
    table_id: string;
    fields: Record<string, FieldValue>;
  }

  export interface UpdateRecordRequest {
    fields: Record<string, FieldValue>;
  }

  export interface ListRecordsQuery {
    table_id: string;
    view_id?: string;
    fields?: string[];
    filter_by_formula?: string;
    sort?: SortSpec[];
    page_size?: number;
    offset?: string;
  }

  export interface SortSpec {
    field: string;
    direction: 'asc' | 'desc';
  }
}

// ============================================================================
// View Service Types (Port 8086)
// ============================================================================

export namespace ViewService {
  export interface View {
    id: string;
    name: string;
    type: ViewType;
    table_id: string;
    configuration: ViewConfiguration;
    created_at: string;
    updated_at: string;
  }

  export type ViewType = 'grid' | 'form' | 'calendar' | 'gallery' | 'kanban';

  export interface ViewConfiguration {
    visible_fields?: string[];
    hidden_fields?: string[];
    field_order?: string[];
    sorts?: SortSpec[];
    filters?: FilterSpec[];
    grouping?: GroupingSpec;
  }

  export interface FilterSpec {
    field_id: string;
    operator: FilterOperator;
    value: unknown;
  }

  export type FilterOperator = 
    | 'equals'
    | 'not_equals' 
    | 'contains'
    | 'not_contains'
    | 'starts_with'
    | 'ends_with'
    | 'is_empty'
    | 'is_not_empty'
    | 'greater_than'
    | 'less_than'
    | 'greater_equal'
    | 'less_equal';

  export interface GroupingSpec {
    field_id: string;
    direction: 'asc' | 'desc';
  }

  export interface SortSpec {
    field_id: string;
    direction: 'asc' | 'desc';
  }
}

// ============================================================================
// File Service Types (Port 8092)
// ============================================================================

export namespace FileService {
  export interface FileRecord {
    id: string;
    filename: string;
    size: number;
    type: string;
    checksum: string;
    storage_path: string;
    public_url?: string;
    uploaded_by: string;
    uploaded_at: string;
    metadata?: Record<string, unknown>;
  }

  export interface UploadRequest {
    file: File;
    table_id?: string;
    record_id?: string;
    field_id?: string;
  }

  export interface UploadResponse {
    file: FileRecord;
    upload_url?: string;
  }
}

// ============================================================================
// Webhook Service Types (Port 8096)
// ============================================================================

export namespace WebhookService {
  export interface Webhook {
    id: string;
    url: string;
    event_types: string[];
    is_active: boolean;
    secret: string;
    created_by: string;
    created_at: string;
    updated_at: string;
    last_triggered_at?: string;
    failure_count: number;
  }

  export interface WebhookDelivery {
    id: string;
    webhook_id: string;
    event_type: string;
    payload: Record<string, unknown>;
    status: 'pending' | 'delivered' | 'failed';
    attempts: number;
    last_attempted_at?: string;
    delivered_at?: string;
    response_status?: number;
    response_body?: string;
  }

  export interface CreateWebhookRequest {
    url: string;
    event_types: string[];
    secret?: string;
  }
}

// ============================================================================
// Real-time Event Types
// ============================================================================

export namespace RealtimeEvents {
  export interface BaseEvent {
    id: string;
    type: string;
    aggregate_id: string;
    aggregate_type: string;
    version: number;
    occurred_at: string;
    tenant_id: string;
    user_id?: string;
  }

  export interface RecordCreatedEvent extends BaseEvent {
    type: 'record.created';
    payload: {
      table_id: string;
      record: RecordService.Record;
    };
  }

  export interface RecordUpdatedEvent extends BaseEvent {
    type: 'record.updated';
    payload: {
      table_id: string;
      record: RecordService.Record;
      changes: Record<string, { old: unknown; new: unknown }>;
    };
  }

  export interface RecordDeletedEvent extends BaseEvent {
    type: 'record.deleted';
    payload: {
      table_id: string;
      record_id: string;
    };
  }

  export interface WorkspaceMemberAddedEvent extends BaseEvent {
    type: 'workspace.member_added';
    payload: {
      workspace_id: string;
      member: WorkspaceService.WorkspaceMember;
    };
  }

  export type DomainEvent = 
    | RecordCreatedEvent
    | RecordUpdatedEvent
    | RecordDeletedEvent
    | WorkspaceMemberAddedEvent;
}

// ============================================================================
// API Client Configuration
// ============================================================================

export interface APIClientConfig {
  baseURL: string;
  timeout: number;
  retryAttempts: number;
  retryDelay: number;
  enableCaching: boolean;
  cacheTimeout: number;
  enableOptimisticUpdates: boolean;
  enableOfflineQueue: boolean;
}

export interface ServiceEndpoint {
  name: string;
  url: string;
  port: number;
  healthCheck: string;
  timeout: number;
  retryCount: number;
  weight: number;
}

// ============================================================================
// Cache Types
// ============================================================================

export interface CacheEntry<T> {
  data: T;
  timestamp: number;
  expiration: number;
  etag?: string;
}

export interface CacheStrategy {
  ttl: number;
  staleWhileRevalidate: boolean;
  maxAge: number;
  cacheKey: (params: unknown) => string;
}