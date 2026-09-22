package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MooringPlanService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.MooringPlan], error)
	Get(context.Context, uint) (model.MooringPlan, error)
	Create(context.Context, dto.CreateMooringPlan, string, string) (model.MooringPlan, error)
	Update(context.Context, uint, dto.UpdateMooringPlan, string, string) (model.MooringPlan, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.MooringPlan, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	ListOccupancies(context.Context, dto.BerthOccupancyQuery) (repository.Page[model.BerthOccupancy], error)
	PreviewConflicts(context.Context, dto.BerthConflictQuery) (BerthConflictPreview, error)
}

// BerthConflictPreview is the read-only answer used by the plan page to show
// 冲突时段 before an approval is submitted.
type BerthConflictPreview struct {
	Berth     string                 `json:"berth"`
	StartAt   time.Time              `json:"startAt"`
	EndAt     time.Time              `json:"endAt"`
	Free      bool                   `json:"free"`
	Conflicts []model.BerthOccupancy `json:"conflicts"`
}

type mooringPlanService struct {
	repository  repository.MooringPlanRepository
	occupancies repository.BerthOccupancyRepository
	windows     repository.WeatherWindowRepository
	security    SecurityService
}

func NewMooringPlanService(repo repository.MooringPlanRepository, occupancies repository.BerthOccupancyRepository, windows repository.WeatherWindowRepository, security SecurityService) MooringPlanService {
	return &mooringPlanService{repository: repo, occupancies: occupancies, windows: windows, security: security}
}

func (s *mooringPlanService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.MooringPlan], error) {
	return s.repository.List(ctx, query)
}

func (s *mooringPlanService) Get(ctx context.Context, id uint) (model.MooringPlan, error) {
	return s.repository.Get(ctx, id)
}

