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
	Create(context.Context, *model.MooringPlan) error
	Update(context.Context, uint, uint, *model.MooringPlan) error
	UpdateTx(context.Context, *gorm.DB, uint, uint, *model.MooringPlan) error
	Delete(context.Context, uint) error
	DeleteTx(context.Context, *gorm.DB, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type mooringPlanRepository struct {
	store *Store[model.MooringPlan]
	db    *gorm.DB
}

func NewMooringPlanRepository(db *gorm.DB) MooringPlanRepository {
	return &mooringPlanRepository{store: NewStore[model.MooringPlan](db), db: db}
}

func (r *mooringPlanRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.MooringPlan], error) {
	return r.store.List(ctx, q)
}
func (r *mooringPlanRepository) Get(ctx context.Context, id uint) (model.MooringPlan, error) {
	return r.store.Get(ctx, id)
}
func (r *mooringPlanRepository) Create(ctx context.Context, item *model.MooringPlan) error {
	return r.store.Create(ctx, item)
}
func (r *mooringPlanRepository) Update(ctx context.Context, id, version uint, item *model.MooringPlan) error {
	return r.store.Update(ctx, id, version, item)
}

// UpdateTx performs the same optimistic-lock update as Update but on a caller
// supplied transaction (used by the berth occupancy closed loop).
func (r *mooringPlanRepository) UpdateTx(ctx context.Context, tx *gorm.DB, id, expectedVersion uint, item *model.MooringPlan) error {
	if tx == nil {
		return r.store.Update(ctx, id, expectedVersion, item)
	}
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

// DeleteTx soft-deletes the plan inside a transaction so an approved plan and
// its berth occupancy are released atomically.
func (r *mooringPlanRepository) DeleteTx(ctx context.Context, tx *gorm.DB, id uint) error {
	if tx == nil {
		return r.store.Delete(ctx, id)
	}
	result := tx.WithContext(ctx).Delete(&model.MooringPlan{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *mooringPlanRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
