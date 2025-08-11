/**
 * Complete Integration Example for PyAirtable Frontend
 * Demonstrates all features: authentication, caching, real-time, optimistic updates
 */

import React, { useState, useEffect, useCallback } from 'react';
import { useSession, signIn, signOut } from 'next-auth/react';
import {
  PyAirtableProvider,
  usePyAirtable,
  useRealtime,
  useOptimisticUpdate,
  RecordService,
  WorkspaceService,
  ServiceError,
} from '@pyairtable/frontend-integration';

// ============================================================================
// App Root with Provider
// ============================================================================

export function App() {
  return (
    <PyAirtableProvider 
      config={{
        baseURL: process.env.NEXT_PUBLIC_API_URL,
        features: {
          enableRealtime: true,
          enableOptimisticUpdates: true,
          enableOfflineSupport: true,
        },
      }}
    >
      <AppContent />
    </PyAirtableProvider>
  );
}

// ============================================================================
// Main App Content
// ============================================================================

function AppContent() {
  const { data: session, status } = useSession();
  const pyairtable = usePyAirtable();

  useEffect(() => {
    if (session) {
      pyairtable.initialize(session);
    }
  }, [session, pyairtable]);

  if (status === 'loading') {
    return <LoadingSpinner />;
  }

  if (!session) {
    return <LoginPage />;
  }

  return (
    <div className="app">
      <Header />
      <main>
        <WorkspaceDashboard />
      </main>
      <ConnectionStatus />
    </div>
  );
}

// ============================================================================
// Authentication Components
// ============================================================================

function LoginPage() {
  const [credentials, setCredentials] = useState({ email: '', password: '' });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const result = await signIn('credentials', {
        email: credentials.email,
        password: credentials.password,
        redirect: false,
      });

      if (result?.error) {
        setError('Invalid credentials');
      }
    } catch (err) {
      setError('Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <form onSubmit={handleLogin} className="login-form">
        <h1>Sign In to PyAirtable</h1>
        
        {error && <div className="error">{error}</div>}
        
        <input
          type="email"
          placeholder="Email"
          value={credentials.email}
          onChange={(e) => setCredentials(prev => ({ ...prev, email: e.target.value }))}
          required
        />
        
        <input
          type="password"
          placeholder="Password"
          value={credentials.password}
          onChange={(e) => setCredentials(prev => ({ ...prev, password: e.target.value }))}
          required
        />
        
        <button type="submit" disabled={loading}>
          {loading ? 'Signing In...' : 'Sign In'}
        </button>
      </form>
    </div>
  );
}

function Header() {
  const { data: session } = useSession();

  return (
    <header className="header">
      <div className="header-content">
        <h1>PyAirtable</h1>
        <div className="user-menu">
          <span>Welcome, {session?.user?.name}</span>
          <button onClick={() => signOut()}>Sign Out</button>
        </div>
      </div>
    </header>
  );
}

// ============================================================================
// Workspace Dashboard with Real-time Updates
// ============================================================================

