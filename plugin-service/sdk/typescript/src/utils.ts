/**
 * Utility functions for creating PyAirtable plugins
 */

import {
  Plugin,
  PluginManifest,
  PluginContext,
  ExecutionContext,
  PluginResult,
  PluginError,
  EventHook,
  UIComponent,
  FormulaFunction,
  Permission,
  ResourceLimits,
} from './types';

/**
 * Creates a plugin with the given manifest and implementation
 */
export function createPlugin(manifest: PluginManifest, implementation: Partial<Plugin>): Plugin {
  return {
    manifest,
    ...implementation,
  };
}

/**
 * Defines an event hook for the plugin
 */
export function defineHook(event: string, handler: EventHook['handler'], priority?: number): EventHook {
  return {
    event,
    handler,
    priority,
  };
}

/**
 * Defines a UI component for the plugin
 */
export function defineUIComponent(type: string, props: Record<string, any>, children?: UIComponent[]): UIComponent {
  return {
    type,
    props,
    children,
  };
}

/**
 * Defines a formula function for the plugin
 */
export function defineFormula(
  name: string,
  description: string,
  category: string,
  parameters: FormulaFunction['parameters'],
  returnType: string,
  handler: FormulaFunction['handler'],
  examples?: string[]
): FormulaFunction {
  return {
    name,
    description,
    category,
    parameters,
    returnType,
    handler,
    examples,
  };
}

/**
 * Higher-order function to add permission checks to plugin functions
 */
export function withPermissions<T extends (...args: any[]) => any>(
  permissions: Permission[],
  func: T
): T {
  return ((...args: any[]) => {
    // In a real implementation, this would check permissions
    // against the current execution context
    return func(...args);
  }) as T;
}

/**
 * Higher-order function to add resource limit checks to plugin functions
 */
export function withResourceLimits<T extends (...args: any[]) => any>(
  limits: ResourceLimits,
  func: T
): T {
  return ((...args: any[]) => {
    // In a real implementation, this would enforce resource limits
    // during function execution
    return func(...args);
  }) as T;
}

/**
 * Creates a standardized plugin error
 */
export function createPluginError(
  code: string,
  message: string,
  details?: any,
  stack?: string
): PluginError {
  return {
    code,
    message,
    details,
    stack: stack || new Error().stack,
  };
}

/**
 * Creates a successful plugin result
 */
export function createSuccessResult(result: any, metadata?: Record<string, any>): PluginResult {
  return {
    success: true,
    result,
    metadata,
  };
}

/**
 * Creates a failed plugin result
 */
export function createErrorResult(error: PluginError, metadata?: Record<string, any>): PluginResult {
  return {
    success: false,
    error,
    metadata,
  };
}

/**
 * Validates a plugin manifest
 */
export function validateManifest(manifest: PluginManifest): { valid: boolean; errors: string[] } {
  const errors: string[] = [];

  if (!manifest.name || typeof manifest.name !== 'string') {
    errors.push('Plugin name is required and must be a string');
  }

  if (!manifest.version || typeof manifest.version !== 'string') {
    errors.push('Plugin version is required and must be a string');
  }

  if (!manifest.type || !['formula', 'ui', 'webhook', 'automation', 'connector', 'view'].includes(manifest.type)) {
    errors.push('Plugin type is required and must be one of: formula, ui, webhook, automation, connector, view');
  }

  if (!manifest.description || typeof manifest.description !== 'string') {
    errors.push('Plugin description is required and must be a string');
  }

  if (!manifest.author || typeof manifest.author !== 'string') {
    errors.push('Plugin author is required and must be a string');
  }

  if (!manifest.entryPoint || typeof manifest.entryPoint !== 'string') {
    errors.push('Plugin entryPoint is required and must be a string');
  }

  // Validate semantic version format
  if (manifest.version && !isValidSemVer(manifest.version)) {
    errors.push('Plugin version must follow semantic versioning (e.g., 1.0.0)');
  }

  // Validate permissions
  if (manifest.permissions) {
    for (const permission of manifest.permissions) {
      if (!permission.resource || typeof permission.resource !== 'string') {
        errors.push('Permission resource is required and must be a string');
      }
      if (!Array.isArray(permission.actions) || permission.actions.length === 0) {
        errors.push('Permission actions must be a non-empty array');
      }
    }
  }

  // Validate resource limits
  if (manifest.resourceLimits) {
    const limits = manifest.resourceLimits;
    if (typeof limits.maxMemoryMB !== 'number' || limits.maxMemoryMB <= 0) {
      errors.push('Resource limit maxMemoryMB must be a positive number');
    }
    if (typeof limits.maxCPUPercent !== 'number' || limits.maxCPUPercent <= 0 || limits.maxCPUPercent > 100) {
      errors.push('Resource limit maxCPUPercent must be a number between 1 and 100');
    }
    if (typeof limits.maxExecutionMs !== 'number' || limits.maxExecutionMs <= 0) {
      errors.push('Resource limit maxExecutionMs must be a positive number');
    }
  }

  return {
    valid: errors.length === 0,
    errors,
  };
}