func (s *mooringPlanService) Create(ctx context.Context, input dto.CreateMooringPlan, actor, requestID string) (model.MooringPlan, error) {
	if err := validateMooringPlanBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.MooringPlan{}, err
	}
	item := model.MooringPlan{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.MooringPlanInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.MooringPlan{}, fmt.Errorf("create 系泊方案: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "MooringPlan", item.ID, "", item.Status, "created 系泊方案")
	return item, nil
}

func (s *mooringPlanService) Update(ctx context.Context, id uint, input dto.UpdateMooringPlan, actor, requestID string) (model.MooringPlan, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.MooringPlan{}, err
	}
	if err := validateMooringPlanBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.MooringPlan{}, err
	}
	// An approved plan holds a live berth occupancy: berthing allocation is
	// immutable through the generic edit form and can only change via
	// 撤回/替代 followed by a new approval cycle.
	if current.Status == "approved" {
		return model.MooringPlan{}, fmt.Errorf("%w: approved plan must be withdrawn or superseded before editing", ErrInvalidTransition)
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.MooringPlan{}, fmt.Errorf("update 系泊方案: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "MooringPlan", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

// berthAllocation is the validated set of fields an approval pins onto the
// plan and its occupancy.
type berthAllocation struct {
	BerthCode  string
	StartAt    time.Time
	EndAt      time.Time
	WindowCode string
}

// resolveAllocation prefers fields carried by the approval request and falls
// back to the allocation frozen on the plan (used when re-approving a
// superseded plan).
func resolveAllocation(input dto.TransitionRequest, current model.MooringPlan) (berthAllocation, error) {
	allocation := berthAllocation{
		BerthCode:  strings.ToUpper(strings.TrimSpace(input.BerthCode)),
		StartAt:    input.StartAt.UTC(),
		EndAt:      input.EndAt.UTC(),
		WindowCode: strings.ToUpper(strings.TrimSpace(input.WindowCode)),
	}
	if allocation.BerthCode == "" {
		allocation.BerthCode = current.BerthCode
	}
	if allocation.WindowCode == "" {
		allocation.WindowCode = current.WindowCode
	}
	if allocation.StartAt.IsZero() && current.BerthStartAt != nil {
		allocation.StartAt = current.BerthStartAt.UTC()
	}
	if allocation.EndAt.IsZero() && current.BerthEndAt != nil {
		allocation.EndAt = current.BerthEndAt.UTC()
	}
	if allocation.BerthCode == "" || allocation.StartAt.IsZero() || allocation.EndAt.IsZero() || allocation.WindowCode == "" {
		return berthAllocation{}, ErrBerthMissing
	}
	if !allocation.EndAt.After(allocation.StartAt) {
		return berthAllocation{}, ErrBerthTimeRange
	}
	return allocation, nil
}

func (s *mooringPlanService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.MooringPlan, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.MooringPlan{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.MooringPlanTransitions, current.Status, target) {
		return model.MooringPlan{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	switch {
	case target == "approved":
		return s.approveWithOccupancy(ctx, current, input, actor, requestID)
	case current.Status == "approved" && current.CurrentOccupancyID != 0:
		// 撤回 (approved -> review) and 替代 (approved -> superseded) release
		// the berth occupancy in the same transaction as the state change.
		return s.releaseOnTransition(ctx, current, input, actor, requestID)
	default:
		return s.plainTransition(ctx, current, input, actor, requestID)
	}
}

func (s *mooringPlanService) plainTransition(ctx context.Context, current model.MooringPlan, input dto.TransitionRequest, actor, requestID string) (model.MooringPlan, error) {
	before := current.Status
	current.Status = input.Status
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, current.ID, input.ExpectedVersion, &current); err != nil {
		return model.MooringPlan{}, fmt.Errorf("transition 系泊方案: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "MooringPlan", current.ID, before, input.Status, input.Reason); err != nil {
		return model.MooringPlan{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.repository.Get(ctx, current.ID)
}

// approveWithOccupancy runs the closed-loop approval: berth serial lock,
// safe-window check, overlap check, plan update and occupancy insert all commit
// together. Any failure rolls back so the plan keeps its original state and no
// occupancy is left behind.
func (s *mooringPlanService) approveWithOccupancy(ctx context.Context, current model.MooringPlan, input dto.TransitionRequest, actor, requestID string) (model.MooringPlan, error) {
	allocation, err := resolveAllocation(input, current)
	if err != nil {
		return model.MooringPlan{}, err
	}
	err = s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.occupancies.AcquireBerthLock(ctx, tx, allocation.BerthCode); err != nil {
			return fmt.Errorf("acquire berth lock: %w", err)
		}
		locked, err := s.repository.GetTx(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		if locked.Version != input.ExpectedVersion || locked.Status != current.Status {
			return repository.ErrVersionConflict
		}
		window, err := s.windows.GetByCodeForUpdate(ctx, tx, allocation.WindowCode)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrBerthWindowUnsafe
			}
			return fmt.Errorf("load related weather window: %w", err)
		}
		if window.Status != "safe" {
			return ErrBerthWindowUnsafe
		}
		conflicts, err := s.occupancies.FindOverlappingActive(ctx, tx, allocation.BerthCode, allocation.StartAt, allocation.EndAt, current.ID)
		if err != nil {
			return fmt.Errorf("check berth overlap: %w", err)
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("%w: %s 与已批准方案 %s 时段重叠", ErrBerthOccupied, allocation.BerthCode, conflicts[0].PlanCode)
		}
		now := time.Now().UTC()
		occupancy := model.BerthOccupancy{
			Code:       "OCC-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
			Status:     string(constants.OccupancyStateActive),
			Version:    1,
			BerthCode:  allocation.BerthCode,
			StartAt:    allocation.StartAt,
			EndAt:      allocation.EndAt,
			PlanID:     locked.ID,
			PlanCode:   locked.Code,
			WindowCode: allocation.WindowCode,
			AcquiredBy: actor,
			AcquiredAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := s.occupancies.Create(ctx, tx, &occupancy); err != nil {
			return fmt.Errorf("create berth occupancy: %w", err)
		}
		before := locked.Status
		locked.Status = "approved"
		locked.BerthCode = allocation.BerthCode
		locked.BerthStartAt = &allocation.StartAt
		locked.BerthEndAt = &allocation.EndAt
		locked.WindowCode = allocation.WindowCode
		locked.CurrentOccupancyID = occupancy.ID
		locked.Version = input.ExpectedVersion + 1
		locked.UpdatedAt = now
		if err := s.repository.UpdateTx(ctx, tx, locked.ID, input.ExpectedVersion, &locked); err != nil {
			return fmt.Errorf("approve 系泊方案: %w", err)
		}
		if err := s.appendAuditTx(ctx, tx, actor, requestID, "transition", "MooringPlan", locked.ID, before, "approved", input.Reason); err != nil {
			return err
		}
		detail := fmt.Sprintf("approved %s with berth %s %s~%s under safe window %s",
			locked.Code, allocation.BerthCode,
			allocation.StartAt.Format(time.RFC3339), allocation.EndAt.Format(time.RFC3339), allocation.WindowCode)
		if err := s.appendAuditTx(ctx, tx, actor, requestID, "occupancy_acquire", "BerthOccupancy", occupancy.ID, "", string(constants.OccupancyStateActive), detail); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return model.MooringPlan{}, err
	}
	return s.repository.Get(ctx, current.ID)
}

func (s *mooringPlanService) releaseOnTransition(ctx context.Context, current model.MooringPlan, input dto.TransitionRequest, actor, requestID string) (model.MooringPlan, error) {
	err := s.repository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.occupancies.AcquireBerthLock(ctx, tx, current.BerthCode); err != nil {
			return fmt.Errorf("acquire berth lock: %w", err)
		}
		locked, err := s.repository.GetTx(ctx, tx, current.ID)
		if err != nil {
			return err
		}
		if locked.Version != input.ExpectedVersion || locked.Status != "approved" || locked.CurrentOccupancyID != current.CurrentOccupancyID {
			return repository.ErrVersionConflict
		}
		existing, err := s.occupancies.Get(ctx, locked.CurrentOccupancyID)
		if err != nil {
			return fmt.Errorf("load active occupancy: %w", err)
		}
		rows, err := s.occupancies.Release(ctx, tx, existing.ID, existing.Version, actor, input.Reason)
		if err != nil {
			return fmt.Errorf("release berth occupancy: %w", err)
		}
		if rows == 0 {
			return repository.ErrVersionConflict
		}
		now := time.Now().UTC()
		before := locked.Status
		locked.Status = input.Status
		locked.CurrentOccupancyID = 0
		locked.Version = input.ExpectedVersion + 1
		locked.UpdatedAt = now
		if err := s.repository.UpdateTx(ctx, tx, locked.ID, input.ExpectedVersion, &locked); err != nil {
			return fmt.Errorf("transition 系泊方案: %w", err)
		}
		if err := s.appendAuditTx(ctx, tx, actor, requestID, "transition", "MooringPlan", locked.ID, before, input.Status, input.Reason); err != nil {
			return err
		}
		action := "occupancy_release"
		if input.Status == "superseded" {
			action = "occupancy_release_supersede"
		}
		detail := fmt.Sprintf("%s -> %s released berth %s %s~%s: %s",
			before, input.Status, existing.BerthCode,
			existing.StartAt.Format(time.RFC3339), existing.EndAt.Format(time.RFC3339), input.Reason)
		if err := s.appendAuditTx(ctx, tx, actor, requestID, action, "BerthOccupancy", existing.ID, string(constants.OccupancyStateActive), string(constants.OccupancyStateReleased), detail); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return model.MooringPlan{}, err
	}
	return s.repository.Get(ctx, current.ID)
}

func (s *mooringPlanService) appendAuditTx(ctx context.Context, tx *gorm.DB, actor, requestID, action, entityType string, entityID uint, before, after, detail string) error {
	return s.security.AuditInTx(ctx, tx, actor, requestID, action, entityType, entityID, before, after, detail)
}

func (s *mooringPlanService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Status == "approved" && current.CurrentOccupancyID != 0 {
		return fmt.Errorf("%w: withdraw or supersede the approved plan before deletion", ErrInvalidTransition)
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "MooringPlan", id, current.Status, "deleted", "soft deleted 系泊方案")
}

func (s *mooringPlanService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func (s *mooringPlanService) ListOccupancies(ctx context.Context, query dto.BerthOccupancyQuery) (repository.Page[model.BerthOccupancy], error) {
	return s.occupancies.List(ctx, query)
}

func (s *mooringPlanService) PreviewConflicts(ctx context.Context, query dto.BerthConflictQuery) (BerthConflictPreview, error) {
	berth := strings.ToUpper(strings.TrimSpace(query.Berth))
	start := query.StartAt.UTC()
	end := query.EndAt.UTC()
	if berth == "" {
		return BerthConflictPreview{}, ErrBerthMissing
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return BerthConflictPreview{}, ErrBerthTimeRange
	}
	conflicts, err := s.occupancies.FindOverlappingActive(ctx, s.repository.DB(), berth, start, end, 0)
	if err != nil {
		return BerthConflictPreview{}, fmt.Errorf("preview berth conflicts: %w", err)
	}
	return BerthConflictPreview{
		Berth: berth, StartAt: start, EndAt: end, Free: len(conflicts) == 0, Conflicts: conflicts,
	}, nil
}

func validateMooringPlanBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