function WorkspaceDashboard() {
  const pyairtable = usePyAirtable();
  const [workspaces, setWorkspaces] = useState<WorkspaceService.Workspace[]>([]);
  const [selectedWorkspace, setSelectedWorkspace] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load workspaces with caching
  const loadWorkspaces = useCallback(async () => {
    try {
      setLoading(true);
      const response = await pyairtable.services.workspace.getWorkspaces();
      setWorkspaces(response.data);
      setError(null);
    } catch (err) {
      if (err instanceof ServiceError) {
        setError(err.message);
      } else {
        setError('Failed to load workspaces');
      }
    } finally {
      setLoading(false);
    }
  }, [pyairtable]);

  useEffect(() => {
    loadWorkspaces();
  }, [loadWorkspaces]);

  // Real-time workspace updates
  useRealtime(
    { channel: 'workspace', enabled: true },
    (event) => {
      switch (event.type) {
        case 'workspace.created':
          setWorkspaces(prev => [...prev, event.payload.workspace]);
          break;
        case 'workspace.updated':
          setWorkspaces(prev => 
            prev.map(ws => 
              ws.id === event.payload.workspace.id 
                ? event.payload.workspace 
                : ws
            )
          );
          break;
        case 'workspace.member_added':
          // Update member count
          setWorkspaces(prev =>
            prev.map(ws =>
              ws.id === event.payload.workspace_id
                ? { ...ws, member_count: ws.member_count + 1 }
                : ws
            )
          );
          break;
      }
    }
  );

  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage message={error} onRetry={loadWorkspaces} />;

  return (
    <div className="dashboard">
      <div className="sidebar">
        <WorkspaceList 
          workspaces={workspaces}
          selectedId={selectedWorkspace}
          onSelect={setSelectedWorkspace}
          onUpdate={setWorkspaces}
        />
        <CreateWorkspaceForm onCreated={(workspace) => {
          setWorkspaces(prev => [...prev, workspace]);
        }} />
      </div>
      
      <div className="main-content">
        {selectedWorkspace ? (
          <WorkspaceDetails workspaceId={selectedWorkspace} />
        ) : (
          <div className="empty-state">
            <h2>Select a workspace to get started</h2>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================================
// Workspace Components with Optimistic Updates
// ============================================================================

function WorkspaceList({ 
  workspaces, 
  selectedId, 
  onSelect, 
  onUpdate 
}: {
  workspaces: WorkspaceService.Workspace[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onUpdate: (workspaces: WorkspaceService.Workspace[]) => void;
}) {
  return (
    <div className="workspace-list">
      <h2>Workspaces</h2>
      {workspaces.map(workspace => (
        <WorkspaceItem
          key={workspace.id}
          workspace={workspace}
          isSelected={workspace.id === selectedId}
          onSelect={() => onSelect(workspace.id)}
          onUpdate={(updated) => {
            onUpdate(workspaces.map(ws => 
              ws.id === updated.id ? updated : ws
            ));
          }}
        />
      ))}
    </div>
  );
}

function WorkspaceItem({
  workspace,
  isSelected,
  onSelect,
  onUpdate,
}: {
  workspace: WorkspaceService.Workspace;
  isSelected: boolean;
  onSelect: () => void;
  onUpdate: (workspace: WorkspaceService.Workspace) => void;
}) {
  const pyairtable = usePyAirtable();
  const [isEditing, setIsEditing] = useState(false);
  const [name, setName] = useState(workspace.name);

  const { mutate, isOptimistic, error } = useOptimisticUpdate(
    async () => {
      const updated = await pyairtable.services.workspace.updateWorkspace(
        workspace.id,
        { name }
      );
      onUpdate(updated);
      setIsEditing(false);
      return updated;
    }
  );

  const handleUpdate = async () => {
    if (name.trim() && name !== workspace.name) {
      // Apply optimistic update
      onUpdate({ ...workspace, name: name.trim() });
      
      try {
        await mutate();
      } catch (err) {
        // Rollback on error
        onUpdate(workspace);
        setName(workspace.name);
      }
    } else {
      setIsEditing(false);
      setName(workspace.name);
    }
  };

  return (
    <div className={`workspace-item ${isSelected ? 'selected' : ''}`}>
      <div onClick={onSelect} className="workspace-info">
        {isEditing ? (
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={handleUpdate}
            onKeyPress={(e) => {
              if (e.key === 'Enter') {
                handleUpdate();
              } else if (e.key === 'Escape') {
                setIsEditing(false);
                setName(workspace.name);
              }
            }}
            autoFocus
            className={isOptimistic ? 'optimistic' : ''}
          />
        ) : (
          <h3 className={isOptimistic ? 'optimistic' : ''}>{workspace.name}</h3>
        )}
        
        <div className="workspace-meta">
          <span>{workspace.member_count} members</span>
          <span>{workspace.base_count} bases</span>
        </div>
      </div>
      
      <div className="workspace-actions">
        <button 
          onClick={(e) => {
            e.stopPropagation();
            setIsEditing(true);
          }}
          disabled={isOptimistic}
        >
          Edit
        </button>
      </div>
      
      {error && (
        <div className="error-message">
          Failed to update workspace
        </div>
      )}
    </div>
  );
}

function CreateWorkspaceForm({ 
  onCreated 
}: { 
  onCreated: (workspace: WorkspaceService.Workspace) => void 
}) {
  const pyairtable = usePyAirtable();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [isOpen, setIsOpen] = useState(false);

  const { mutate, isOptimistic, error } = useOptimisticUpdate(
    async () => {
      const workspace = await pyairtable.services.workspace.createWorkspace({
        name: name.trim(),
        description: description.trim() || undefined,
      });
      
      onCreated(workspace);
      setName('');
      setDescription('');
      setIsOpen(false);
      
      return workspace;
    }
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (name.trim()) {
      // Create optimistic workspace
      const optimisticWorkspace: WorkspaceService.Workspace = {
        id: `temp_${Date.now()}`,
        name: name.trim(),
        description: description.trim() || undefined,
        owner_id: 'current_user',
        tenant_id: 'current_tenant',
        is_active: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        member_count: 1,
        base_count: 0,
      };
      
      onCreated(optimisticWorkspace);
      
      try {
        await mutate();
      } catch (err) {
        // Remove optimistic workspace on error
        // This would be handled by the rollback mechanism
      }
    }
  };

  if (!isOpen) {
    return (
      <button 
        className="create-workspace-btn"
        onClick={() => setIsOpen(true)}
      >
        + Create Workspace
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="create-workspace-form">
      <h3>Create New Workspace</h3>
      
      <input
        type="text"
        placeholder="Workspace name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
        disabled={isOptimistic}
      />
      
      <textarea
        placeholder="Description (optional)"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        disabled={isOptimistic}
      />
      
      <div className="form-actions">
        <button 
          type="submit" 
          disabled={!name.trim() || isOptimistic}
          className={isOptimistic ? 'optimistic' : ''}
        >
          {isOptimistic ? 'Creating...' : 'Create'}
        </button>
        
        <button 
          type="button" 
          onClick={() => {
            setIsOpen(false);
            setName('');
            setDescription('');
          }}
          disabled={isOptimistic}
        >
          Cancel
        </button>
      </div>
      
      {error && (
        <div className="error-message">
          Failed to create workspace: {error.message}
        </div>
      )}
    </form>
  );
}

// ============================================================================
// Workspace Details with Record Management
// ============================================================================

function WorkspaceDetails({ workspaceId }: { workspaceId: string }) {
  const pyairtable = usePyAirtable();
  const [workspace, setWorkspace] = useState<WorkspaceService.Workspace | null>(null);
  const [tables, setTables] = useState<any[]>([]);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function loadWorkspaceDetails() {
      try {
        setLoading(true);
        
        // Load workspace details
        const workspaceData = await pyairtable.services.workspace.getWorkspace(workspaceId);
        setWorkspace(workspaceData);
        
        // Load tables
        const basesData = await pyairtable.services.base.getBases({ workspace_id: workspaceId });
        if (basesData.data.length > 0) {
          const tablesData = await pyairtable.services.table.getTables({ 
            base_id: basesData.data[0].id 
          });
          setTables(tablesData.data);
          if (tablesData.data.length > 0) {
            setSelectedTable(tablesData.data[0].id);
          }
        }
      } catch (err) {
        console.error('Failed to load workspace details:', err);
      } finally {
        setLoading(false);
      }
    }

    loadWorkspaceDetails();
  }, [workspaceId, pyairtable]);

  if (loading) return <LoadingSpinner />;
  if (!workspace) return <ErrorMessage message="Workspace not found" />;

  return (
    <div className="workspace-details">
      <div className="workspace-header">
        <h1>{workspace.name}</h1>
        {workspace.description && <p>{workspace.description}</p>}
      </div>
      
      {selectedTable ? (
        <RecordTable tableId={selectedTable} />
      ) : (
        <div className="empty-state">
          <h2>No tables found</h2>
          <p>Create a base and table to start managing records</p>
        </div>
      )}
    </div>
  );
}

// ============================================================================
// Record Table with Real-time Updates
// ============================================================================

function RecordTable({ tableId }: { tableId: string }) {
  const pyairtable = usePyAirtable();
  const [records, setRecords] = useState<RecordService.Record[]>([]);
  const [fields, setFields] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  // Load initial data
  useEffect(() => {
    async function loadTableData() {
      try {
        setLoading(true);
        
        // Load fields
        const fieldsData = await pyairtable.services.field.getFields({ table_id: tableId });
        setFields(fieldsData);
        
        // Load records
        const recordsData = await pyairtable.services.record.getRecords({
          table_id: tableId,
          page_size: 50,
        });
        setRecords(recordsData.data);
      } catch (err) {
        console.error('Failed to load table data:', err);
      } finally {
        setLoading(false);
      }
    }

    loadTableData();
  }, [tableId, pyairtable]);

  // Real-time record updates
  useRealtime(
    { 
      channel: 'record', 
      filters: { table_id: tableId },
      enabled: true 
    },
    (event) => {
      switch (event.type) {
        case 'record.created':
          setRecords(prev => [event.payload.record, ...prev]);
          break;
          
        case 'record.updated':
          setRecords(prev =>
            prev.map(record =>
              record.id === event.payload.record.id
                ? event.payload.record
                : record
            )
          );
          break;
          
        case 'record.deleted':
          setRecords(prev =>
            prev.filter(record => record.id !== event.payload.record_id)
          );
          break;
      }
    }
  );

  if (loading) return <LoadingSpinner />;

  return (
    <div className="record-table">
      <div className="table-header">
        <h2>Records</h2>
        <CreateRecordButton 
          tableId={tableId}
          fields={fields}
          onCreated={(record) => setRecords(prev => [record, ...prev])}
        />
      </div>
      
      <div className="table-container">
        <table>
          <thead>
            <tr>
              {fields.map(field => (
                <th key={field.id}>{field.name}</th>
              ))}
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {records.map(record => (
              <RecordRow
                key={record.id}
                record={record}
                fields={fields}
                onUpdate={(updated) => {
                  setRecords(prev =>
                    prev.map(r => r.id === updated.id ? updated : r)
                  );
                }}
                onDelete={(deletedId) => {
                  setRecords(prev => prev.filter(r => r.id !== deletedId));
                }}
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function RecordRow({ 
  record, 
  fields, 
  onUpdate, 
  onDelete 
}: {
  record: RecordService.Record;
  fields: any[];
  onUpdate: (record: RecordService.Record) => void;
  onDelete: (id: string) => void;
}) {
  const pyairtable = usePyAirtable();
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState(record.fields);

  const { mutate: updateMutate, isOptimistic: isUpdating } = useOptimisticUpdate(
    async () => {
      const updated = await pyairtable.services.record.updateRecord(record.id, {
        fields: editData,
      });
      onUpdate(updated);
      setIsEditing(false);
      return updated;
    }
  );

  const { mutate: deleteMutate, isOptimistic: isDeleting } = useOptimisticUpdate(
    async () => {
      await pyairtable.services.record.deleteRecord(record.id);
      onDelete(record.id);
    }
  );

  const handleUpdate = async () => {
    // Apply optimistic update
    onUpdate({ ...record, fields: editData });
    
    try {
      await updateMutate();
    } catch (err) {
      // Rollback on error
      onUpdate(record);
      setEditData(record.fields);
    }
  };

  const handleDelete = async () => {
    if (confirm('Are you sure you want to delete this record?')) {
      // Apply optimistic delete
      onDelete(record.id);
      
      try {
        await deleteMutate();
      } catch (err) {
        // Rollback on error
        onUpdate(record);
      }
    }
  };

  return (
    <tr className={`${isUpdating ? 'optimistic' : ''} ${isDeleting ? 'deleting' : ''}`}>
      {fields.map(field => (
        <td key={field.id}>
          {isEditing ? (
            <input
              type="text"
              value={editData[field.name] || ''}
              onChange={(e) => setEditData(prev => ({
                ...prev,
                [field.name]: e.target.value,
              }))}
            />
          ) : (
            <span>{record.fields[field.name] || ''}</span>
          )}
        </td>
      ))}
      <td>
        {isEditing ? (
          <div className="edit-actions">
            <button onClick={handleUpdate} disabled={isUpdating}>
              Save
            </button>
            <button onClick={() => {
              setIsEditing(false);
              setEditData(record.fields);
            }}>
              Cancel
            </button>
          </div>
        ) : (
          <div className="record-actions">
            <button onClick={() => setIsEditing(true)} disabled={isDeleting}>
              Edit
            </button>
            <button onClick={handleDelete} disabled={isDeleting}>
              Delete
            </button>
          </div>
        )}
      </td>
    </tr>
  );
}

function CreateRecordButton({ 
  tableId, 
  fields, 
  onCreated 
}: {
  tableId: string;
  fields: any[];
  onCreated: (record: RecordService.Record) => void;
}) {
  const pyairtable = usePyAirtable();
  const [isOpen, setIsOpen] = useState(false);
  const [formData, setFormData] = useState<Record<string, string>>({});

  const { mutate, isOptimistic, error } = useOptimisticUpdate(
    async () => {
      const record = await pyairtable.services.record.createRecord({
        table_id: tableId,
        fields: formData,
      });
      
      onCreated(record);
      setFormData({});
      setIsOpen(false);
      
      return record;
    }
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Create optimistic record
    const optimisticRecord: RecordService.Record = {
      id: `temp_${Date.now()}`,
      table_id: tableId,
      fields: formData,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
      created_by: 'current_user',
      updated_by: 'current_user',
    };
    
    onCreated(optimisticRecord);
    
    try {
      await mutate();
    } catch (err) {
      // Rollback handled by optimistic update system
    }
  };

  if (!isOpen) {
    return (
      <button onClick={() => setIsOpen(true)} className="create-record-btn">
        + Add Record
      </button>
    );
  }

  return (
    <form onSubmit={handleSubmit} className="create-record-form">
      {fields.map(field => (
        <div key={field.id} className="form-field">
          <label>{field.name}</label>
          <input
            type="text"
            value={formData[field.name] || ''}
            onChange={(e) => setFormData(prev => ({
              ...prev,
              [field.name]: e.target.value,
            }))}
            disabled={isOptimistic}
          />
        </div>
      ))}
      
      <div className="form-actions">
        <button type="submit" disabled={isOptimistic}>
          {isOptimistic ? 'Creating...' : 'Create'}
        </button>
        <button type="button" onClick={() => setIsOpen(false)}>
          Cancel
        </button>
      </div>
      
      {error && (
        <div className="error-message">
          Failed to create record: {error.message}
        </div>
      )}
    </form>
  );
}

// ============================================================================
// Utility Components
// ============================================================================

function LoadingSpinner() {
  return (
    <div className="loading-spinner">
      <div className="spinner"></div>
      <span>Loading...</span>
    </div>
  );
}

function ErrorMessage({ 
  message, 
  onRetry 
}: { 
  message: string; 
  onRetry?: () => void 
}) {
  return (
    <div className="error-message">
      <h3>Error</h3>
      <p>{message}</p>
      {onRetry && (
        <button onClick={onRetry}>Try Again</button>
      )}
    </div>
  );
}

function ConnectionStatus() {
  const pyairtable = usePyAirtable();
  const [status, setStatus] = useState<any>(null);

  useEffect(() => {
    const interval = setInterval(() => {
      setStatus(pyairtable.getConnectionStatus());
    }, 5000);

    return () => clearInterval(interval);
  }, [pyairtable]);

  if (!status) return null;

  const isOnline = status.realtime.activeClient !== null;
  const hasOfflineQueue = status.http.offlineQueueSize > 0;

  return (
    <div className={`connection-status ${isOnline ? 'online' : 'offline'}`}>
      <div className="status-indicator">
        <span className={`dot ${isOnline ? 'green' : 'red'}`}></span>
        {isOnline ? 'Online' : 'Offline'}
      </div>
      
      {hasOfflineQueue && (
        <div className="offline-queue">
          {status.http.offlineQueueSize} queued requests
        </div>
      )}
      
      {status.optimistic?.hasUpdates && (
        <div className="optimistic-updates">
          {status.optimistic.pendingUpdates} pending updates
        </div>
      )}
    </div>
  );
}

export default App;