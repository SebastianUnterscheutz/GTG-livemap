# Migration Guide: MariaDB to PostgreSQL

This guide helps you migrate an existing GTG Live Map installation from MariaDB to PostgreSQL.

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

2. The application will automatically create the PostgreSQL schema on first run.

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

### Performance Optimization

After migration, consider these PostgreSQL optimizations:

1. Create additional indexes for frequently queried columns
2. Adjust `work_mem` and `shared_buffers` in PostgreSQL configuration
3. Enable connection pooling if running multiple app instances

## Key Differences

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
