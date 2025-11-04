# db/

Database migrations and notes.

- `migrations/0001_init.sql` – baseline schema for tenants, projects, apps, clusters, events, condition history.
- The Manager prefers Postgres when `DATABASE_URL` is set.
- Tests fallback to an embedded copy of the baseline SQL if the file is not present at runtime.

Applying migrations
- In CI the Manager applies the SQL on startup.
- For manual apply:
```
psql "$DATABASE_URL" -f db/migrations/0001_init.sql
```
