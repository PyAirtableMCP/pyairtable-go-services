/**
 * NextAuth.js Provider for PyAirtable Backend Authentication
 * Integrates with the Go-based auth service on port 8081
 */

import { NextAuthOptions, User, Account, Profile } from 'next-auth';
import { JWT } from 'next-auth/jwt';
import CredentialsProvider from 'next-auth/providers/credentials';
import { AuthService } from '../types/api';
import { HTTPClient, APIClientFactory } from '../client/api-client';

// ============================================================================
// Extended NextAuth Types
// ============================================================================

declare module 'next-auth' {
  interface Session {
    user: {
      id: string;
      email: string;
      name: string;
      role: string;
      tenant_id: string;
      is_active: boolean;
      email_verified: boolean;
    };
    accessToken: string;
    refreshToken: string;
    tokenExpires: number;
  }

  interface User extends AuthService.User {
    accessToken: string;
    refreshToken: string;
    tokenExpires: number;
  }
}

declare module 'next-auth/jwt' {
  interface JWT {
    userId: string;
    email: string;
    name: string;
    role: string;
    tenant_id: string;
    is_active: boolean;
    email_verified: boolean;
    accessToken: string;
    refreshToken: string;
    tokenExpires: number;
  }
}

// ============================================================================
// Auth Service Client
// ============================================================================

class AuthServiceClient {
  private httpClient: HTTPClient;

  constructor() {
    this.httpClient = APIClientFactory.createDefault();
  }

  async login(credentials: AuthService.LoginRequest): Promise<{
    user: AuthService.User;
    tokens: AuthService.TokenResponse;
  }> {
    const response = await this.httpClient.request<AuthService.TokenResponse>('auth', {
      method: 'POST',
      url: '/auth/login',
      data: credentials,
      skipAuth: true,
    });

    if (!response.data) {
      throw new Error('Login failed: No token received');
    }

    const tokens = response.data;

    // Get user profile with the access token
    this.httpClient.setAuthTokens(tokens.access_token, tokens.refresh_token);
    
    const userResponse = await this.httpClient.request<AuthService.User>('auth', {
      method: 'GET',
      url: '/auth/me',
    });

    if (!userResponse.data) {
      throw new Error('Failed to fetch user profile');
    }

    return {
      user: userResponse.data,
      tokens,
    };
  }

  async register(data: AuthService.RegisterRequest): Promise<AuthService.User> {
    const response = await this.httpClient.request<AuthService.User>('auth', {
      method: 'POST',
      url: '/auth/register',
      data,
      skipAuth: true,
    });

    if (!response.data) {
      throw new Error('Registration failed');
    }

    return response.data;
  }

  async refreshToken(refreshToken: string): Promise<AuthService.TokenResponse> {
    const response = await this.httpClient.request<AuthService.TokenResponse>('auth', {
      method: 'POST',
      url: '/auth/refresh',
      data: { refresh_token: refreshToken },
      skipAuth: true,
    });

    if (!response.data) {
      throw new Error('Token refresh failed');
    }

    return response.data;
  }

  async validateToken(token: string): Promise<boolean> {
    try {
      const response = await this.httpClient.request<{ valid: boolean }>('auth', {
        method: 'POST',
        url: '/auth/validate',
        data: { token },
        skipAuth: true,
      });

      return response.data?.valid ?? false;
    } catch {
      return false;
    }
  }

  async logout(token: string): Promise<void> {
    await this.httpClient.request('auth', {
      method: 'POST',
      url: '/auth/logout',
      headers: {
        Authorization: `Bearer ${token}`,
      },
      skipAuth: true,
    });
  }
}

// ============================================================================
// NextAuth Configuration
// ============================================================================

const authServiceClient = new AuthServiceClient();

