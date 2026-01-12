package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/fly-io/162719/pkg/errors"
	_ "modernc.org/sqlite"
)

// Repository provides database operations for images
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new repository
func NewRepository(dbPath string) (*Repository, error) {
	slog.Info("database_init", "db_path", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("database_open_failed", "db_path", dbPath, "error", err)
		return nil, errors.Wrap(err, "failed to open database")
	}

	// Create schema
	slog.Info("database_create_schema", "db_path", dbPath)
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		slog.Error("database_schema_failed", "db_path", dbPath, "error", err)
		return nil, errors.Wrap(err, "failed to create schema")
	}

	slog.Info("database_ready", "db_path", dbPath)
	return &Repository{db: db}, nil
}

// Close closes the database connection
func (r *Repository) Close() error {
	return r.db.Close()
}

// Create inserts a new image record
func (r *Repository) Create(img *Image) error {
	slog.Info("database_create_image", "s3_key", img.S3Key, "status", img.Status)

	query := `
		INSERT INTO images (s3_key, sha256, status, device_path, base_device_id, snapshot_id, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Exec(query,
		img.S3Key, img.SHA256, img.Status,
		img.DevicePath, img.BaseDeviceID, img.SnapshotID, img.ErrorMessage)
	if err != nil {
		slog.Error("database_insert_failed", "s3_key", img.S3Key, "error", err)
		return errors.Wrap(err, "failed to insert image")
	}

	id, err := result.LastInsertId()
	if err != nil {
		slog.Error("database_last_insert_id_failed", "s3_key", img.S3Key, "error", err)
		return errors.Wrap(err, "failed to get last insert id")
	}
	img.ID = id

	slog.Info("database_image_created", "s3_key", img.S3Key, "image_id", img.ID, "status", img.Status)
	return nil
}

// GetByS3Key retrieves an image by S3 key
func (r *Repository) GetByS3Key(s3Key string) (*Image, error) {
	slog.Info("database_query_image", "s3_key", s3Key)

	query := `
		SELECT id, s3_key, sha256, status,
		       device_path, base_device_id, snapshot_id, error_message, created_at, updated_at
		FROM images WHERE s3_key = ?
	`
	var img Image
	var devicePath, errorMessage sql.NullString
	var baseDeviceID sql.NullInt64
	var snapshotID sql.NullInt64

	err := r.db.QueryRow(query, s3Key).Scan(
		&img.ID, &img.S3Key, &img.SHA256, &img.Status,
		&devicePath, &baseDeviceID, &snapshotID, &errorMessage,
		&img.CreatedAt, &img.UpdatedAt)

	if err == sql.ErrNoRows {
		slog.Info("database_image_not_found", "s3_key", s3Key)
		return nil, nil // Not found
	}
	if err != nil {
		slog.Error("database_query_failed", "s3_key", s3Key, "error", err)
		return nil, errors.Wrap(err, "failed to query image")
	}

	// Handle nullable fields
	img.DevicePath = devicePath.String
	img.BaseDeviceID = int(baseDeviceID.Int64)
	img.SnapshotID = int(snapshotID.Int64)
	img.ErrorMessage = errorMessage.String

	slog.Info("database_image_found", "s3_key", s3Key, "image_id", img.ID, "status", img.Status)
	return &img, nil
}

// Update updates an existing image record
func (r *Repository) Update(img *Image) error {
	slog.Info("database_update_image", "image_id", img.ID, "s3_key", img.S3Key, "status", img.Status)

	query := `
		UPDATE images
		SET sha256 = ?, status = ?,
		    device_path = ?, base_device_id = ?, snapshot_id = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	result, err := r.db.Exec(query,
		img.SHA256, img.Status,
		img.DevicePath, img.BaseDeviceID, img.SnapshotID, img.ErrorMessage, img.ID)
	if err != nil {
		slog.Error("database_update_failed", "image_id", img.ID, "s3_key", img.S3Key, "error", err)
		return errors.Wrap(err, "failed to update image")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		slog.Error("database_rows_affected_failed", "image_id", img.ID, "error", err)
		return errors.Wrap(err, "failed to get rows affected")
	}
	if rows == 0 {
		slog.Error("database_image_not_found_for_update", "image_id", img.ID)
		return fmt.Errorf("image not found: id=%d", img.ID)
	}

	slog.Info("database_image_updated", "image_id", img.ID, "s3_key", img.S3Key, "status", img.Status)
	return nil
}

