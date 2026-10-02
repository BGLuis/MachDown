package repositories

import (
	"machdown/server/models"
	"gorm.io/gorm"
)

type JobRepository struct {
	db *gorm.DB
}

func NewJobRepository(db *gorm.DB) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) CreateJob(job *models.DownloadJob) error {
	return r.db.Create(job).Error
}

func (r *JobRepository) UpdateJobStatus(id, status string) error {
	return r.db.Model(&models.DownloadJob{}).Where("id = ?", id).Update("status", status).Error
}

func (r *JobRepository) UpdateJobFileName(id, fileName string) error {
	return r.db.Model(&models.DownloadJob{}).Where("id = ?", id).Update("file_name", fileName).Error
}

func (r *JobRepository) FindJobByURL(url string) (*models.DownloadJob, error) {
	var job models.DownloadJob
	err := r.db.Where("url = ?", url).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *JobRepository) FindByURL(url string) (*models.DownloadJob, error) {
	var job models.DownloadJob
	err := r.db.Where("url = ?", url).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *JobRepository) FindJobByID(id string) (*models.DownloadJob, error) {
	var job models.DownloadJob
	err := r.db.Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (r *JobRepository) SaveChunk(chunk *models.ChunkTask) error {
	return r.db.Save(chunk).Error
}

func (r *JobRepository) FindChunksByJob(jobID string) ([]models.ChunkTask, error) {
	var chunks []models.ChunkTask
	err := r.db.Where("job_id = ?", jobID).Find(&chunks).Error
	return chunks, err
}

// FindJobByETag busca um job existente pelo ETag do arquivo para deduplicação.
func (r *JobRepository) FindJobByETag(etag string) (*models.DownloadJob, error) {
	if etag == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var job models.DownloadJob
	err := r.db.Where("e_tag = ? AND status != ?", etag, "Error").First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateRootHash persiste o RootHash da Merkle Tree no banco de dados.
func (r *JobRepository) UpdateRootHash(id, rootHash string) error {
	return r.db.Model(&models.DownloadJob{}).Where("id = ?", id).Update("root_hash", rootHash).Error
}

func (r *JobRepository) FindCompletedJobsByClient(clientID string) ([]models.DownloadJob, error) {
	var jobs []models.DownloadJob
	err := r.db.Where("status = ? AND target_clients LIKE ?", "Completed", "%"+clientID+"%").Find(&jobs).Error
	return jobs, err
}

func (r *JobRepository) FindAllJobs(limit, offset int) ([]models.DownloadJob, error) {
	var jobs []models.DownloadJob
	err := r.db.Order("created_at desc").Limit(limit).Offset(offset).Find(&jobs).Error
	return jobs, err
}

