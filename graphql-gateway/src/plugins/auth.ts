import { ApolloServerPlugin } from '@apollo/server';
import { GraphQLError } from 'graphql';
import { Context, FieldPermission } from '../types';
import { logger, graphqlLogger } from '../utils/logger';

interface AuthDirective {
  requires?: string[];
  roles?: string[];
  permissions?: string[];
  owner?: boolean;
}

export function createAuthPlugin(): ApolloServerPlugin<Context> {
  return {
    requestDidStart() {
      return {
        didResolveOperation(requestContext) {
          const { document, contextValue } = requestContext;
          
          if (!document) return;

          // Check if operation requires authentication
          const requiresAuth = this.operationRequiresAuth(document);
          
          if (requiresAuth && !contextValue.user) {
            graphqlLogger.logAuth('OPERATION_AUTH_REQUIRED', undefined, false);
            throw new GraphQLError('Authentication required', {
              extensions: {
                code: 'UNAUTHENTICATED',
              },
            });
          }

          // Log successful authentication
          if (contextValue.user) {
            graphqlLogger.logAuth('OPERATION_AUTHENTICATED', contextValue.user.id, true);
          }
        },

        willSendResponse(requestContext) {
          // Add user context to response headers for debugging (non-production)
          if (process.env.NODE_ENV !== 'production' && requestContext.contextValue.user) {
            requestContext.response.http.headers = requestContext.response.http.headers || new Map();
            requestContext.response.http.headers.set('X-User-ID', requestContext.contextValue.user.id);
            requestContext.response.http.headers.set('X-User-Role', requestContext.contextValue.user.role);
          }
        },
      };
    },

    operationRequiresAuth(document: any): boolean {
      // Check if any field in the operation has auth directives
      let requiresAuth = false;

      function traverse(node: any): void {
        if (!node) return;

        // Check for auth directives
        if (node.directives) {
          for (const directive of node.directives) {
            if (['auth', 'requireAuth', 'authenticated'].includes(directive.name.value)) {
              requiresAuth = true;
              return;
            }
          }
        }

        // Traverse child nodes
        if (node.selectionSet && node.selectionSet.selections) {
          for (const selection of node.selectionSet.selections) {
            traverse(selection);
          }
        }
      }

      if (document.definitions) {
        for (const definition of document.definitions) {
          if (definition.kind === 'OperationDefinition') {
            traverse(definition);
          }
        }
      }

      return requiresAuth;
    },
  };
}

// Field-level authorization class
export class FieldAuthorization {
  private permissions: Map<string, FieldPermission> = new Map();

  addPermission(fieldPath: string, permission: FieldPermission): void {
    this.permissions.set(fieldPath, permission);
  }

  async checkFieldAccess(
    fieldPath: string,
    context: Context,
    args: any,
    info: any
  ): Promise<boolean> {
    const permission = this.permissions.get(fieldPath);
    
    if (!permission) {
      // No specific permission required, allow access
      return true;
    }

    // Check if user is authenticated when required
    if (!context.user) {
      return false;
    }

    // Check role-based access
    if (permission.roles && permission.roles.length > 0) {
      if (!permission.roles.includes(context.user.role)) {
        graphqlLogger.logAuth('FIELD_ACCESS_DENIED_ROLE', context.user.id, false);
        return false;
      }
    }

    // Check permission-based access
    if (permission.permissions && permission.permissions.length > 0) {
      const hasPermission = permission.permissions.some(perm =>
        context.user!.permissions.includes(perm)
      );
      
      if (!hasPermission) {
        graphqlLogger.logAuth('FIELD_ACCESS_DENIED_PERMISSION', context.user.id, false);
        return false;
      }
    }

    // Check custom condition
    if (permission.condition) {
      const conditionResult = permission.condition(context, args, info);
      if (!conditionResult) {
        graphqlLogger.logAuth('FIELD_ACCESS_DENIED_CONDITION', context.user.id, false);
        return false;
      }
    }

    return true;
  }

  // Create field resolver wrapper with authorization
  authorizeField(originalResolver: any) {
    return async (parent: any, args: any, context: Context, info: any) => {
      const fieldPath = `${info.parentType.name}.${info.fieldName}`;
      
      const hasAccess = await this.checkFieldAccess(fieldPath, context, args, info);
      
      if (!hasAccess) {
        throw new GraphQLError(`Access denied to field ${fieldPath}`, {
          extensions: {
            code: 'FORBIDDEN',
            fieldPath,
          },
        });
      }

      return originalResolver(parent, args, context, info);
    };
  }
}

// Pre-configured field permissions for PyAirtable
export const fieldAuthorization = new FieldAuthorization();

// User-related permissions
fieldAuthorization.addPermission('User.email', {
  field: 'User.email',
  roles: ['admin', 'user'],
  permissions: ['read:user_email'],
  condition: (context, args, info) => {
    // Users can only see their own email or admins can see all
    return context.user?.role === 'admin' || 
           context.user?.id === info.source?.id;
  },
});

fieldAuthorization.addPermission('User.permissions', {
  field: 'User.permissions',
  roles: ['admin'],
  permissions: ['read:user_permissions'],
});

