# Migration Guide: MariaDB to PostgreSQL with TimescaleDB

This guide helps you migrate an existing GTG Live Map installation from MariaDB to PostgreSQL with TimescaleDB extension for optimized time-series data storage.

## Prerequisites

- Docker and Docker Compose installed
- Backup of your current MariaDB database
- Access to your current installation

## Step-by-Step Migration

### 1. Backup Your Current Data

Before starting the migration, create a backup of your MariaDB database:

```bash
# If using Docker Compose, run this command:
docker-compose exec db mysqldump -u root -p gtglivemap > backup.sql

# Or if you have direct access to MariaDB:
mysqldump -u root -p gtglivemap > backup.sql
```

### 2. Stop the Current Application

```bash
docker-compose down
```

### 3. Update Your Repository

Pull the latest changes that include the PostgreSQL migration:

```bash
git pull origin master
```

### 4. Update Configuration Files

**A) Update `config.yaml`:**

Change your database configuration:

```yaml
database:
  host: "postgres"  # Changed from 'db' or 'localhost'
  port: 5432        # Changed from 3306
  user: "gtglivemap"
  password: "your_secure_password"
  dbname: "gtglivemap"
```

Also update Redis configuration if needed:

```yaml
redis:
  addr: "redis:6379"
  username: ""
  password: "your_redis_password"
  db: 0
```

**B) Update `docker-compose.yaml`:**

The new `docker-compose.yaml` includes PostgreSQL and Redis services. Update the passwords:

- Change `POSTGRES_PASSWORD` to a strong, unique password
- Change the Redis password in the command (`--requirepass`)

Make sure these passwords match your `config.yaml`.

### 5. Data Migration

Since PostgreSQL and MariaDB have different syntax and data types, you'll need to migrate your data. Here are your options:

#### Option A: Fresh Start (Recommended for Small/Testing Installations)

If you have minimal data or can afford to start fresh:

1. Start the new stack:
   ```bash
   docker-compose up -d
   ```

2. The application will automatically:
   - Create the PostgreSQL schema
   - Enable the TimescaleDB extension
   - Convert `player_positions` and `damage_events` tables to TimescaleDB hypertables for optimized time-series storage

3. Reconfigure your servers in the dashboard.

#### Option B: Migrate Existing Data

For installations with important historical data, you'll need to convert the MySQL dump to PostgreSQL format:

1. Install `pgloader` (a tool for migrating from MySQL to PostgreSQL):
   ```bash
   apt-get install pgloader  # Debian/Ubuntu
   # or
   brew install pgloader      # macOS
   ```

2. Create a pgloader configuration file `migration.load`:
   ```
   LOAD DATABASE
        FROM mysql://root:password@localhost/gtglivemap
        INTO postgresql://gtglivemap:password@localhost/gtglivemap
   
   WITH include drop, create tables, create indexes, reset sequences
   
   SET work_mem to '256MB', maintenance_work_mem to '512MB';
   ```

3. Run the migration:
   ```bash
   docker-compose up -d postgres  # Start only PostgreSQL
   pgloader migration.load
   ```

4. Start the full stack:
   ```bash
   docker-compose up -d
   ```

### 6. Verify the Migration

1. Check that the application starts successfully:
   ```bash
   docker-compose logs -f app
   ```

2. Access the web interface at `http://localhost:8080`

3. Verify your data:
   - Check that servers are listed correctly
   - Verify map configurations
   - Test position data display
   - Check historical data (if migrated)

### 7. Update Your Backups

Update your backup scripts to use PostgreSQL commands:

```bash
# PostgreSQL backup command
docker-compose exec postgres pg_dump -U gtglivemap gtglivemap > backup.sql

# Restore command
docker-compose exec -T postgres psql -U gtglivemap gtglivemap < backup.sql
```

## Troubleshooting

### Connection Issues

If the app can't connect to PostgreSQL:

1. Check that the PostgreSQL service is healthy:
   ```bash
   docker-compose ps
   ```

2. Verify the configuration in `config.yaml` matches `docker-compose.yaml`

3. Check logs:
   ```bash
   docker-compose logs postgres
   docker-compose logs app
   ```

### Schema Issues

If you encounter schema-related errors:

1. Drop and recreate the database:
   ```bash
   docker-compose exec postgres psql -U gtglivemap -c "DROP DATABASE IF EXISTS gtglivemap;"
   docker-compose exec postgres psql -U gtglivemap -c "CREATE DATABASE gtglivemap;"
   ```

2. Restart the application to trigger automatic migration:
   ```bash
   docker-compose restart app
   ```

### TimescaleDB Issues

If you encounter TimescaleDB-related errors:

1. Verify that the TimescaleDB extension is enabled:
   ```bash
   docker-compose exec postgres psql -U gtglivemap -d gtglivemap -c "SELECT * FROM pg_extension WHERE extname = 'timescaledb';"
   ```

2. Check if hypertables were created successfully:
   ```bash
   docker-compose exec postgres psql -U gtglivemap -d gtglivemap -c "SELECT * FROM timescaledb_information.hypertables;"
   ```

3. If hypertables weren't created, you can manually create them:
   ```bash
   docker-compose exec postgres psql -U gtglivemap -d gtglivemap -c "SELECT create_hypertable('player_positions', 'event_timestamp', if_not_exists => TRUE);"
   docker-compose exec postgres psql -U gtglivemap -d gtglivemap -c "SELECT create_hypertable('damage_events', 'event_timestamp', if_not_exists => TRUE);"
   ```

### Performance Optimization

After migration, consider these PostgreSQL/TimescaleDB optimizations:

1. **TimescaleDB-specific**:
   - Configure data retention policies to automatically remove old data
   - Enable compression for older chunks to save disk space
   - Tune chunk intervals based on your data ingestion rate

2. **PostgreSQL general**:
   - Create additional indexes for frequently queried columns
   - Adjust `work_mem` and `shared_buffers` in PostgreSQL configuration
   - Enable connection pooling if running multiple app instances

Example retention policy (keeps data for 90 days):
```sql
SELECT add_retention_policy('player_positions', INTERVAL '90 days');
SELECT add_retention_policy('damage_events', INTERVAL '90 days');
```

## Key Differences

### Database Technology

- **Database**: MariaDB → PostgreSQL with TimescaleDB extension
- **Time-Series Tables**: `player_positions` and `damage_events` are now TimescaleDB hypertables, providing:
  - Automatic data partitioning by time
  - Optimized query performance for time-based queries
  - Built-in data retention policies
  - Compression for older data

### Data Types

- **UUIDs**: Now stored as native PostgreSQL `uuid` type instead of `varchar(36)`
- **Enums**: Replaced with `varchar` with CHECK constraints
- **Binary Data**: Changed from `longblob` to `bytea`

### SQL Functions

- **Modulo**: Changed from `MOD(a, b)` to `a % b`
- **String Concatenation**: Use `||` instead of `CONCAT()`
- **Case Sensitivity**: PostgreSQL identifiers are case-sensitive by default

## Rollback

If you need to rollback to MariaDB:

1. Stop the current stack:
   ```bash
   docker-compose down
   ```

2. Checkout the previous version:
   ```bash
   git checkout <previous-commit-hash>
   ```

3. Restore your MariaDB backup and restart the old stack.

## Support

If you encounter issues during migration, please:

1. Check the logs: `docker-compose logs`
2. Review this guide carefully
3. Open an issue on GitHub with detailed error messages
