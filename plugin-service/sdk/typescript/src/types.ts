/**
 * Core types for PyAirtable Plugin SDK
 */

// Plugin Types
export type PluginType = 'formula' | 'ui' | 'webhook' | 'automation' | 'connector' | 'view';

export type PluginStatus = 'pending' | 'installed' | 'active' | 'suspended' | 'deprecated' | 'error';

// Core Plugin Interface
export interface Plugin {
  /** Plugin metadata */
  readonly manifest: PluginManifest;
  
  /** Initialize the plugin */
  initialize?(context: PluginContext): Promise<void> | void;
  
  /** Cleanup when plugin is unloaded */
  cleanup?(): Promise<void> | void;
  
  /** Handle plugin execution */
  execute?(functionName: string, input: any, context: ExecutionContext): Promise<PluginResult>;
  
  /** Handle configuration changes */
  onConfigChange?(newConfig: Record<string, any>): Promise<void> | void;
  
  /** Handle events */
  onEvent?(event: PluginEvent): Promise<void> | void;
}

// Plugin Manifest
export interface PluginManifest {
  name: string;
  version: string;
  type: PluginType;
  description: string;
  author: string;
  license?: string;
  homepage?: string;
  repository?: string;
  
  /** Entry point function name */
  entryPoint: string;
  
  /** Plugin dependencies */
  dependencies?: PluginDependency[];
  
  /** Required permissions */
  permissions?: Permission[];
  
  /** Resource limits */
  resourceLimits?: ResourceLimits;
  
  /** Configuration schema */
  config?: ConfigSchema;
  
  /** Plugin tags */
  tags?: string[];
  
  /** Plugin categories */
  categories?: string[];
  
  /** Minimum API version required */
  minAPIVersion?: string;
  
  /** Maximum API version supported */
  maxAPIVersion?: string;
}

// Plugin Dependency
export interface PluginDependency {
  name: string;
  version: string;
  type: 'plugin' | 'library' | 'service';
}

// Permissions
export interface Permission {
  resource: string;
  actions: string[];
  scope?: string;
}

// Resource Limits
export interface ResourceLimits {
  maxMemoryMB: number;
  maxCPUPercent: number;
  maxExecutionMs: number;
  maxStorageMB: number;
  maxNetworkReqs: number;
}

// Configuration Schema
export interface ConfigSchema {
  schema?: any; // JSON Schema
  uiSchema?: any; // UI Schema for rendering forms
  defaultData?: Record<string, any>;
}

// Plugin Context
export interface PluginContext {
  /** Plugin ID */
  readonly pluginId: string;
  
  /** Installation ID */
  readonly installationId: string;
  
  /** Current user */
  readonly user: User;
  
  /** Current workspace */
  readonly workspace: Workspace;
  
  /** Plugin configuration */
  readonly config: Record<string, any>;
  
  /** PyAirtable API instance */
  readonly api: PyAirtableAPI;
  
  /** Storage interface */
  readonly storage: PluginStorage;
  
  /** Logger instance */
  readonly logger: Logger;
  
  /** Emit events */
  emit(event: string, data?: any): void;
  
  /** Listen to events */
  on(event: string, handler: (data: any) => void): void;
  
  /** Remove event listener */
  off(event: string, handler: (data: any) => void): void;
}

// Execution Context
export interface ExecutionContext {
  /** Request ID for tracing */
  readonly requestId: string;
  
  /** Execution timestamp */
  readonly timestamp: Date;
  
  /** User who triggered the execution */
  readonly user: User;
  
  /** Workspace context */
  readonly workspace: Workspace;
  
  /** Input data */
  readonly input: any;
  
  /** Additional metadata */
  readonly metadata: Record<string, any>;
  
  /** Abort signal for cancellation */
  readonly signal?: AbortSignal;
}

// Plugin Result
export interface PluginResult {
  success: boolean;
  result?: any;
  error?: PluginError;
  metadata?: Record<string, any>;
}

// Plugin Error
export interface PluginError {
  code: string;
  message: string;
  details?: any;
  stack?: string;
}

// Plugin Event
export interface PluginEvent {
  type: string;
  source: string;
  timestamp: Date;
  data: any;
}