/**
 * Checks if a version string follows semantic versioning
 */
export function isValidSemVer(version: string): boolean {
  const semVerRegex = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;
  return semVerRegex.test(version);
}

/**
 * Compares two semantic version strings
 * Returns: -1 if a < b, 0 if a === b, 1 if a > b
 */
export function compareSemVer(a: string, b: string): number {
  const normalize = (v: string) => {
    const parts = v.split('.');
    return parts.map(part => parseInt(part, 10));
  };

  const versionA = normalize(a);
  const versionB = normalize(b);

  for (let i = 0; i < Math.max(versionA.length, versionB.length); i++) {
    const partA = versionA[i] || 0;
    const partB = versionB[i] || 0;

    if (partA < partB) return -1;
    if (partA > partB) return 1;
  }

  return 0;
}

/**
 * Checks if a version satisfies a version range
 */
export function satisfiesVersion(version: string, range: string): boolean {
  // Simple implementation - in reality you'd use a more sophisticated library
  if (range.startsWith('>=')) {
    const targetVersion = range.substring(2);
    return compareSemVer(version, targetVersion) >= 0;
  }
  
  if (range.startsWith('<=')) {
    const targetVersion = range.substring(2);
    return compareSemVer(version, targetVersion) <= 0;
  }
  
  if (range.startsWith('>')) {
    const targetVersion = range.substring(1);
    return compareSemVer(version, targetVersion) > 0;
  }
  
  if (range.startsWith('<')) {
    const targetVersion = range.substring(1);
    return compareSemVer(version, targetVersion) < 0;
  }
  
  if (range.startsWith('^')) {
    const targetVersion = range.substring(1);
    const [major] = targetVersion.split('.');
    const [vMajor] = version.split('.');
    return vMajor === major && compareSemVer(version, targetVersion) >= 0;
  }
  
  if (range.startsWith('~')) {
    const targetVersion = range.substring(1);
    const [major, minor] = targetVersion.split('.');
    const [vMajor, vMinor] = version.split('.');
    return vMajor === major && vMinor === minor && compareSemVer(version, targetVersion) >= 0;
  }
  
  return version === range;
}

/**
 * Debounces a function call
 */
export function debounce<T extends (...args: any[]) => any>(
  func: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timeoutId: NodeJS.Timeout;
  
  return (...args: Parameters<T>) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => func(...args), delay);
  };
}

/**
 * Throttles a function call
 */
export function throttle<T extends (...args: any[]) => any>(
  func: T,
  delay: number
): (...args: Parameters<T>) => void {
  let lastCall = 0;
  
  return (...args: Parameters<T>) => {
    const now = Date.now();
    
    if (now - lastCall >= delay) {
      lastCall = now;
      func(...args);
    }
  };
}

/**
 * Creates a promise that resolves after a specified delay
 */
