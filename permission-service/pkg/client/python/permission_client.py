"""
Python gRPC client for Permission Service with async support, connection pooling, and retry logic.
"""

import asyncio
import logging
import random
import time
from typing import Dict, List, Optional, Union
from dataclasses import dataclass
from enum import Enum

import grpc
from grpc import aio as aio_grpc
import grpc.experimental.gevent as grpc_gevent

# These would be the generated protobuf classes
# from pyairtable.permission.v1 import permission_pb2, permission_pb2_grpc
# from pyairtable.common.v1 import common_pb2


class CircuitState(Enum):
    CLOSED = "closed"
    OPEN = "open"
    HALF_OPEN = "half_open"


@dataclass
class ClientConfig:
    """Configuration for the Permission gRPC client."""
    address: str
    max_retries: int = 3
    retry_interval: float = 1.0
    connection_timeout: float = 10.0
    keepalive_time: int = 30
    keepalive_timeout: int = 5
    keepalive_permit_without_calls: bool = True
    max_receive_message_length: int = 4 * 1024 * 1024  # 4MB
    max_send_message_length: int = 4 * 1024 * 1024     # 4MB
    enable_compression: bool = True
    enable_retry: bool = True
    pool_size: int = 5


class CircuitBreaker:
    """Simple circuit breaker implementation."""
    
    def __init__(self, max_failures: int = 5, timeout: float = 30.0):
        self.max_failures = max_failures
        self.timeout = timeout
        self.state = CircuitState.CLOSED
        self.failures = 0
        self.last_fail_time = 0.0
        self._lock = asyncio.Lock()
    
    async def call(self, func, *args, **kwargs):
        """Execute function through circuit breaker."""
        async with self._lock:
            if self.state == CircuitState.OPEN:
                if time.time() - self.last_fail_time > self.timeout:
                    self.state = CircuitState.HALF_OPEN
                else:
                    raise Exception("Circuit breaker is open")
        
        try:
            result = await func(*args, **kwargs)
            async with self._lock:
                self.failures = 0
                self.state = CircuitState.CLOSED
            return result
        except Exception as e:
            async with self._lock:
                self.failures += 1
                self.last_fail_time = time.time()
                if self.failures >= self.max_failures:
                    self.state = CircuitState.OPEN
            raise e


class ConnectionPool:
    """Async connection pool for gRPC channels."""
    
    def __init__(self, config: ClientConfig, logger: logging.Logger):
        self.config = config
        self.logger = logger
        self.channels: List[aio_grpc.Channel] = []
        self.current_index = 0
        self._lock = asyncio.Lock()
    
    async def initialize(self):
        """Initialize the connection pool."""
        for i in range(self.config.pool_size):
            channel = await self._create_channel()
            self.channels.append(channel)
        
        self.logger.info(f"gRPC connection pool initialized with {len(self.channels)} connections")
    
    async def _create_channel(self) -> aio_grpc.Channel:
        """Create a new gRPC channel."""
        options = [
            ('grpc.keepalive_time_ms', self.config.keepalive_time * 1000),
            ('grpc.keepalive_timeout_ms', self.config.keepalive_timeout * 1000),
            ('grpc.keepalive_permit_without_calls', self.config.keepalive_permit_without_calls),
            ('grpc.http2.max_pings_without_data', 0),
            ('grpc.http2.min_time_between_pings_ms', 10 * 1000),
            ('grpc.http2.min_ping_interval_without_data_ms', 5 * 1000),
            ('grpc.max_receive_message_length', self.config.max_receive_message_length),
            ('grpc.max_send_message_length', self.config.max_send_message_length),
        ]
        
        if self.config.enable_compression:
            options.append(('grpc.default_compression_algorithm', grpc.Compression.Gzip))
        
        channel = aio_grpc.insecure_channel(self.config.address, options=options)
        
        # Wait for channel to be ready
        try:
            await asyncio.wait_for(
                channel.channel_ready(),
                timeout=self.config.connection_timeout
            )
        except asyncio.TimeoutError:
            await channel.close()
            raise Exception(f"Failed to connect to {self.config.address} within {self.config.connection_timeout}s")
        
        return channel
    
    async def get_channel(self) -> aio_grpc.Channel:
        """Get a channel from the pool using round-robin."""
        async with self._lock:
            channel = self.channels[self.current_index]
            self.current_index = (self.current_index + 1) % len(self.channels)
        
        # Check if channel is healthy
        try:
            state = channel.get_state()
            if state in [grpc.ChannelConnectivity.TRANSIENT_FAILURE, grpc.ChannelConnectivity.SHUTDOWN]:
                self.logger.warning(f"Channel is unhealthy (state: {state}), attempting to reconnect")
                # Try to create a new channel
                try:
                    new_channel = await self._create_channel()
                    async with self._lock:
                        old_channel = self.channels[self.current_index]
                        self.channels[self.current_index] = new_channel
                        # Close old channel in background
                        asyncio.create_task(old_channel.close())
                    channel = new_channel
                except Exception as e:
                    self.logger.error(f"Failed to reconnect channel: {e}")
        except Exception as e:
            self.logger.error(f"Error checking channel state: {e}")
        
        return channel
    
    async def close(self):
        """Close all channels in the pool."""
        close_tasks = [channel.close() for channel in self.channels]
        await asyncio.gather(*close_tasks, return_exceptions=True)
        self.logger.info("gRPC connection pool closed")


