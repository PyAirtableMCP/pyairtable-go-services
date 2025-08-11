-- Initialize databases for PyAirtable microservices

-- Create databases for each service
CREATE DATABASE pyairtable_users;
CREATE DATABASE pyairtable_workspaces;
CREATE DATABASE pyairtable_airtable;
CREATE DATABASE pyairtable_files;
CREATE DATABASE pyairtable_permissions;
CREATE DATABASE pyairtable_notifications;
CREATE DATABASE pyairtable_analytics;
CREATE DATABASE pyairtable_ai;

-- Create a shared database for event sourcing
CREATE DATABASE pyairtable_events;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE pyairtable_users TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_workspaces TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_airtable TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_files TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_permissions TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_notifications TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_analytics TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_ai TO postgres;
GRANT ALL PRIVILEGES ON DATABASE pyairtable_events TO postgres;