// User Interface
export interface User {
  readonly id: string;
  readonly email: string;
  readonly name: string;
  readonly avatar?: string;
  readonly permissions: string[];
}

// Workspace Interface
export interface Workspace {
  readonly id: string;
  readonly name: string;
  readonly plan: string;
  readonly settings: Record<string, any>;
}

// PyAirtable API Interface
export interface PyAirtableAPI {
  readonly records: Records;
  readonly fields: Fields;
  readonly tables: Tables;
  readonly bases: Bases;
  readonly attachments: Attachments;
  readonly comments: Comments;
  readonly webhooks: Webhooks;
  readonly formulas: Formulas;
}

// Records API
export interface Records {
  list(tableId: string, options?: ListRecordsOptions): Promise<Record[]>;
  get(tableId: string, recordId: string): Promise<Record>;
  create(tableId: string, fields: Record<string, any>): Promise<Record>;
  update(tableId: string, recordId: string, fields: Record<string, any>): Promise<Record>;
  delete(tableId: string, recordId: string): Promise<void>;
  batch(tableId: string, operations: BatchOperation[]): Promise<BatchResult>;
}

// Fields API
export interface Fields {
  list(tableId: string): Promise<Field[]>;
  get(tableId: string, fieldId: string): Promise<Field>;
  create(tableId: string, field: CreateFieldRequest): Promise<Field>;
  update(tableId: string, fieldId: string, updates: UpdateFieldRequest): Promise<Field>;
  delete(tableId: string, fieldId: string): Promise<void>;
}

// Tables API
export interface Tables {
  list(baseId: string): Promise<Table[]>;
  get(baseId: string, tableId: string): Promise<Table>;
  create(baseId: string, table: CreateTableRequest): Promise<Table>;
  update(baseId: string, tableId: string, updates: UpdateTableRequest): Promise<Table>;
  delete(baseId: string, tableId: string): Promise<void>;
}

// Bases API
export interface Bases {
  list(): Promise<Base[]>;
  get(baseId: string): Promise<Base>;
  create(base: CreateBaseRequest): Promise<Base>;
  update(baseId: string, updates: UpdateBaseRequest): Promise<Base>;
  delete(baseId: string): Promise<void>;
}

// Attachments API
export interface Attachments {
  upload(file: File | Blob, options?: UploadOptions): Promise<Attachment>;
  get(attachmentId: string): Promise<Attachment>;
  delete(attachmentId: string): Promise<void>;
}

// Comments API
export interface Comments {
  list(recordId: string): Promise<Comment[]>;
  create(recordId: string, text: string): Promise<Comment>;
  update(commentId: string, text: string): Promise<Comment>;
  delete(commentId: string): Promise<void>;
}

// Webhooks API
export interface Webhooks {
  list(): Promise<Webhook[]>;
  create(webhook: CreateWebhookRequest): Promise<Webhook>;
  update(webhookId: string, updates: UpdateWebhookRequest): Promise<Webhook>;
  delete(webhookId: string): Promise<void>;
  test(webhookId: string): Promise<WebhookTestResult>;
}

// Formulas API
export interface Formulas {
  evaluate(formula: string, record: Record): Promise<any>;
  validate(formula: string): Promise<FormulaValidationResult>;
  functions: Record<string, FormulaFunction>;
}

// Data Types
export interface Record {
  id: string;
  fields: Record<string, any>;
  createdTime: string;
  createdBy?: User;
  lastModifiedTime?: string;
  lastModifiedBy?: User;
}

export interface Field {
  id: string;
  name: string;
  type: FieldType;
  description?: string;
  options?: any;
}

export interface Table {
  id: string;
  name: string;
  description?: string;
  fields: Field[];
  views: View[];
}

export interface Base {
  id: string;
  name: string;
  description?: string;
  tables: Table[];
  permissions: Permission[];
}

export interface View {
  id: string;
  name: string;
  type: ViewType;
  configuration: any;
}

export interface Attachment {
  id: string;
  filename: string;
  url: string;
  type: string;
  size: number;
  thumbnails?: Record<string, AttachmentThumbnail>;
}

export interface AttachmentThumbnail {
  url: string;
  width: number;
  height: number;
}

export interface Comment {
  id: string;
  text: string;
  author: User;
  createdTime: string;
  lastModifiedTime?: string;
  mentions?: User[];
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  events: string[];
  filters?: WebhookFilter[];
  isEnabled: boolean;
  createdTime: string;
  lastTriggeredTime?: string;
}