export function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Creates a promise that rejects after a specified timeout
 */
export function timeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    promise,
    new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error(`Operation timed out after ${ms}ms`)), ms)
    ),
  ]);
}

/**
 * Retries a function with exponential backoff
 */
export async function retry<T>(
  func: () => Promise<T>,
  maxAttempts: number = 3,
  baseDelay: number = 1000
): Promise<T> {
  let lastError: Error;

  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      return await func();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));
      
      if (attempt === maxAttempts) {
        throw lastError;
      }
      
      const delayMs = baseDelay * Math.pow(2, attempt - 1);
      await delay(delayMs);
    }
  }

  throw lastError!;
}

/**
 * Deep clones an object
 */
export function deepClone<T>(obj: T): T {
  if (obj === null || typeof obj !== 'object') {
    return obj;
  }

  if (obj instanceof Date) {
    return new Date(obj.getTime()) as unknown as T;
  }

  if (obj instanceof Array) {
    return obj.map(item => deepClone(item)) as unknown as T;
  }

  if (obj instanceof Object) {
    const cloned = {} as T;
    for (const key in obj) {
      if (obj.hasOwnProperty(key)) {
        cloned[key] = deepClone(obj[key]);
      }
    }
    return cloned;
  }

  return obj;
}

/**
 * Merges objects deeply
 */
export function deepMerge<T extends Record<string, any>>(target: T, ...sources: Partial<T>[]): T {
  if (!sources.length) return target;
  
  const source = sources.shift();
  if (!source) return target;

  for (const key in source) {
    if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
      if (!target[key] || typeof target[key] !== 'object') {
        target[key] = {} as any;
      }
      deepMerge(target[key], source[key]);
    } else {
      target[key] = source[key] as any;
    }
  }

  return deepMerge(target, ...sources);
}

/**
 * Validates an email address
 */
export function isValidEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(email);
}

/**
 * Validates a URL
 */
export function isValidURL(url: string): boolean {
  try {
    new URL(url);
    return true;
  } catch {
    return false;
  }
}

/**
 * Sanitizes HTML content
 */
export function sanitizeHTML(html: string): string {
  // Basic HTML sanitization - in a real implementation you'd use a proper library
  return html
    .replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
    .replace(/<iframe\b[^<]*(?:(?!<\/iframe>)<[^<]*)*<\/iframe>/gi, '')
    .replace(/javascript:/gi, '')
    .replace(/on\w+\s*=/gi, '');
}

/**
 * Escapes HTML entities
 */
export function escapeHTML(text: string): string {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

/**
 * Formats a file size in bytes to human readable format
 */
export function formatFileSize(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unitIndex = 0;

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }

  return `${size.toFixed(1)} ${units[unitIndex]}`;
}

/**
 * Formats a duration in milliseconds to human readable format
 */
export function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms}ms`;
  }
  
  if (ms < 60000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  
  const minutes = Math.floor(ms / 60000);
  const seconds = Math.floor((ms % 60000) / 1000);
  
  return `${minutes}m ${seconds}s`;
}

/**
 * Generates a random UUID
 */
export function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}

/**
 * Type guard to check if a value is defined
 */
export function isDefined<T>(value: T | undefined | null): value is T {
  return value !== undefined && value !== null;
}

/**
 * Type guard to check if a value is a string
 */
export function isString(value: any): value is string {
  return typeof value === 'string';
}

/**
 * Type guard to check if a value is a number
 */
export function isNumber(value: any): value is number {
  return typeof value === 'number' && !isNaN(value);
}

/**
 * Type guard to check if a value is a boolean
 */
export function isBoolean(value: any): value is boolean {
  return typeof value === 'boolean';
}

/**
 * Type guard to check if a value is an object
 */
export function isObject(value: any): value is Record<string, any> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

/**
 * Type guard to check if a value is an array
 */
export function isArray(value: any): value is any[] {
  return Array.isArray(value);
}