package repositories

import (
	"machdown/server/models"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestJobRepository_CreateAndFind(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	db.AutoMigrate(&models.DownloadJob{})

	repo := NewJobRepository(db)
	job := &models.DownloadJob{
		ID:     "test-job-123",
		URL:    "http://example.com/test.zip",
		Status: "Pending",
	}

	if err := repo.CreateJob(job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	savedJob, err := repo.FindJobByURL("http://example.com/test.zip")
	if err != nil {
		t.Fatalf("failed to find job: %v", err)
	}
	if savedJob.ID != job.ID {
		t.Errorf("expected job ID %s, got %s", job.ID, savedJob.ID)
	}
}
