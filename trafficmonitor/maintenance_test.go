package trafficmonitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanupDeletesExpiredRowsAndReclaimsPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.sqlite")
	database, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.close()

	var autoVacuum int
	if err := database.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		t.Fatal(err)
	}
	if autoVacuum != 2 {
		t.Fatalf("new database did not enable incremental auto-vacuum: %d", autoVacuum)
	}

	now := time.Now().Truncate(time.Minute)
	buckets := make([]minuteBucket, 0, 6101)
	for index := 0; index < 6000; index++ {
		buckets = append(buckets, minuteBucket{
			Minute: now.Add(-31 * 24 * time.Hour).Unix(), Domain: fmt.Sprintf("expired-%04d.example", index),
			Node: "test-node", Rule: "Domain", Network: "tcp", Process: "test", Route: RouteProxy,
			UploadBytes: 1024, DownloadBytes: 4096, ConnectionCount: 1,
		})
	}
	for index := 0; index < 100; index++ {
		buckets = append(buckets, minuteBucket{
			Minute: now.Add(-time.Hour).Unix(), Domain: fmt.Sprintf("recent-%04d.example", index),
			Node: "test-node", Rule: "Domain", Network: "tcp", Process: "test", Route: RouteProxy,
			UploadBytes: 1024, DownloadBytes: 4096, ConnectionCount: 1,
		})
	}
	buckets = append(buckets, minuteBucket{
		Minute: now.Add(-30 * 24 * time.Hour).Unix(), Domain: "cutoff.example",
		Node: "test-node", Rule: "Domain", Network: "tcp", Process: "test", Route: RouteProxy,
		UploadBytes: 1024, DownloadBytes: 4096, ConnectionCount: 1,
	})
	if err := database.upsertBuckets(context.Background(), buckets); err != nil {
		t.Fatal(err)
	}
	if err := database.checkpointWAL(context.Background()); err != nil {
		t.Fatal(err)
	}

	beforePages := pragmaInteger(t, database, `PRAGMA page_count`)
	beforeFile, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.cleanup(context.Background(), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	afterPages := pragmaInteger(t, database, `PRAGMA page_count`)
	afterFile, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	freePages := pragmaInteger(t, database, `PRAGMA freelist_count`)
	if afterPages >= beforePages {
		t.Fatalf("cleanup did not shrink database pages: before=%d after=%d", beforePages, afterPages)
	}
	if afterFile.Size() >= beforeFile.Size() {
		t.Fatalf("cleanup did not shrink database file: before=%d after=%d", beforeFile.Size(), afterFile.Size())
	}
	if freePages != 0 {
		t.Fatalf("cleanup left reusable pages instead of returning them to disk: %d", freePages)
	}

	var rows int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM traffic_minute`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 101 {
		t.Fatalf("cleanup retained unexpected row count: %d", rows)
	}
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() != 0 {
		t.Fatalf("cleanup did not truncate WAL file: %d", wal.Size())
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestCleanupContinuesReclaimingBeyondDailyPageBudget(t *testing.T) {
	database, err := openStore(filepath.Join(t.TempDir(), "traffic.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.close()

	now := time.Now().Truncate(time.Minute)
	payload := strings.Repeat("x", 8*1024)
	buckets := make([]minuteBucket, 0, 1501)
	for index := 0; index < 1500; index++ {
		buckets = append(buckets, minuteBucket{
			Minute: now.Add(-31 * 24 * time.Hour).Unix(), Domain: fmt.Sprintf("large-expired-%04d.example", index),
			Node: "test-node", Rule: "Domain", RulePayload: payload, Network: "tcp", Process: "test", Route: RouteProxy,
			UploadBytes: 1024, DownloadBytes: 4096, ConnectionCount: 1,
		})
	}
	buckets = append(buckets, minuteBucket{
		Minute: now.Add(-time.Hour).Unix(), Domain: "recent.example",
		Node: "test-node", Rule: "Domain", Network: "tcp", Process: "test", Route: RouteProxy,
		UploadBytes: 1024, DownloadBytes: 4096, ConnectionCount: 1,
	})
	if err := database.upsertBuckets(context.Background(), buckets); err != nil {
		t.Fatal(err)
	}
	if err := database.checkpointWAL(context.Background()); err != nil {
		t.Fatal(err)
	}

	beforePages := pragmaInteger(t, database, `PRAGMA page_count`)
	if beforePages <= maxIncrementalVacuumPages {
		t.Fatalf("test database did not exceed the daily vacuum budget: %d", beforePages)
	}
	if err := database.cleanup(context.Background(), now.Add(-30*24*time.Hour)); !errors.Is(err, errVacuumPagesRemaining) {
		t.Fatalf("first maintenance should report remaining pages: %v", err)
	}
	remaining := pragmaInteger(t, database, `PRAGMA freelist_count`)
	if remaining == 0 {
		t.Fatal("first maintenance unexpectedly reclaimed more than the configured page budget")
	}

	for attempt := 0; attempt < 8 && remaining > 0; attempt++ {
		previous := remaining
		err := database.cleanup(context.Background(), now.Add(-30*24*time.Hour))
		if err != nil && !errors.Is(err, errVacuumPagesRemaining) {
			t.Fatal(err)
		}
		remaining = pragmaInteger(t, database, `PRAGMA freelist_count`)
		if remaining >= previous {
			t.Fatalf("subsequent maintenance did not reduce free pages: before=%d after=%d", previous, remaining)
		}
	}
	if remaining != 0 {
		t.Fatalf("maintenance did not eventually return all free pages: %d", remaining)
	}
	if afterPages := pragmaInteger(t, database, `PRAGMA page_count`); afterPages >= beforePages {
		t.Fatalf("repeated maintenance did not shrink database: before=%d after=%d", beforePages, afterPages)
	}
}

func TestStoreSwitchesFullAutoVacuumToIncremental(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.sqlite")
	database, err := openLegacyStoreWithAutoVacuum(path, "FULL")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	if mode := pragmaInteger(t, store, `PRAGMA auto_vacuum`); mode != 2 {
		t.Fatalf("full auto-vacuum was not switched to incremental: %d", mode)
	}
}

func openLegacyStoreWithAutoVacuum(path, mode string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := database.Exec(`PRAGMA auto_vacuum=` + mode); err != nil {
		database.Close()
		return nil, err
	}
	if _, err := database.Exec(`CREATE TABLE legacy (value TEXT)`); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func pragmaInteger(t *testing.T, database *store, statement string) int {
	t.Helper()
	var value int
	if err := database.db.QueryRow(statement).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