// Request/Response Types
export interface ListRecordsOptions {
  fields?: string[];
  filterByFormula?: string;
  maxRecords?: number;
  pageSize?: number;
  sort?: SortOption[];
  view?: string;
  cellFormat?: 'json' | 'string';
  timeZone?: string;
  userLocale?: string;
}

export interface SortOption {
  field: string;
  direction: 'asc' | 'desc';
}

export interface BatchOperation {
  type: 'create' | 'update' | 'delete';
  recordId?: string;
  fields?: Record<string, any>;
}

export interface BatchResult {
  records: Record[];
  errors?: BatchError[];
}

export interface BatchError {
  type: string;
  message: string;
  recordId?: string;
}

export interface CreateFieldRequest {
  name: string;
  type: FieldType;
  description?: string;
  options?: any;
}

export interface UpdateFieldRequest {
  name?: string;
  description?: string;
  options?: any;
}

export interface CreateTableRequest {
  name: string;
  description?: string;
  fields: CreateFieldRequest[];
}

export interface UpdateTableRequest {
  name?: string;
  description?: string;
}

export interface CreateBaseRequest {
  name: string;
  description?: string;
  tables: CreateTableRequest[];
}

export interface UpdateBaseRequest {
  name?: string;
  description?: string;
}

export interface UploadOptions {
  filename?: string;
  contentType?: string;
}

export interface CreateWebhookRequest {
  name: string;
  url: string;
  events: string[];
  filters?: WebhookFilter[];
}

export interface UpdateWebhookRequest {
  name?: string;
  url?: string;
  events?: string[];
  filters?: WebhookFilter[];
  isEnabled?: boolean;
}

export interface WebhookFilter {
  field: string;
  operator: string;
  value: any;
}

export interface WebhookTestResult {
  success: boolean;
  statusCode?: number;
  response?: any;
  error?: string;
}

export interface FormulaValidationResult {
  isValid: boolean;
  errors?: string[];
  returnType?: string;
}

// Enums
export type FieldType = 
  | 'singleLineText'
  | 'email'
  | 'url'
  | 'multilineText'
  | 'number'
  | 'percent'
  | 'currency'
  | 'singleSelect'
  | 'multipleSelects'
  | 'singleCollaborator'
  | 'multipleCollaborators'
  | 'multipleRecordLinks'
  | 'date'
  | 'dateTime'
  | 'phoneNumber'
  | 'multipleAttachments'
  | 'checkbox'
  | 'formula'
  | 'createdTime'
  | 'rollup'
  | 'count'
  | 'lookup'
  | 'createdBy'
  | 'lastModifiedTime'
  | 'lastModifiedBy'
  | 'button'
  | 'rating'
  | 'richText'
  | 'duration'
  | 'barcode'
  | 'ai';

export type ViewType = 'grid' | 'form' | 'calendar' | 'gallery' | 'kanban' | 'timeline' | 'gantt';

// Plugin Storage Interface
export interface PluginStorage {
  get<T = any>(key: string): Promise<T | null>;
  set<T = any>(key: string, value: T): Promise<void>;
  delete(key: string): Promise<void>;
  clear(): Promise<void>;
  keys(): Promise<string[]>;
  has(key: string): Promise<boolean>;
}

// Logger Interface
export interface Logger {
  debug(message: string, ...args: any[]): void;
  info(message: string, ...args: any[]): void;
  warn(message: string, ...args: any[]): void;
  error(message: string | Error, ...args: any[]): void;
}

// UI Component Types
export interface UIComponent {
  type: string;
  props: Record<string, any>;
  children?: UIComponent[];
}

// Event Hook Types
export interface EventHook {
  event: string;
  priority?: number;
  handler: (data: any, context: ExecutionContext) => Promise<any> | any;
}

// Formula Function Types
export interface FormulaFunction {
  name: string;
  description: string;
  category: string;
  parameters: FormulaParameter[];
  returnType: string;
  examples?: string[];
  handler: (...args: any[]) => any;
}

export interface FormulaParameter {
  name: string;
  type: string;
  description: string;
  required?: boolean;
  defaultValue?: any;
}