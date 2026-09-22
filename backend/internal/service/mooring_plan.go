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
	"gorm.io/gorm"
)

type MooringPlanService interface {
	List(context.Context, dto.PageQuery) (repository.Page[dto.MooringPlanView], error)
	Get(context.Context, uint) (dto.MooringPlanView, error)
	Create(context.Context, dto.CreateMooringPlan, string, string) (dto.MooringPlanView, error)
	Update(context.Context, uint, dto.UpdateMooringPlan, string, string) (dto.MooringPlanView, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (dto.MooringPlanView, error)
	Approve(context.Context, uint, dto.ApproveMooringPlan, string, string) (dto.MooringPlanView, error)
	CheckOccupancy(context.Context, dto.CheckOccupancyRequest) (dto.OccupancyCheckView, error)
	Occupancies(context.Context, dto.OccupancyQuery) (repository.Page[model.BerthOccupancy], error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
}

type mooringPlanService struct {
	repository          repository.MooringPlanRepository
	occupancyRepository repository.BerthOccupancyRepository
	windowRepository    repository.WeatherWindowRepository
	security            SecurityService
	db                  *gorm.DB
	berthLocks          *keyedMutex
}

func NewMooringPlanService(
	db *gorm.DB,
	repo repository.MooringPlanRepository,
	occupancy repository.BerthOccupancyRepository,
	windows repository.WeatherWindowRepository,
	security SecurityService,
) MooringPlanService {
	return &mooringPlanService{
		repository: repo, occupancyRepository: occupancy, windowRepository: windows,
		security: security, db: db, berthLocks: newKeyedMutex(),
	}
}

func (s *mooringPlanService) List(ctx context.Context, query dto.PageQuery) (repository.Page[dto.MooringPlanView], error) {
	return s.listViews(ctx, query)
}

func (s *mooringPlanService) Get(ctx context.Context, id uint) (dto.MooringPlanView, error) {
	return s.getView(ctx, id)
}

func (s *mooringPlanService) Create(ctx context.Context, input dto.CreateMooringPlan, actor, requestID string) (dto.MooringPlanView, error) {
	if err := validateMooringPlanBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return dto.MooringPlanView{}, err
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
		return dto.MooringPlanView{}, fmt.Errorf("create 系泊方案: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "MooringPlan", item.ID, "", item.Status, "created 系泊方案")
	return s.getView(ctx, item.ID)
}

func (s *mooringPlanService) Update(ctx context.Context, id uint, input dto.UpdateMooringPlan, actor, requestID string) (dto.MooringPlanView, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	if err := validateMooringPlanBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return dto.MooringPlanView{}, err
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
		return dto.MooringPlanView{}, fmt.Errorf("update 系泊方案: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "MooringPlan", id, current.Status, current.Status, "updated business fields")
	return s.getView(ctx, id)
}

func (s *mooringPlanService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (dto.MooringPlanView, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	target := strings.TrimSpace(input.Status)
	if !constants.CanTransition(constants.MooringPlanTransitions, current.Status, target) {
		return dto.MooringPlanView{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}

	// Approval carries berth slot + window inputs and must go through Approve.
	if target == string(constants.PlanApprovedState()) {
		return dto.MooringPlanView{}, fmt.Errorf("%w: approval requires berth, slot and weather window via approve action", ErrInvalidInput)
	}

	// Leaving approved (withdraw to review or replacement) releases occupancy.
	if current.Status == string(constants.PlanApprovedState()) {
		return s.transitionWithRelease(ctx, current, input, actor, requestID, target)
	}

	before := current.Status
	current.Status = target
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return dto.MooringPlanView{}, fmt.Errorf("transition 系泊方案: %w", err)
	}
	if err := s.security.Audit(ctx, actor, requestID, "transition", "MooringPlan", id, before, target, input.Reason); err != nil {
		return dto.MooringPlanView{}, fmt.Errorf("persist transition audit: %w", err)
	}
	return s.getView(ctx, id)
}

func (s *mooringPlanService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	// An approved plan still holds a berth: soft-delete and release in one
	// transaction so the occupancy board cannot leak the slot.
	if current.Status == string(constants.PlanApprovedState()) {
		unlock := s.berthLocks.lock(current.Berth)
		defer unlock()
		return s.serializableTx(ctx, func(tx *gorm.DB) error {
			if err := tx.First(&current, id).Error; err != nil {
				return err
			}
			if _, _, err := s.releaseApprovedOccupancy(ctx, tx, current, actor, requestID, "plan_deleted"); err != nil {
				return err
			}
			if err := s.repository.DeleteTx(ctx, tx, id); err != nil {
				return err
			}
			return s.appendAuditTx(ctx, tx, actor, requestID, "delete", id, current.Status, "deleted", "soft deleted approved 系泊方案 and released berth occupancy", current.WindowVersion)
		})
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "MooringPlan", id, current.Status, "deleted", "soft deleted 系泊方案")
}

func (s *mooringPlanService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func validateMooringPlanBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
