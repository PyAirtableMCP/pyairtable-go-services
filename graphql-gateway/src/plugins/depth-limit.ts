import { ApolloServerPlugin } from '@apollo/server';
import { GraphQLError } from 'graphql';
import depthLimit from 'graphql-depth-limit';
import { Context } from '../types';
import { logger, graphqlLogger } from '../utils/logger';
import { config } from '../config/config';

export function createDepthLimitPlugin(): ApolloServerPlugin<Context> {
  const maxDepth = config.rateLimit.maxDepth;

  return {
    requestDidStart() {
      return {
        didResolveOperation(requestContext) {
          const { document, schema } = requestContext;
          
          if (!document || !schema) return;

          try {
            // Use graphql-depth-limit to validate query depth
            const depthLimitRule = depthLimit(maxDepth, {
              ignore: [
                // Ignore introspection queries
                '__schema',
                '__type',
                '__inputFields',
                '__fields',
                '__possibleTypes',
                '__enumValues',
                '__directives',
              ],
              callback: (args: any) => {
                const { depth, query } = args;
                
                // Log queries approaching the limit
                if (depth > maxDepth * 0.8) {
                  graphqlLogger.logComplexity(
                    depth,
                    maxDepth,
                    query || ''
                  );
                }

                // Throw error if depth exceeds limit
                if (depth > maxDepth) {
                  throw new GraphQLError(
                    `Query depth limit of ${maxDepth} exceeded, found ${depth}`,
                    {
                      extensions: {
                        code: 'QUERY_DEPTH_TOO_HIGH',
                        depth,
                        maxDepth,
                      },
                    }
                  );
                }
              },
            });

            // Validate the document against the depth limit rule
            const errors = depthLimitRule(requestContext);
            if (errors && errors.length > 0) {
              throw errors[0];
            }
          } catch (error) {
            logger.error('Depth limit validation error', { error });
            throw error;
          }
        },
      };
    },
  };
}

// Utility function to calculate query depth manually
export function calculateQueryDepth(query: any, maxDepth: number = 15): number {
  let depth = 0;

  function traverse(node: any, currentDepth: number = 0): void {
    if (!node || currentDepth > maxDepth) {
      return;
    }

    depth = Math.max(depth, currentDepth);

    if (node.selectionSet && node.selectionSet.selections) {
      for (const selection of node.selectionSet.selections) {
        if (selection.kind === 'Field') {
          // Skip introspection fields
          if (selection.name.value.startsWith('__')) {
            continue;
          }
          traverse(selection, currentDepth + 1);
        } else if (selection.kind === 'InlineFragment' || selection.kind === 'FragmentSpread') {
          traverse(selection, currentDepth);
        }
      }
    }
  }

  if (query.definitions) {
    for (const definition of query.definitions) {
      if (definition.kind === 'OperationDefinition') {
        traverse(definition, 0);
      }
    }
  }

  return depth;
}

// Advanced depth analysis for specific patterns
export class DepthAnalyzer {
  static analyzeQueryPattern(query: any): {
    maxDepth: number;
    avgDepth: number;
    deepestPath: string[];
    circularReferences: boolean;
  } {
    const paths: string[][] = [];
    const visitedNodes = new Set();
    let circularReferences = false;

    function traverse(
      node: any,
      currentPath: string[] = [],
      currentDepth: number = 0
    ): void {
      if (!node || currentDepth > 20) { // Prevent infinite recursion
        return;
      }

      // Check for circular references
      const nodeKey = `${node.kind}-${node.name?.value || 'anonymous'}-${currentDepth}`;
      if (visitedNodes.has(nodeKey)) {
        circularReferences = true;
        return;
      }
      visitedNodes.add(nodeKey);

      if (node.selectionSet && node.selectionSet.selections) {
        for (const selection of node.selectionSet.selections) {
          if (selection.kind === 'Field' && !selection.name.value.startsWith('__')) {
            const newPath = [...currentPath, selection.name.value];
            paths.push(newPath);
            traverse(selection, newPath, currentDepth + 1);
          } else if (selection.kind === 'InlineFragment' || selection.kind === 'FragmentSpread') {
            traverse(selection, currentPath, currentDepth);
          }
        }
      }
    }

    if (query.definitions) {
      for (const definition of query.definitions) {
        if (definition.kind === 'OperationDefinition') {
          traverse(definition);
        }
      }
    }

    const depths = paths.map(path => path.length);
    const maxDepth = Math.max(...depths, 0);
    const avgDepth = depths.length > 0 ? depths.reduce((a, b) => a + b, 0) / depths.length : 0;
    const deepestPath = paths.find(path => path.length === maxDepth) || [];

    return {
      maxDepth,
      avgDepth: Math.round(avgDepth * 100) / 100,
      deepestPath,
      circularReferences,
    };
  }

  static suggestOptimizations(analysis: ReturnType<typeof DepthAnalyzer.analyzeQueryPattern>): string[] {
    const suggestions: string[] = [];

    if (analysis.maxDepth > 8) {
      suggestions.push(
        'Consider breaking down deep queries into multiple smaller queries for better performance'
      );
    }

    if (analysis.circularReferences) {
      suggestions.push(
        'Detected potential circular references. Use fragments or pagination to optimize'
      );
    }

    if (analysis.avgDepth > 5) {
      suggestions.push(
        'Average query depth is high. Consider using DataLoader to batch related queries'
      );
    }

    if (analysis.deepestPath.length > 0) {
      const pathStr = analysis.deepestPath.join(' -> ');
      suggestions.push(
        `Deepest query path: ${pathStr}. Consider if all nested data is necessary`
      );
    }

    return suggestions;
  }
}

// Custom depth limit rules for specific types
export const customDepthLimits = {
  // More restrictive limits for expensive operations
  airtableRecords: 5,
  workspaceHierarchy: 6,
  userPermissions: 4,
  fileMetadata: 3,
  
  // More lenient limits for simple data
  notifications: 8,
  userProfile: 7,
  workspace: 6,
};

// Depth limit configuration based on user role
export function getDepthLimitForUser(userRole: string): number {
  const baseLimits = {
    admin: 15,
    premium: 12,
    standard: 10,
    free: 8,
  };

  return baseLimits[userRole as keyof typeof baseLimits] || config.rateLimit.maxDepth;
}

// Dynamic depth limit based on query complexity
export function calculateDynamicDepthLimit(
  baseComplexity: number,
  maxComplexity: number
): number {
  const complexityRatio = baseComplexity / maxComplexity;
  const baseDepthLimit = config.rateLimit.maxDepth;
  
  // Reduce depth limit as complexity increases
  if (complexityRatio > 0.8) {
    return Math.max(5, Math.floor(baseDepthLimit * 0.6));
  } else if (complexityRatio > 0.5) {
    return Math.max(6, Math.floor(baseDepthLimit * 0.8));
  }
  
  return baseDepthLimit;
}