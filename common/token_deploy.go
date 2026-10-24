package common

import "os"

// TOKEN: on Cloud Run there is no single long-lived master that migrates at
// startup. Database migrations run in a one-off job before each deploy:
//
//   - TOKEN_MIGRATE_ONLY=true (master): run every startup migration, then
//     exit 0 instead of serving.
//   - TOKEN_SKIP_MIGRATIONS=true: a master that runs the singleton background
//     tasks (cleanup, system task runner) but never migrates.
//   - TOKEN_RETENTION_ONLY=true: delete log rows past their retention period
//     (model.TokenEnforceRetention), then exit 0; never migrates.
//
// Neither is set on upstream-style deployments, so behaviour is unchanged.
var (
	TokenMigrateOnly    = os.Getenv("TOKEN_MIGRATE_ONLY") == "true"
	TokenRetentionOnly  = os.Getenv("TOKEN_RETENTION_ONLY") == "true" && !TokenMigrateOnly
	TokenSkipMigrations = os.Getenv("TOKEN_SKIP_MIGRATIONS") == "true" && !TokenMigrateOnly
)

// TokenMigrates reports whether this process runs database migrations.
func TokenMigrates() bool {
	return IsMasterNode && !TokenSkipMigrations && !TokenRetentionOnly
}