export const authOptions: NextAuthOptions = {
  providers: [
    CredentialsProvider({
      id: 'credentials',
      name: 'Email and Password',
      credentials: {
        email: {
          label: 'Email',
          type: 'email',
          placeholder: 'your-email@example.com',
        },
        password: {
          label: 'Password',
          type: 'password',
        },
      },
      async authorize(credentials, _req) {
        if (!credentials?.email || !credentials?.password) {
          throw new Error('Email and password required');
        }

        try {
          const { user, tokens } = await authServiceClient.login({
            email: credentials.email,
            password: credentials.password,
          });

          if (!user.is_active) {
            throw new Error('Account is deactivated');
          }

          return {
            ...user,
            name: `${user.first_name} ${user.last_name}`,
            accessToken: tokens.access_token,
            refreshToken: tokens.refresh_token,
            tokenExpires: Date.now() + tokens.expires_in * 1000,
          };
        } catch (error) {
          console.error('Authentication error:', error);
          throw new Error('Invalid credentials');
        }
      },
    }),
  ],

  session: {
    strategy: 'jwt',
    maxAge: 24 * 60 * 60, // 24 hours
    updateAge: 60 * 60, // 1 hour
  },

  jwt: {
    maxAge: 24 * 60 * 60, // 24 hours
  },

  pages: {
    signIn: '/auth/signin',
    signUp: '/auth/signup',
    error: '/auth/error',
  },

  callbacks: {
    async jwt({ token, user, account }: { token: JWT; user?: User; account?: Account | null }) {
      // Initial sign in
      if (account && user) {
        return {
          userId: user.id,
          email: user.email,
          name: `${user.first_name} ${user.last_name}`,
          role: user.role,
          tenant_id: user.tenant_id,
          is_active: user.is_active,
          email_verified: user.email_verified,
          accessToken: user.accessToken,
          refreshToken: user.refreshToken,
          tokenExpires: user.tokenExpires,
        };
      }

      // Return previous token if the access token has not expired yet
      if (Date.now() < token.tokenExpires) {
        return token;
      }

      // Access token has expired, try to update it
      try {
        const refreshedTokens = await authServiceClient.refreshToken(token.refreshToken);
        
        return {
          ...token,
          accessToken: refreshedTokens.access_token,
          refreshToken: refreshedTokens.refresh_token,
          tokenExpires: Date.now() + refreshedTokens.expires_in * 1000,
        };
      } catch (error) {
        console.error('Token refresh failed:', error);
        // Return token with error flag to trigger re-authentication
        return {
          ...token,
          error: 'RefreshAccessTokenError' as const,
        };
      }
    },

    async session({ session, token }: { session: any; token: JWT }) {
      if (token.error === 'RefreshAccessTokenError') {
        // Force re-authentication
        throw new Error('Token refresh failed');
      }

      return {
        ...session,
        user: {
          id: token.userId,
          email: token.email,
          name: token.name,
          role: token.role,
          tenant_id: token.tenant_id,
          is_active: token.is_active,
          email_verified: token.email_verified,
        },
        accessToken: token.accessToken,
        refreshToken: token.refreshToken,
        tokenExpires: token.tokenExpires,
      };
    },

    async redirect({ url, baseUrl }: { url: string; baseUrl: string }) {
      // Allows relative callback URLs
      if (url.startsWith('/')) return `${baseUrl}${url}`;
      // Allows callback URLs on the same origin
      else if (new URL(url).origin === baseUrl) return url;
      return baseUrl;
    },
  },

  events: {
    async signIn({ user, account, profile }: { user: User; account: Account | null; profile?: Profile }) {
      console.log('User signed in:', { userId: user.id, email: user.email });
    },

    async signOut({ token }: { token: JWT }) {
      if (token?.accessToken) {
        try {
          await authServiceClient.logout(token.accessToken);
        } catch (error) {
          console.error('Logout error:', error);
        }
      }
      console.log('User signed out:', { userId: token?.userId });
    },

    async session({ session, token }: { session: any; token: JWT }) {
      // Update HTTP client with current tokens
      const httpClient = APIClientFactory.getInstance();
      httpClient.setAuthTokens(token.accessToken, token.refreshToken);
    },
  },

  debug: process.env.NODE_ENV === 'development',
};

// ============================================================================
// Auth Utilities
// ============================================================================

export interface AuthContext {
  isAuthenticated: boolean;
  user: AuthService.User | null;
  accessToken: string | null;
  refreshToken: string | null;
  login: (credentials: AuthService.LoginRequest) => Promise<void>;
  logout: () => Promise<void>;
  register: (data: AuthService.RegisterRequest) => Promise<void>;
}

/**
 * Hook to get the current user's authentication status
 */
export function useAuth(): AuthContext {
  // This would typically use next-auth's useSession hook
  // Implementation depends on the frontend framework being used
  throw new Error('useAuth hook must be implemented in the frontend application');
}

/**
 * Higher-order component to protect routes
 */
export function withAuth<P extends object>(
  Component: React.ComponentType<P>,
  options?: {
    roles?: string[];
    redirectTo?: string;
  }
) {
  return function AuthenticatedComponent(props: P) {
    // Implementation would check authentication status and user roles
    // This is a placeholder that should be implemented in the frontend app
    throw new Error('withAuth HOC must be implemented in the frontend application');
  };
}

/**
 * Server-side authentication check
 */
export async function getServerAuthSession(req: any, res: any) {
  // This would use NextAuth's getServerSession
  // Implementation depends on the specific Next.js setup
  throw new Error('getServerAuthSession must be implemented in the frontend application');
}

// ============================================================================
// Auth Service Integration
// ============================================================================

export class AuthServiceIntegration {
  private httpClient: HTTPClient;

  constructor() {
    this.httpClient = APIClientFactory.getInstance();
  }

  /**
   * Initialize the auth service integration with session tokens
   */
  initializeWithSession(session: any): void {
    if (session?.accessToken && session?.refreshToken) {
      this.httpClient.setAuthTokens(session.accessToken, session.refreshToken);
    }
  }

  /**
   * Clear authentication tokens
   */
  clearTokens(): void {
    this.httpClient.clearAuthTokens();
  }

  /**
   * Check if the user has the required role
   */
  hasRole(userRole: string, requiredRoles: string[]): boolean {
    return requiredRoles.includes(userRole);
  }

  /**
   * Check if the user belongs to the specified tenant
   */
  belongsToTenant(userTenantId: string, requiredTenantId: string): boolean {
    return userTenantId === requiredTenantId;
  }

  /**
   * Get user permissions for a specific resource
   */
  async getUserPermissions(resourceType: string, resourceId?: string): Promise<string[]> {
    try {
      const response = await this.httpClient.request<{ permissions: string[] }>('auth', {
        method: 'GET',
        url: `/auth/permissions`,
        params: {
          resource_type: resourceType,
          resource_id: resourceId || '',
        },
      });

      return response.data?.permissions || [];
    } catch (error) {
      console.error('Failed to fetch user permissions:', error);
      return [];
    }
  }

  /**
   * Check if the user has a specific permission
   */
  async hasPermission(permission: string, resourceType: string, resourceId?: string): Promise<boolean> {
    const permissions = await this.getUserPermissions(resourceType, resourceId);
    return permissions.includes(permission);
  }
}

export default authOptions;