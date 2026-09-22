package repository

import (
	"context"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"gorm.io/gorm"
)

// MooringPlanRepository owns all persistence operations for 系泊方案.
type MooringPlanRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.MooringPlan], error)
	Get(context.Context, uint) (model.MooringPlan, error)
	GetTx(context.Context, *gorm.DB, uint) (model.MooringPlan, error)
	Create(context.Context, *model.MooringPlan) error
	Update(context.Context, uint, uint, *model.MooringPlan) error
	UpdateTx(context.Context, *gorm.DB, uint, uint, *model.MooringPlan) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	DB() *gorm.DB
}

type mooringPlanRepository struct {
	db    *gorm.DB
	store *Store[model.MooringPlan]
}

func NewMooringPlanRepository(db *gorm.DB) MooringPlanRepository {
	return &mooringPlanRepository{db: db, store: NewStore[model.MooringPlan](db)}
}

func (r *mooringPlanRepository) DB() *gorm.DB { return r.db }

func (r *mooringPlanRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.MooringPlan], error) {
	return r.store.List(ctx, q)
}
func (r *mooringPlanRepository) Get(ctx context.Context, id uint) (model.MooringPlan, error) {
	return r.store.Get(ctx, id)
}
func (r *mooringPlanRepository) GetTx(ctx context.Context, tx *gorm.DB, id uint) (model.MooringPlan, error) {
	var item model.MooringPlan
	err := tx.WithContext(ctx).First(&item, id).Error
	return item, err
}
func (r *mooringPlanRepository) Create(ctx context.Context, item *model.MooringPlan) error {
	return r.store.Create(ctx, item)
}
func (r *mooringPlanRepository) Update(ctx context.Context, id, version uint, item *model.MooringPlan) error {
	return r.store.Update(ctx, id, version, item)
}

// UpdateTx performs the same optimistic-lock update as Update but inside an
// in-flight transaction so approval can atomically pair it with occupancy
// creation and audit writes.
func (r *mooringPlanRepository) UpdateTx(ctx context.Context, tx *gorm.DB, id, expectedVersion uint, item *model.MooringPlan) error {
	result := tx.WithContext(ctx).Model(&model.MooringPlan{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Select("*").Omit("id", "code", "created_at", "deleted_at").Updates(item)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *mooringPlanRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}
func (r *mooringPlanRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
