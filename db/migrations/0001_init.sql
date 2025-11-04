-- tenants, projects, apps, events (skeleton)
CREATE TABLE IF NOT EXISTS tenants (
  id SERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  created_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS projects (
  id SERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,name)
);
CREATE TABLE IF NOT EXISTS apps (
  id SERIAL PRIMARY KEY,
  tenant TEXT NOT NULL,
  project TEXT NOT NULL,
  name TEXT NOT NULL,
  UNIQUE(tenant,project,name)
);
CREATE TABLE IF NOT EXISTS events (
  id BIGSERIAL PRIMARY KEY,
  cluster_id INT NULL REFERENCES clusters(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  resource TEXT NOT NULL,
  payload JSONB,
  ts TIMESTAMP DEFAULT NOW()
);

-- clusters table for manager-driven registration and status
CREATE TABLE IF NOT EXISTS clusters (
  id SERIAL PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  kubeconfig_enc TEXT NOT NULL,
  labels JSONB DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);

-- history of computed cluster conditions
CREATE TABLE IF NOT EXISTS cluster_conditions (
  id BIGSERIAL PRIMARY KEY,
  cluster_id INT NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  reason TEXT,
  message TEXT,
  ts TIMESTAMP DEFAULT NOW()
);
