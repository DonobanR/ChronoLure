package models

import (
	"database/sql"
	"path/filepath"
	"testing"

	"bitbucket.org/liamstask/goose/lib/goose"
	_ "github.com/mattn/go-sqlite3"
)

// Migration versions straddling the CL-102R rework.
const (
	verBeforeSoftDelete = int64(20260728000002) // last version WITH results.excluded*
	verSoftDelete       = int64(20260729000001) // adds soft-delete cols + converts
	verDropExcluded     = int64(20260729000002) // drops results.excluded*
)

// migrateTo runs the sqlite migrations of this repo up to (or down to) target on
// the given database. It works on an isolated temp file DB so it never disturbs
// the shared in-memory database used by the gocheck suite.
func migrateTo(t *testing.T, dbConn *sql.DB, dbPath string, target int64) {
	t.Helper()
	dir, err := filepath.Abs("../db/db_sqlite3/migrations/")
	if err != nil {
		t.Fatalf("abs migrations dir: %v", err)
	}
	conf := &goose.DBConf{
		MigrationsDir: dir,
		Env:           "production",
		Driver: goose.DBDriver{
			Name:    "sqlite3",
			OpenStr: dbPath,
			Import:  "github.com/mattn/go-sqlite3",
			Dialect: &goose.Sqlite3Dialect{},
		},
	}
	if err := goose.RunMigrationsOnDb(conf, conf.MigrationsDir, target, dbConn); err != nil {
		t.Fatalf("migrate to %d: %v", target, err)
	}
}

// TestMigrationConvertsExcludedToSoftDeleted and
// TestMigrationWritesAuditEntryPerRow (ticket §10, "Migración"): a row left over
// from the CL-102 `excluded` design must be CONVERTED to a soft delete (never
// dropped), keeping its reason/actor, and must leave one audit entry behind so the
// history is not mute. Runs the real .sql files against a throwaway DB.
func TestMigrationConvertsExcludedToSoftDeleted(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migration.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	defer conn.Close()

	// 1) Schema as it was BEFORE the rework (results.excluded* present).
	migrateTo(t, conn, dbPath, verBeforeSoftDelete)
	if _, err := conn.Exec(`INSERT INTO campaigns (id, user_id, name) VALUES (7, 1, 'Campaña Legacy')`); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	// Two rows: one excluded (must convert), one untouched (must stay active).
	if _, err := conn.Exec(`INSERT INTO results (id, campaign_id, user_id, r_id, email, status, excluded, excluded_reason, excluded_by, excluded_at)
	                        VALUES (101, 7, 1, 'legacyRID', 'qa@empresa.com', 'Email Sent', 1, 'correo interno QA', 1, '2026-07-01 10:00:00')`); err != nil {
		t.Fatalf("seed excluded result: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO results (id, campaign_id, user_id, r_id, email, status, excluded)
	                        VALUES (102, 7, 1, 'activeRID', 'real@cliente.com', 'Email Sent', 0)`); err != nil {
		t.Fatalf("seed active result: %v", err)
	}

	// 2) Apply the conversion migration.
	migrateTo(t, conn, dbPath, verSoftDelete)

	var deletedAt, reason, scope sql.NullString
	var deletedBy sql.NullInt64
	if err := conn.QueryRow(`SELECT deleted_at, delete_reason, delete_scope, deleted_by FROM results WHERE id = 101`).
		Scan(&deletedAt, &reason, &scope, &deletedBy); err != nil {
		t.Fatalf("read converted row: %v", err)
	}
	if !deletedAt.Valid || deletedAt.String == "" {
		t.Fatalf("excluded row was not converted to soft-delete (deleted_at empty)")
	}
	if reason.String != "correo interno QA" {
		t.Fatalf("reason not preserved: %q", reason.String)
	}
	if scope.String != "campaign" {
		t.Fatalf("delete_scope = %q, want campaign", scope.String)
	}
	if deletedBy.Int64 != 1 {
		t.Fatalf("deleted_by = %d, want 1", deletedBy.Int64)
	}

	// The untouched row must remain active — the migration converts, never sweeps.
	var activeDeleted sql.NullString
	if err := conn.QueryRow(`SELECT deleted_at FROM results WHERE id = 102`).Scan(&activeDeleted); err != nil {
		t.Fatalf("read active row: %v", err)
	}
	if activeDeleted.Valid {
		t.Fatalf("a non-excluded row was soft-deleted by the migration")
	}

	// One audit entry per migrated row, tagged with its provenance.
	var audits int
	var metadata string
	if err := conn.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'recipient_soft_deleted' AND actor_name = 'system:cl-102r-migration'`).Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits != 1 {
		t.Fatalf("expected exactly 1 migration audit row, got %d", audits)
	}
	if err := conn.QueryRow(`SELECT metadata FROM audit_log WHERE actor_name = 'system:cl-102r-migration'`).Scan(&metadata); err != nil {
		t.Fatalf("read audit metadata: %v", err)
	}
	for _, want := range []string{"migrated_from", "excluded", "CL-102R", "qa@empresa.com"} {
		if !contains(metadata, want) {
			t.Fatalf("audit metadata %q missing %q", metadata, want)
		}
	}

	// 3) Dropping the legacy columns leaves the converted data intact.
	migrateTo(t, conn, dbPath, verDropExcluded)
	var stillDeleted sql.NullString
	if err := conn.QueryRow(`SELECT deleted_at FROM results WHERE id = 101`).Scan(&stillDeleted); err != nil {
		t.Fatalf("read after drop: %v", err)
	}
	if !stillDeleted.Valid {
		t.Fatalf("converted row lost its soft-delete after dropping excluded columns")
	}
	if _, err := conn.Query(`SELECT excluded FROM results LIMIT 1`); err == nil {
		t.Fatalf("results.excluded still exists after the drop migration")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
