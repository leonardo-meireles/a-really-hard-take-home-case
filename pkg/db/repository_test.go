package db

import (
	"os"
	"testing"
)

func TestRepository_CreateAndGet(t *testing.T) {
	dbPath := "/tmp/test_images.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	img := &Image{
		S3Key:  "test-image.tar",
		SHA256: "abc123",
		Status: StatusPending,
	}

	if err := repo.Create(img); err != nil {
		t.Fatalf("failed to create image: %v", err)
	}

	retrieved, err := repo.GetByS3Key("test-image.tar")
	if err != nil {
		t.Fatalf("failed to get image: %v", err)
	}

	if retrieved.S3Key != img.S3Key || retrieved.SHA256 != img.SHA256 {
		t.Errorf("retrieved image mismatch: got %+v, want %+v", retrieved, img)
	}
}

func TestRepository_UpdateStatus(t *testing.T) {
	dbPath := "/tmp/test_images2.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	img := &Image{
		S3Key:  "test-image.tar",
		SHA256: "abc123",
		Status: StatusPending,
	}
	repo.Create(img)

	if err := repo.UpdateStatus(img.ID, StatusDownloading, ""); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, _ := repo.GetByS3Key("test-image.tar")
	if updated.Status != StatusDownloading {
		t.Errorf("status not updated: got %s, want %s", updated.Status, StatusDownloading)
	}
}

func TestRepository_List(t *testing.T) {
	dbPath := "/tmp/test_images3.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	repo.Create(&Image{S3Key: "image1.tar", SHA256: "hash1", Status: StatusReady})
	repo.Create(&Image{S3Key: "image2.tar", SHA256: "hash2", Status: StatusFailed})

	images, err := repo.List()
	if err != nil {
		t.Fatalf("failed to list images: %v", err)
	}

	if len(images) != 2 {
		t.Errorf("expected 2 images, got %d", len(images))
	}
}