// Workspace-related permissions
fieldAuthorization.addPermission('Workspace.members', {
  field: 'Workspace.members',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['read:workspace_members'],
  condition: (context, args, info) => {
    // Check if user is a member of the workspace
    return true; // This would be implemented based on workspace membership
  },
});

fieldAuthorization.addPermission('Workspace.settings', {
  field: 'Workspace.settings',
  roles: ['admin', 'workspace_admin'],
  permissions: ['read:workspace_settings'],
});

// Airtable-related permissions
fieldAuthorization.addPermission('AirtableBase.records', {
  field: 'AirtableBase.records',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['read:airtable_records'],
  condition: (context, args, info) => {
    // Check if user has access to the specific base
    return true; // This would be implemented based on base permissions
  },
});

fieldAuthorization.addPermission('AirtableRecord.create', {
  field: 'AirtableRecord.create',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['create:airtable_records'],
});

fieldAuthorization.addPermission('AirtableRecord.update', {
  field: 'AirtableRecord.update',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['update:airtable_records'],
});

fieldAuthorization.addPermission('AirtableRecord.delete', {
  field: 'AirtableRecord.delete',
  roles: ['admin', 'workspace_admin'],
  permissions: ['delete:airtable_records'],
});

// File-related permissions
fieldAuthorization.addPermission('File.content', {
  field: 'File.content',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['read:file_content'],
  condition: (context, args, info) => {
    // Check if user has access to the file
    return true; // This would be implemented based on file permissions
  },
});

fieldAuthorization.addPermission('File.upload', {
  field: 'File.upload',
  roles: ['admin', 'workspace_admin', 'user'],
  permissions: ['upload:files'],
});

fieldAuthorization.addPermission('File.delete', {
  field: 'File.delete',
  roles: ['admin', 'workspace_admin'],
  permissions: ['delete:files'],
});

// Notification-related permissions
fieldAuthorization.addPermission('Notification.markAsRead', {
  field: 'Notification.markAsRead',
  roles: ['admin', 'user'],
  permissions: ['update:notifications'],
  condition: (context, args, info) => {
    // Users can only mark their own notifications as read
    return context.user?.id === info.source?.userId;
  },
});

// Permission-related permissions (meta!)
fieldAuthorization.addPermission('Permission.grant', {
  field: 'Permission.grant',
  roles: ['admin', 'workspace_admin'],
  permissions: ['grant:permissions'],
});

fieldAuthorization.addPermission('Permission.revoke', {
  field: 'Permission.revoke',
  roles: ['admin', 'workspace_admin'],
  permissions: ['revoke:permissions'],
});

// AI-related permissions
fieldAuthorization.addPermission('AI.generateContent', {
  field: 'AI.generateContent',
  roles: ['admin', 'workspace_admin', 'premium_user'],
  permissions: ['use:ai_features'],
});

fieldAuthorization.addPermission('AI.analyzeData', {
  field: 'AI.analyzeData',
  roles: ['admin', 'workspace_admin', 'premium_user'],
  permissions: ['use:ai_analytics'],
});

// Utility functions for authorization
export const authUtils = {
  // Check if user has any of the required permissions
  hasAnyPermission(user: any, permissions: string[]): boolean {
    return permissions.some(perm => user.permissions.includes(perm));
  },

  // Check if user has all required permissions
  hasAllPermissions(user: any, permissions: string[]): boolean {
    return permissions.every(perm => user.permissions.includes(perm));
  },

  // Check if user has required role
  hasRole(user: any, roles: string[]): boolean {
    return roles.includes(user.role);
  },

  // Check if user owns the resource
  isOwner(user: any, resource: any): boolean {
    return resource.userId === user.id || resource.ownerId === user.id;
  },

  // Check if user is member of workspace
  isMemberOfWorkspace(user: any, workspaceId: string): boolean {
    // This would typically query the database
    // For now, return true as placeholder
    return true;
  },

  // Generate permission denied error
  createPermissionDeniedError(field: string, reason: string): GraphQLError {
    return new GraphQLError(`Access denied to ${field}: ${reason}`, {
      extensions: {
        code: 'FORBIDDEN',
        field,
        reason,
      },
    });
  },
};

// Authorization middleware for resolvers
export function withAuth(
  roles?: string[],
  permissions?: string[],
  condition?: (context: Context, args: any) => boolean
) {
  return function (target: any, propertyName: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value;

    descriptor.value = async function (parent: any, args: any, context: Context, info: any) {
      // Check authentication
      if (!context.user) {
        throw new GraphQLError('Authentication required', {
          extensions: { code: 'UNAUTHENTICATED' },
        });
      }

      // Check roles
      if (roles && !authUtils.hasRole(context.user, roles)) {
        throw authUtils.createPermissionDeniedError(
          `${target.constructor.name}.${propertyName}`,
          'Insufficient role'
        );
      }

      // Check permissions
      if (permissions && !authUtils.hasAnyPermission(context.user, permissions)) {
        throw authUtils.createPermissionDeniedError(
          `${target.constructor.name}.${propertyName}`,
          'Insufficient permissions'
        );
      }

      // Check custom condition
      if (condition && !condition(context, args)) {
        throw authUtils.createPermissionDeniedError(
          `${target.constructor.name}.${propertyName}`,
          'Custom condition failed'
        );
      }

      return originalMethod.call(this, parent, args, context, info);
    };

    return descriptor;
  };
}