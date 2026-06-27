package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrServiceNotFound is returned when a service id is not in the index.
var ErrServiceNotFound = errors.New("storage: service not in index")

// ServiceRecord is one row of the admin service index — declared state for a
// registered service (its lifecycle status and hard-saved canonical cfg).
type ServiceRecord struct {
	ServiceID    string
	Name         string
	Version      string
	Status       string
	CfgJSON      string
	RegisteredAt string
}

// RecordService inserts a service into the index. RegisteredAt defaults to now
// when empty. Re-registration is deregister-then-register (delete + insert).
func (db *DB) RecordService(ctx context.Context, rec ServiceRecord) error {
	if rec.ServiceID == "" {
		return fmt.Errorf("storage: service record has no id")
	}
	if rec.RegisteredAt == "" {
		rec.RegisteredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO wuxing_admin_service (service_id, name, version, status, cfg_json, registered_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
		rec.ServiceID, rec.Name, rec.Version, rec.Status, rec.CfgJSON, rec.RegisteredAt)
	if err != nil {
		return fmt.Errorf("storage: record service %q: %w", rec.ServiceID, err)
	}
	return nil
}

// RemoveService deletes a service from the index.
func (db *DB) RemoveService(ctx context.Context, serviceID string) error {
	res, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM wuxing_admin_service WHERE service_id = ?`), serviceID)
	if err != nil {
		return fmt.Errorf("storage: remove service %q: %w", serviceID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// GetService returns a service's index record.
func (db *DB) GetService(ctx context.Context, serviceID string) (ServiceRecord, error) {
	var rec ServiceRecord
	err := db.QueryRowContext(ctx, db.Rebind(
		`SELECT service_id, name, version, status, cfg_json, registered_at
			FROM wuxing_admin_service WHERE service_id = ?`), serviceID).
		Scan(&rec.ServiceID, &rec.Name, &rec.Version, &rec.Status, &rec.CfgJSON, &rec.RegisteredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceRecord{}, ErrServiceNotFound
	}
	if err != nil {
		return ServiceRecord{}, fmt.Errorf("storage: get service %q: %w", serviceID, err)
	}
	return rec, nil
}

// ListServices enumerates the index, ordered by service id.
func (db *DB) ListServices(ctx context.Context) ([]ServiceRecord, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT service_id, name, version, status, cfg_json, registered_at
			FROM wuxing_admin_service ORDER BY service_id`)
	if err != nil {
		return nil, fmt.Errorf("storage: list services: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ServiceRecord
	for rows.Next() {
		var rec ServiceRecord
		if err := rows.Scan(&rec.ServiceID, &rec.Name, &rec.Version, &rec.Status, &rec.CfgJSON, &rec.RegisteredAt); err != nil {
			return nil, fmt.Errorf("storage: scan service: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