class PermissionClient:
    """Async gRPC client for Permission Service."""
    
    def __init__(self, config: ClientConfig, logger: Optional[logging.Logger] = None):
        self.config = config
        self.logger = logger or logging.getLogger(__name__)
        self.pool = ConnectionPool(config, self.logger)
        self.circuit_breaker = CircuitBreaker()
    
    async def initialize(self):
        """Initialize the client."""
        await self.pool.initialize()
    
    async def close(self):
        """Close the client and release resources."""
        await self.pool.close()
    
    async def _with_retry(self, func, *args, **kwargs):
        """Execute function with retry logic and circuit breaker."""
        return await self.circuit_breaker.call(self._retry_wrapper, func, *args, **kwargs)
    
    async def _retry_wrapper(self, func, *args, **kwargs):
        """Wrapper that implements retry logic."""
        last_exception = None
        
        for attempt in range(self.config.max_retries + 1):
            try:
                channel = await self.pool.get_channel()
                stub = permission_pb2_grpc.PermissionServiceStub(channel)
                return await func(stub, *args, **kwargs)
            except Exception as e:
                last_exception = e
                
                if not self._is_retryable_error(e):
                    raise e
                
                if attempt < self.config.max_retries:
                    self.logger.warning(
                        f"Request failed (attempt {attempt + 1}/{self.config.max_retries + 1}): {e}"
                    )
                    await asyncio.sleep(self.config.retry_interval * (2 ** attempt) + random.uniform(0, 1))
        
        raise last_exception
    
    def _is_retryable_error(self, error: Exception) -> bool:
        """Determine if an error is retryable."""
        if isinstance(error, grpc.RpcError):
            code = error.code()
            return code in [
                grpc.StatusCode.UNAVAILABLE,
                grpc.StatusCode.DEADLINE_EXCEEDED,
                grpc.StatusCode.RESOURCE_EXHAUSTED,
                grpc.StatusCode.ABORTED
            ]
        return False
    
    # Permission Service Methods
    
    async def check_permission(self, request) -> object:
        """Check if a user has permission for a specific action."""
        async def _call(stub, req):
            return await stub.CheckPermission(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def batch_check_permission(self, request) -> object:
        """Check multiple permissions in a single request."""
        async def _call(stub, req):
            return await stub.BatchCheckPermission(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def grant_permission(self, request) -> object:
        """Grant a permission to a user."""
        async def _call(stub, req):
            return await stub.GrantPermission(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def revoke_permission(self, request) -> object:
        """Revoke a permission from a user."""
        async def _call(stub, req):
            return await stub.RevokePermission(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def list_permissions(self, request) -> object:
        """List permissions with optional filtering."""
        async def _call(stub, req):
            return await stub.ListPermissions(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def create_role(self, request) -> object:
        """Create a new role."""
        async def _call(stub, req):
            return await stub.CreateRole(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def assign_role(self, request) -> object:
        """Assign a role to a user."""
        async def _call(stub, req):
            return await stub.AssignRole(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def list_user_roles(self, request) -> object:
        """List roles assigned to a user."""
        async def _call(stub, req):
            return await stub.ListUserRoles(req, timeout=30.0)
        
        return await self._with_retry(_call, request)
    
    async def health_check(self) -> object:
        """Perform a health check."""
        async def _call(stub):
            # return await stub.HealthCheck(common_pb2.Empty(), timeout=10.0)
            pass  # Placeholder until protobuf is properly imported
        
        return await self._with_retry(_call)
    
    async def get_stats(self) -> Dict:
        """Get client statistics."""
        return {
            "pool_size": len(self.pool.channels),
            "address": self.config.address,
            "circuit_breaker_state": self.circuit_breaker.state.value,
            "circuit_breaker_failures": self.circuit_breaker.failures,
        }


# Context manager for easy client usage
class PermissionClientContext:
    """Context manager for Permission client."""
    
    def __init__(self, config: ClientConfig, logger: Optional[logging.Logger] = None):
        self.client = PermissionClient(config, logger)
    
    async def __aenter__(self):
        await self.client.initialize()
        return self.client
    
    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.client.close()


# Helper functions
def create_client_config(address: str, **kwargs) -> ClientConfig:
    """Create a client configuration with defaults."""
    return ClientConfig(address=address, **kwargs)


async def create_permission_client(address: str, **kwargs) -> PermissionClient:
    """Create and initialize a Permission client."""
    config = create_client_config(address, **kwargs)
    client = PermissionClient(config)
    await client.initialize()
    return client


# Example usage
async def main():
    """Example usage of the Permission client."""
    config = create_client_config("localhost:50051")
    
    async with PermissionClientContext(config) as client:
        # Example health check
        try:
            await client.health_check()
            print("Health check successful")
        except Exception as e:
            print(f"Health check failed: {e}")
        
        # Get client statistics
        stats = await client.get_stats()
        print(f"Client stats: {stats}")


if __name__ == "__main__":
    # Configure logging
    logging.basicConfig(level=logging.INFO)
    
    # Run example
    asyncio.run(main())