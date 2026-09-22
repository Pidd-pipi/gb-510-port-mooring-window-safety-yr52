package repository

import (
	"context"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"gorm.io/gorm"
)

// BerthOccupancyRepository owns 泊位时段占用 persistence. All mutating calls
// accept the *gorm.DB handle so the service can run them inside a serializable
// transaction that makes "same berth + overlapping time" approvals safe under
// concurrency.
type BerthOccupancyRepository interface {
	Create(ctx context.Context, tx *gorm.DB, item *model.BerthOccupancy) error
	ActiveByPlan(ctx context.Context, tx *gorm.DB, planID uint) (model.BerthOccupancy, error)
	ListActiveByPlans(ctx context.Context, planIDs []uint) ([]model.BerthOccupancy, error)
	Release(ctx context.Context, tx *gorm.DB, id uint, releasedAt time.Time, releasedBy, reason string) error
	List(ctx context.Context, query dto.OccupancyQuery) (Page[model.BerthOccupancy], error)
	// FindOverlappingActive returns active occupancies of the same berth whose
	// [start,end) intersects [start,end). excludePlanID allows re-approval of
	// the same plan to ignore its own stale record.
	FindOverlappingActive(ctx context.Context, tx *gorm.DB, berth string, start, end time.Time, excludePlanID uint) ([]model.BerthOccupancy, error)
}

type berthOccupancyRepository struct {
	db *gorm.DB
}

func NewBerthOccupancyRepository(db *gorm.DB) BerthOccupancyRepository {
	return &berthOccupancyRepository{db: db}
}

func (r *berthOccupancyRepository) conn(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *berthOccupancyRepository) Create(ctx context.Context, tx *gorm.DB, item *model.BerthOccupancy) error {
	return r.conn(ctx, tx).Create(item).Error
}

func (r *berthOccupancyRepository) ActiveByPlan(ctx context.Context, tx *gorm.DB, planID uint) (model.BerthOccupancy, error) {
	var item model.BerthOccupancy
	err := r.conn(ctx, tx).
		Where("plan_id = ? AND status = ?", planID, model.OccupancyActive).
		First(&item).Error
	return item, err
}

func (r *berthOccupancyRepository) ListActiveByPlans(ctx context.Context, planIDs []uint) ([]model.BerthOccupancy, error) {
	items := make([]model.BerthOccupancy, 0)
	if len(planIDs) == 0 {
		return items, nil
	}
	err := r.db.WithContext(ctx).
		Where("status = ? AND plan_id IN ?", model.OccupancyActive, planIDs).
		Find(&items).Error
	return items, err
}

func (r *berthOccupancyRepository) Release(ctx context.Context, tx *gorm.DB, id uint, releasedAt time.Time, releasedBy, reason string) error {
	result := r.conn(ctx, tx).Model(&model.BerthOccupancy{}).
		Where("id = ? AND status = ?", id, model.OccupancyActive).
		Updates(map[string]any{
			"status":         model.OccupancyReleased,
			"released_at":    releasedAt,
			"released_by":    releasedBy,
			"release_reason": reason,
			"updated_at":     releasedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *berthOccupancyRepository) List(ctx context.Context, query dto.OccupancyQuery) (Page[model.BerthOccupancy], error) {
	page, pageSize := normalizePage(query.Page, query.PageSize)
	db := r.db.WithContext(ctx).Model(&model.BerthOccupancy{})
	if berth := query.Berth; berth != "" {
		db = db.Where("berth = ?", berth)
	}
	status := query.Status
	if status != model.OccupancyActive && status != model.OccupancyReleased {
		status = model.OccupancyActive
	}
	db = db.Where("status = ?", status)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return Page[model.BerthOccupancy]{}, err
	}
	items := make([]model.BerthOccupancy, 0)
	err := db.Order("start_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return Page[model.BerthOccupancy]{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}

func (r *berthOccupancyRepository) FindOverlappingActive(ctx context.Context, tx *gorm.DB, berth string, start, end time.Time, excludePlanID uint) ([]model.BerthOccupancy, error) {
	items := make([]model.BerthOccupancy, 0)
	db := r.conn(ctx, tx).
		Where("status = ?", model.OccupancyActive).
		Where("berth = ?", berth).
		Where("start_at < ? AND end_at > ?", end, start)
	if excludePlanID != 0 {
		db = db.Where("plan_id <> ?", excludePlanID)
	}
	err := db.Order("start_at ASC").Find(&items).Error
	return items, err
}
