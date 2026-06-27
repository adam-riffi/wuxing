package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func adminStore(t *testing.T) *DB {
	t.Helper()
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "wuxing.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestServiceIndex_RecordGetList(t *testing.T) {
	db := adminStore(t)
	ctx := context.Background()

	if err := db.RecordService(ctx, ServiceRecord{ServiceID: "mtg", Name: "mtg", Version: "1", Status: "live", CfgJSON: `{"name":"mtg"}`}); err != nil {
		t.Fatalf("RecordService: %v", err)
	}
	if err := db.RecordService(ctx, ServiceRecord{ServiceID: "notifier", Name: "notifier", Status: "live", CfgJSON: `{}`}); err != nil {
		t.Fatal(err)
	}

	rec, err := db.GetService(ctx, "mtg")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if rec.Name != "mtg" || rec.Version != "1" || rec.Status != "live" || rec.CfgJSON != `{"name":"mtg"}` {
		t.Errorf("record: %+v", rec)
	}
	if rec.RegisteredAt == "" {
		t.Error("RegisteredAt should default to now")
	}

	list, err := db.ListServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ServiceID != "mtg" || list[1].ServiceID != "notifier" {
		t.Errorf("list (ordered by id): %+v", list)
	}
}

func TestServiceIndex_Remove(t *testing.T) {
	db := adminStore(t)
	ctx := context.Background()
	_ = db.RecordService(ctx, ServiceRecord{ServiceID: "mtg", Name: "mtg", Status: "live", CfgJSON: "{}"})

	if err := db.RemoveService(ctx, "mtg"); err != nil {
		t.Fatalf("RemoveService: %v", err)
	}
	if _, err := db.GetService(ctx, "mtg"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound after remove, got %v", err)
	}
	if err := db.RemoveService(ctx, "mtg"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("removing an absent service should return ErrServiceNotFound, got %v", err)
	}
}

func TestServiceIndex_Errors(t *testing.T) {
	db := adminStore(t)
	ctx := context.Background()
	if err := db.RecordService(ctx, ServiceRecord{Name: "x"}); err == nil {
		t.Error("expected error recording a service with no id")
	}
	if _, err := db.GetService(ctx, "ghost"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("get unknown: got %v want ErrServiceNotFound", err)
	}
}
