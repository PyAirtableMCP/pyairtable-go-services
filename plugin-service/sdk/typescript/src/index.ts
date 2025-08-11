/**
 * PyAirtable Plugin SDK for TypeScript
 * 
 * This SDK provides the core interfaces and utilities for building PyAirtable plugins
 * that run in a secure WebAssembly environment.
 */

export * from './types';
export * from './api';
export * from './hooks';
export * from './ui';
export * from './storage';
export * from './permissions';
export * from './formula';
export * from './utils';

// Re-export core types for convenience
export type {
  Plugin,
  PluginManifest,
  PluginContext,
  ExecutionContext,
  PluginResult,
  PluginError,
  Permission,
  ResourceLimits,
  UIComponent,
  EventHook,
  FormulaFunction,
} from './types';

// Re-export main API classes
export {
  PyAirtableAPI,
  Records,
  Fields,
  Tables,
  Bases,
  Attachments,
  Comments,
  Webhooks,
} from './api';

// Re-export plugin utilities
export {
  createPlugin,
  defineHook,
  defineUIComponent,
  defineFormula,
  withPermissions,
  withResourceLimits,
} from './utils';

// Version information
export const SDK_VERSION = '1.0.0';
export const API_VERSION = '1.0';
export const MIN_RUNTIME_VERSION = '1.0.0';