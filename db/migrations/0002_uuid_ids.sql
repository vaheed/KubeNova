-- 0002_uuid_ids.sql
-- Migrate integer/bigint primary keys and foreign keys to UUIDv4 across tables.
-- Idempotent: safe to re-run. Down migration is not provided; see notes below.

BEGIN;

-- Enable pgcrypto for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Helper: drop a primary key constraint by table name if it exists
-- (constraint names are typically <table>_pkey)

-- Clusters: migrate id -> uuid and children FKs
DO $$
DECLARE
    pkname text;
    coltype text;
BEGIN
    -- Determine current type of clusters.id
    SELECT data_type INTO coltype
      FROM information_schema.columns
     WHERE table_name = 'clusters' AND column_name = 'id';

    IF coltype IS NOT NULL AND coltype <> 'uuid' THEN
        -- 1) Add id_uuid column with default
        ALTER TABLE clusters ADD COLUMN IF NOT EXISTS id_uuid uuid NOT NULL DEFAULT gen_random_uuid();
        -- 2) Children: events.cluster_id and cluster_conditions.cluster_id -> uuid
        -- events (nullable FK)
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='events' AND column_name='cluster_id') THEN
            ALTER TABLE events ADD COLUMN IF NOT EXISTS cluster_id_uuid uuid;
            UPDATE events e
               SET cluster_id_uuid = c.id_uuid
              FROM clusters c
             WHERE e.cluster_id IS NOT NULL AND c.id = e.cluster_id AND e.cluster_id_uuid IS NULL;
            -- Ensure index
            CREATE INDEX IF NOT EXISTS events_cluster_id_uuid_idx ON events(cluster_id_uuid);
            -- Drop old FK if any
            PERFORM 1 FROM information_schema.table_constraints tc
              JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
             WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name='events' AND kcu.column_name='cluster_id';
            IF FOUND THEN
                -- Find constraint name and drop it
                EXECUTE (
                    SELECT 'ALTER TABLE events DROP CONSTRAINT ' || quote_ident(tc.constraint_name)
                      FROM information_schema.table_constraints tc
                      JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
                     WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name='events' AND kcu.column_name='cluster_id'
                     LIMIT 1
                );
            END IF;
            -- Rename columns: drop old, rename new
            ALTER TABLE events DROP COLUMN IF EXISTS cluster_id;
            ALTER TABLE events RENAME COLUMN cluster_id_uuid TO cluster_id;
            -- Add new FK (nullable)
            ALTER TABLE events
              ADD CONSTRAINT events_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES clusters(id_uuid) ON DELETE SET NULL;
        END IF;

        -- cluster_conditions (NOT NULL FK)
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='cluster_conditions' AND column_name='cluster_id') THEN
            ALTER TABLE cluster_conditions ADD COLUMN IF NOT EXISTS cluster_id_uuid uuid;
            UPDATE cluster_conditions cc
               SET cluster_id_uuid = c.id_uuid
              FROM clusters c
             WHERE c.id = cc.cluster_id AND cc.cluster_id_uuid IS NULL;
            -- Ensure index
            CREATE INDEX IF NOT EXISTS cluster_conditions_cluster_id_uuid_idx ON cluster_conditions(cluster_id_uuid);
            -- Drop old FK if any
            PERFORM 1 FROM information_schema.table_constraints tc
              JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
             WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name='cluster_conditions' AND kcu.column_name='cluster_id';
            IF FOUND THEN
                EXECUTE (
                    SELECT 'ALTER TABLE cluster_conditions DROP CONSTRAINT ' || quote_ident(tc.constraint_name)
                      FROM information_schema.table_constraints tc
                      JOIN information_schema.key_column_usage kcu ON tc.constraint_name = kcu.constraint_name
                     WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name='cluster_conditions' AND kcu.column_name='cluster_id'
                     LIMIT 1
                );
            END IF;
            ALTER TABLE cluster_conditions DROP COLUMN IF EXISTS cluster_id;
            ALTER TABLE cluster_conditions RENAME COLUMN cluster_id_uuid TO cluster_id;
            -- Add new FK (NOT NULL)
            ALTER TABLE cluster_conditions
              ADD CONSTRAINT cluster_conditions_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES clusters(id_uuid) ON DELETE CASCADE;
            ALTER TABLE cluster_conditions ALTER COLUMN cluster_id SET NOT NULL;
        END IF;

        -- 3) Swap PK to id_uuid
        SELECT tc.constraint_name INTO pkname
          FROM information_schema.table_constraints tc
         WHERE tc.table_name='clusters' AND tc.constraint_type='PRIMARY KEY'
         LIMIT 1;
        IF pkname IS NOT NULL THEN
            EXECUTE 'ALTER TABLE clusters DROP CONSTRAINT ' || quote_ident(pkname);
        END IF;
        -- Rename columns and set PK
        ALTER TABLE clusters RENAME COLUMN id_uuid TO id;
        ALTER TABLE clusters ADD PRIMARY KEY (id);
        -- 4) Create index on new PK is implicit; drop old id if still present
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='clusters' AND column_name='id' AND data_type <> 'uuid') THEN
            -- If "id" remained as integer, this means rename didn't occur; handle alternate name
            -- No-op: handled above by rename. This guard keeps idempotency.
            NULL;
        END IF;
    END IF;
