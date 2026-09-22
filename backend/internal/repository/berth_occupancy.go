package repository

import (
	"context"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BerthOccupancyRepository owns persistence for the 泊位时段占用 closed loop.
// Methods taking *gorm.DB expect either the root handle or an in-flight
// transaction, which lets approval and release hold a berth serial lock while
// checking overlaps and writing the occupancy.
type BerthOccupancyRepository interface {
	AcquireBerthLock(context.Context, *gorm.DB, string) error
	FindOverlappingActive(context.Context, *gorm.DB, string, time.Time, time.Time, uint) ([]model.BerthOccupancy, error)
	Create(context.Context, *gorm.DB, *model.BerthOccupancy) error
	Get(context.Context, uint) (model.BerthOccupancy, error)
	Release(context.Context, *gorm.DB, uint, uint, string, string) (int64, error)
	List(context.Context, dto.BerthOccupancyQuery) (Page[model.BerthOccupancy], error)
}

type berthOccupancyRepository struct {
	db *gorm.DB
}

func NewBerthOccupancyRepository(db *gorm.DB) BerthOccupancyRepository {
	return &berthOccupancyRepository{db: db}
}

// AcquireBerthLock ensures a serial-lock row exists for the berth and takes a
// row lock on it for the duration of tx. Two transactions approving the same
// berth therefore run their overlap checks serially; different berths use
// different lock rows. OnConflict do-nothing keeps the first-wins insert
// portable across PostgreSQL, MySQL and SQLite.
func (r *berthOccupancyRepository) AcquireBerthLock(ctx context.Context, tx *gorm.DB, berth string) error {
	now := time.Now().UTC()
	lock := model.BerthSerialLock{BerthCode: berth, CreatedAt: now, UpdatedAt: now}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	query := tx.WithContext(ctx).Where("berth_code = ?", berth)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var held model.BerthSerialLock
	return query.First(&held).Error
}

// FindOverlappingActive returns active occupancies of one berth whose interval
// overlaps [start, end). Touching endpoints are not a conflict
// (existing.end == new.start is allowed). excludePlanID lets release/re-approve
// flows ignore a plan's own occupancy.
func (r *berthOccupancyRepository) FindOverlappingActive(ctx context.Context, tx *gorm.DB, berth string, start, end time.Time, excludePlanID uint) ([]model.BerthOccupancy, error) {
	items := make([]model.BerthOccupancy, 0)
	query := tx.WithContext(ctx).
		Where("berth_code = ? AND status = ? AND start_at < ? AND end_at > ?",
			berth, string(constants.OccupancyStateActive), end, start)
	if excludePlanID > 0 {
		query = query.Where("plan_id <> ?", excludePlanID)
	}
	err := query.Order("start_at ASC, id ASC").Find(&items).Error
	return items, err
}

func (r *berthOccupancyRepository) Create(ctx context.Context, tx *gorm.DB, item *model.BerthOccupancy) error {
	return tx.WithContext(ctx).Create(item).Error
}

func (r *berthOccupancyRepository) Get(ctx context.Context, id uint) (model.BerthOccupancy, error) {
	var item model.BerthOccupancy
	err := r.db.WithContext(ctx).First(&item, id).Error
	return item, err
}

// Release flips one occupancy active -> released with optimistic locking. It
// returns the number of rows changed (0 means the occupancy was missing, no
// longer active, or the version changed).
func (r *berthOccupancyRepository) Release(ctx context.Context, tx *gorm.DB, id, expectedVersion uint, actor, reason string) (int64, error) {
	now := time.Now().UTC()
	result := tx.WithContext(ctx).Model(&model.BerthOccupancy{}).
		Where("id = ? AND version = ? AND status = ?", id, expectedVersion, string(constants.OccupancyStateActive)).
		Updates(map[string]any{
			"status":         string(constants.OccupancyStateReleased),
			"version":        gorm.Expr("version + 1"),
			"released_by":    actor,
			"released_at":    now,
			"release_reason": reason,
			"updated_at":     now,
		})
	return result.RowsAffected, result.Error
}

func (r *berthOccupancyRepository) List(ctx context.Context, q dto.BerthOccupancyQuery) (Page[model.BerthOccupancy], error) {
	page, pageSize := normalizePage(q.Page, q.PageSize)
	db := r.db.WithContext(ctx).Model(&model.BerthOccupancy{})
	if status := strings.TrimSpace(q.Status); status != "" {
		db = db.Where("status = ?", status)
	}
	if berth := strings.TrimSpace(q.Berth); berth != "" {
		db = db.Where("berth_code = ?", berth)
	}
	if q.PlanID > 0 {
		db = db.Where("plan_id = ?", q.PlanID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return Page[model.BerthOccupancy]{}, err
	}
	items := make([]model.BerthOccupancy, 0)
	err := db.Order("start_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	return Page[model.BerthOccupancy]{Items: items, Total: total, Page: page, PageSize: pageSize}, err
}
