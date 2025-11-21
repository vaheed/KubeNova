-- 0003_catalog_items.sql
-- Introduce catalog_items table for Phase2 catalog storage.
-- Idempotent; safe to rerun.

CREATE TABLE IF NOT EXISTS catalog_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT,
  icon TEXT,
  category TEXT,
  version TEXT,
  scope TEXT NOT NULL DEFAULT 'global',
  tenant_id UUID NULL REFERENCES tenants(id) ON DELETE SET NULL,
  source JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMP DEFAULT NOW()
);

ALTER TABLE catalog_items
  ADD CONSTRAINT catalog_items_scope_check CHECK (scope IN ('global', 'tenant'));

CREATE INDEX IF NOT EXISTS catalog_items_tenant_idx ON catalog_items(tenant_id);