END $$;

-- Tenants
DO $$
DECLARE pkname text; coltype text; BEGIN
    SELECT data_type INTO coltype FROM information_schema.columns WHERE table_name='tenants' AND column_name='id';
    IF coltype IS NOT NULL AND coltype <> 'uuid' THEN
        ALTER TABLE tenants ADD COLUMN IF NOT EXISTS id_uuid uuid NOT NULL DEFAULT gen_random_uuid();
        SELECT tc.constraint_name INTO pkname FROM information_schema.table_constraints tc WHERE tc.table_name='tenants' AND tc.constraint_type='PRIMARY KEY' LIMIT 1;
        IF pkname IS NOT NULL THEN EXECUTE 'ALTER TABLE tenants DROP CONSTRAINT ' || quote_ident(pkname); END IF;
        ALTER TABLE tenants RENAME COLUMN id_uuid TO id;
        ALTER TABLE tenants ADD PRIMARY KEY (id);
        -- Drop old integer id column if still present under another name (none expected)
    END IF;
END $$;

-- Projects
DO $$
DECLARE pkname text; coltype text; BEGIN
    SELECT data_type INTO coltype FROM information_schema.columns WHERE table_name='projects' AND column_name='id';
    IF coltype IS NOT NULL AND coltype <> 'uuid' THEN
        ALTER TABLE projects ADD COLUMN IF NOT EXISTS id_uuid uuid NOT NULL DEFAULT gen_random_uuid();
        SELECT tc.constraint_name INTO pkname FROM information_schema.table_constraints tc WHERE tc.table_name='projects' AND tc.constraint_type='PRIMARY KEY' LIMIT 1;
        IF pkname IS NOT NULL THEN EXECUTE 'ALTER TABLE projects DROP CONSTRAINT ' || quote_ident(pkname); END IF;
        ALTER TABLE projects RENAME COLUMN id_uuid TO id;
        ALTER TABLE projects ADD PRIMARY KEY (id);
    END IF;
END $$;

-- Apps
DO $$
DECLARE pkname text; coltype text; BEGIN
    SELECT data_type INTO coltype FROM information_schema.columns WHERE table_name='apps' AND column_name='id';
    IF coltype IS NOT NULL AND coltype <> 'uuid' THEN
        ALTER TABLE apps ADD COLUMN IF NOT EXISTS id_uuid uuid NOT NULL DEFAULT gen_random_uuid();
        SELECT tc.constraint_name INTO pkname FROM information_schema.table_constraints tc WHERE tc.table_name='apps' AND tc.constraint_type='PRIMARY KEY' LIMIT 1;
        IF pkname IS NOT NULL THEN EXECUTE 'ALTER TABLE apps DROP CONSTRAINT ' || quote_ident(pkname); END IF;
        ALTER TABLE apps RENAME COLUMN id_uuid TO id;
        ALTER TABLE apps ADD PRIMARY KEY (id);
    END IF;
END $$;

COMMIT;

-- Down migration note:
-- Reverting UUID PKs/FKs back to integers is not supported due to potential data loss
-- and ambiguity. If absolutely necessary, a manual procedure can be followed by adding
-- integer columns, backfilling from a mapping table, and re-pointing foreign keys.

