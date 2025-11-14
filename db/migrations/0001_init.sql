-- Enable pgcrypto for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- tenants, projects, apps, events (skeleton)
CREATE TABLE IF NOT EXISTS tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  name TEXT UNIQUE NOT NULL,
  owners TEXT[] DEFAULT '{}'::text[],
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);
ALTER TABLE IF EXISTS tenants ADD COLUMN IF NOT EXISTS owners TEXT[] DEFAULT '{}'::text[];
ALTER TABLE IF EXISTS tenants ADD COLUMN IF NOT EXISTS labels JSONB DEFAULT '{}'::jsonb;
CREATE TABLE IF NOT EXISTS projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,name)
);
CREATE TABLE IF NOT EXISTS apps (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  tenant TEXT NOT NULL,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,project,name)
);

-- clusters table for manager-driven registration and status
CREATE TABLE IF NOT EXISTS clusters (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  uid TEXT UNIQUE NOT NULL,
  name TEXT UNIQUE NOT NULL,
  kubeconfig_enc TEXT NOT NULL,
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);

-- indexes for clusters listing and label filtering
CREATE INDEX IF NOT EXISTS clusters_labels_gin_idx ON clusters USING gin(labels);

CREATE TABLE IF NOT EXISTS events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cluster_id UUID NULL REFERENCES clusters(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  resource TEXT NOT NULL,
  payload JSONB,
  ts TIMESTAMP DEFAULT NOW()
);

-- history of computed cluster conditions
CREATE TABLE IF NOT EXISTS cluster_conditions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cluster_id UUID NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  reason TEXT,
  message TEXT,
  ts TIMESTAMP DEFAULT NOW()
);