// UpdateStatus updates only the status field
func (r *Repository) UpdateStatus(id int64, status, errorMessage string) error {
	slog.Info("database_update_status", "image_id", id, "status", status)

	query := `UPDATE images SET status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := r.db.Exec(query, status, errorMessage, id)
	if err != nil {
		slog.Error("database_status_update_failed", "image_id", id, "status", status, "error", err)
		return errors.Wrap(err, "failed to update status")
	}

	slog.Info("database_status_updated", "image_id", id, "status", status)
	return nil
}

// List retrieves all images
func (r *Repository) List() ([]*Image, error) {
	slog.Info("database_list_images")

	query := `
		SELECT id, s3_key, sha256, status,
		       device_path, base_device_id, snapshot_id, error_message, created_at, updated_at
		FROM images ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query)
	if err != nil {
		slog.Error("database_list_query_failed", "error", err)
		return nil, errors.Wrap(err, "failed to list images")
	}
	defer rows.Close()

	var images []*Image
	for rows.Next() {
		var img Image
		var devicePath, errorMessage sql.NullString
		var baseDeviceID sql.NullInt64
		var snapshotID sql.NullInt64

		err := rows.Scan(
			&img.ID, &img.S3Key, &img.SHA256, &img.Status,
			&devicePath, &baseDeviceID, &snapshotID, &errorMessage,
			&img.CreatedAt, &img.UpdatedAt)
		if err != nil {
			slog.Error("database_scan_row_failed", "error", err)
			return nil, errors.Wrap(err, "failed to scan row")
		}

		img.DevicePath = devicePath.String
		img.BaseDeviceID = int(baseDeviceID.Int64)
		img.SnapshotID = int(snapshotID.Int64)
		img.ErrorMessage = errorMessage.String

		images = append(images, &img)
	}

	if err := rows.Err(); err != nil {
		slog.Error("database_rows_error", "error", err)
		return nil, errors.Wrap(err, "rows error")
	}

	slog.Info("database_list_complete", "image_count", len(images))
	return images, nil
}

// Delete deletes an image by ID
func (r *Repository) Delete(id int64) error {
	slog.Info("database_delete_image", "image_id", id)

	query := `DELETE FROM images WHERE id = ?`
	_, err := r.db.Exec(query, id)
	if err != nil {
		slog.Error("database_delete_failed", "image_id", id, "error", err)
		return errors.Wrap(err, "failed to delete image")
	}

	slog.Info("database_image_deleted", "image_id", id)
	return nil
}

// AllocateNextDeviceID returns the next available device ID for both base devices and snapshots
func (r *Repository) AllocateNextDeviceID(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed_to_begin_transaction", "error", err)
		return 0, errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	var nextID int
	query := "SELECT next_device_id FROM device_sequence WHERE id = 1"
	err = tx.QueryRowContext(ctx, query).Scan(&nextID)
	if err != nil {
		slog.Error("failed_to_query_device_sequence", "error", err)
		return 0, errors.Wrap(err, "failed to query device sequence")
	}

	updateQuery := "UPDATE device_sequence SET next_device_id = ? WHERE id = 1"
	_, err = tx.ExecContext(ctx, updateQuery, nextID+1)
	if err != nil {
		slog.Error("failed_to_update_device_sequence", "error", err)
		return 0, errors.Wrap(err, "failed to update device sequence")
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed_to_commit_transaction", "error", err)
		return 0, errors.Wrap(err, "failed to commit transaction")
	}

	slog.Info("allocated_device_id", "device_id", nextID, "next_available", nextID+1)
	return nextID, nil
}
