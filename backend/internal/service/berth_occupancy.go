package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"gorm.io/gorm"
)

// serializableTx runs fn inside a transaction at the strictest isolation the
// database supports. PostgreSQL uses SERIALIZABLE; SQLite is a single-writer
// file database whose write transactions are already serializable. Any
// serialization anomaly surfaces as ErrBerthBusy so the caller can return 409.
func (s *mooringPlanService) serializableTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	dialect := s.db.Dialector.Name()
	isolation := sqlSerializableLevel(dialect)
	options := isolationOrDefault(isolation)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if options != nil {
			if err := tx.Exec(setIsolationSQL(dialect)).Error; err != nil {
				return err
			}
		}
		return fn(tx)
	}, options)
	if err != nil && isSerializationFailure(err) {
		return ErrBerthBusy
	}
	return err
}

func (s *mooringPlanService) validateSlot(input dto.BerthSlotFields) (string, time.Time, time.Time, error) {
	berth := strings.TrimSpace(input.Berth)
	start := input.StartAt.UTC()
	end := input.EndAt.UTC()
	if berth == "" || input.WindowCode == "" || input.WindowVersion == 0 {
		return "", time.Time{}, time.Time{}, ErrInvalidInput
	}
	if !start.Before(end) {
		return "", time.Time{}, time.Time{}, fmt.Errorf("%w: berth start must be before end", ErrInvalidInput)
	}
	return berth, start, end, nil
}

// resolveSafeWindow loads the linked weather window and enforces both the
// "window must be safe" rule and optimistic version pinning.
func (s *mooringPlanService) resolveSafeWindow(ctx context.Context, code string, version uint) (model.WeatherWindow, error) {
	window, err := s.windowRepository.GetByCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.WeatherWindow{}, fmt.Errorf("%w: linked weather window was not found", ErrInvalidInput)
		}
		return model.WeatherWindow{}, err
	}
	if window.Status != string(constants.WeatherWindowSafeState()) {
		return model.WeatherWindow{}, fmt.Errorf("%w: window %s is %s", ErrWeatherUnsafe, window.Code, window.Status)
	}
	if window.Version != version {
		return model.WeatherWindow{}, fmt.Errorf("%w: expected v%d", ErrWindowVersion, window.Version)
	}
	return window, nil
}

// Approve implements the berth occupancy closed loop for draft/review ->
// approved. Approval succeeds only when the linked window is safe AND the same
// berth slot is free; otherwise the plan keeps its previous status and no
// occupancy row is created.
func (s *mooringPlanService) Approve(ctx context.Context, id uint, input dto.ApproveMooringPlan, actor, requestID string) (dto.MooringPlanView, error) {
	berth, start, end, err := s.validateSlot(input.BerthSlotFields)
	if err != nil {
		return dto.MooringPlanView{}, err
	}

	unlock := s.berthLocks.lock(berth)
	defer unlock()

	window, err := s.resolveSafeWindow(ctx, input.WindowCode, input.WindowVersion)
	if err != nil {
		return dto.MooringPlanView{}, err
	}

	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	if !constants.CanTransition(constants.MooringPlanTransitions, current.Status, string(constants.PlanApprovedState())) {
		return dto.MooringPlanView{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, constants.PlanApprovedState())
	}

	target := string(constants.PlanApprovedState())
	err = s.serializableTx(ctx, func(tx *gorm.DB) error {
		// Re-read inside the transaction to work against the latest row.
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		if !constants.CanTransition(constants.MooringPlanTransitions, current.Status, target) {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
		}

		conflicts, err := s.occupancyRepository.FindOverlappingActive(ctx, tx, berth, start, end, id)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			slots := make([]ConflictSlot, 0, len(conflicts))
			for _, conflict := range conflicts {
				slots = append(slots, ConflictSlot{
					PlanCode: conflict.PlanCode, Berth: conflict.Berth,
					StartAt: conflict.StartAt.UTC().Format(time.RFC3339),
					EndAt:   conflict.EndAt.UTC().Format(time.RFC3339),
				})
			}
			return &SlotConflictError{Berth: berth, Conflicts: slots}
		}

		// A previously released occupancy of the same plan is replaced atomically.
		prior, priorErr := s.occupancyRepository.ActiveByPlan(ctx, tx, id)
		switch {
		case priorErr == nil:
			if err := s.occupancyRepository.Release(ctx, tx, prior.ID, time.Now().UTC(), actor, "replaced"); err != nil {
				return err
			}
		case errors.Is(priorErr, gorm.ErrRecordNotFound):
			// expected for a fresh approval
		default:
			return priorErr
		}

		now := time.Now().UTC()
		before := current.Status
		current.Status = target
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = now
		current.Berth = berth
		current.BerthStartAt = &start
		current.BerthEndAt = &end
		current.WindowID = window.ID
		current.WindowCode = window.Code
		current.WindowVersion = window.Version
		if err := s.repository.UpdateTx(ctx, tx, id, input.ExpectedVersion, &current); err != nil {
			return err
		}

		occupancy := model.BerthOccupancy{
			Berth: berth, PlanID: id, PlanCode: current.Code,
			StartAt: start, EndAt: end, WindowID: window.ID,
			WindowCode: window.Code, WindowVersion: window.Version,
			Status: model.OccupancyActive, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.occupancyRepository.Create(ctx, tx, &occupancy); err != nil {
			return err
		}
		detail := fmt.Sprintf("approved with berth %s %s~%s against window %s v%d: occupancy #%d acquired",
			berth, start.Format(time.RFC3339), end.Format(time.RFC3339), window.Code, window.Version, occupancy.ID)
		return s.appendAuditTx(ctx, tx, actor, requestID, "plan_approve", id, before, target, detail, window.Version)
	})
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	return s.getView(ctx, id)
}

// releaseApprovedOccupancy releases the active occupancy of a plan inside an
// open transaction and writes the release audit. Returns whether an occupancy
// was actually released.
func (s *mooringPlanService) releaseApprovedOccupancy(ctx context.Context, tx *gorm.DB, plan model.MooringPlan, actor, requestID, reason string) (bool, uint, error) {
	occupancy, err := s.occupancyRepository.ActiveByPlan(ctx, tx, plan.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, 0, nil
		}
		return false, 0, err
	}
	now := time.Now().UTC()
	if err := s.occupancyRepository.Release(ctx, tx, occupancy.ID, now, actor, reason); err != nil {
		return false, 0, err
	}
	detail := fmt.Sprintf("berth %s slot %s~%s released after %s",
		occupancy.Berth, occupancy.StartAt.Format(time.RFC3339), occupancy.EndAt.Format(time.RFC3339), reason)
	if err := s.appendAuditTx(ctx, tx, actor, requestID, "occupancy_release", plan.ID, model.OccupancyActive, model.OccupancyReleased, detail, occupancy.WindowVersion); err != nil {
		return false, 0, err
	}
	return true, occupancy.ID, nil
}

// transitionWithRelease handles approved -> review (withdraw) and
// approved -> superseded (replaced), moving the plan and releasing its berth
// occupancy atomically with audit records.
func (s *mooringPlanService) transitionWithRelease(ctx context.Context, current model.MooringPlan, input dto.TransitionRequest, actor, requestID, target string) (dto.MooringPlanView, error) {
	releaseReason := "withdrawn"
	action := "plan_withdraw"
	if target == "superseded" {
		releaseReason = "superseded"
		action = "plan_supersede"
	}
	unlock := s.berthLocks.lock(current.Berth)
	defer unlock()

	err := s.serializableTx(ctx, func(tx *gorm.DB) error {
		if err := tx.First(&current, current.ID).Error; err != nil {
			return err
		}
		released, _, err := s.releaseApprovedOccupancy(ctx, tx, current, actor, requestID, releaseReason)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		before := current.Status
		current.Status = target
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = now
		if err := s.repository.UpdateTx(ctx, tx, current.ID, input.ExpectedVersion, &current); err != nil {
			return err
		}
		conclusion := "occupancy released"
		if !released {
			conclusion = "no active occupancy found"
		}
		detail := fmt.Sprintf("%s %s -> %s; %s", releaseReason, before, target, conclusion)
		return s.appendAuditTx(ctx, tx, actor, requestID, action, current.ID, before, target, detail, current.WindowVersion)
	})
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	return s.getView(ctx, current.ID)
}

// CheckOccupancy previews whether a berth slot can be approved: linked window
// safe and free of overlapping active occupancies. It never mutates state.
func (s *mooringPlanService) CheckOccupancy(ctx context.Context, input dto.CheckOccupancyRequest) (dto.OccupancyCheckView, error) {
	berth, start, end, err := s.validateSlot(input.BerthSlotFields)
	if err != nil {
		return dto.OccupancyCheckView{}, err
	}
	view := dto.OccupancyCheckView{
		Berth: berth, StartAt: start, EndAt: end,
		WindowCode:    strings.ToUpper(strings.TrimSpace(input.WindowCode)),
		WindowVersion: input.WindowVersion, Conflicts: []model.BerthOccupancy{},
	}
	window, err := s.windowRepository.GetByCode(ctx, view.WindowCode)
	switch {
	case err == nil:
		view.WindowStatus = window.Status
		view.WindowSafe = window.Status == string(constants.WeatherWindowSafeState()) && window.Version == input.WindowVersion
		view.WindowVersion = window.Version
	case errors.Is(err, gorm.ErrRecordNotFound):
		view.WindowSafe = false
	default:
		return dto.OccupancyCheckView{}, err
	}
	conflicts, err := s.occupancyRepository.FindOverlappingActive(ctx, nil, berth, start, end, 0)
	if err != nil {
		return dto.OccupancyCheckView{}, err
	}
	view.Conflicts = conflicts
	view.Available = view.WindowSafe && len(conflicts) == 0
	return view, nil
}

// Occupancies backs the berth occupancy board with pagination.
func (s *mooringPlanService) Occupancies(ctx context.Context, query dto.OccupancyQuery) (repository.Page[model.BerthOccupancy], error) {
	return s.occupancyRepository.List(ctx, query)
}

func (s *mooringPlanService) getView(ctx context.Context, id uint) (dto.MooringPlanView, error) {
	plan, err := s.repository.Get(ctx, id)
	if err != nil {
		return dto.MooringPlanView{}, err
	}
	occupancy, err := s.occupancyRepository.ActiveByPlan(ctx, nil, id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.MooringPlanView{}, err
	}
	view := dto.MooringPlanView{MooringPlan: plan}
	if err == nil {
		view.Occupancy = &occupancy
	}
	return view, nil
}

func (s *mooringPlanService) listViews(ctx context.Context, query dto.PageQuery) (repository.Page[dto.MooringPlanView], error) {
	page, err := s.repository.List(ctx, query)
	if err != nil {
		return repository.Page[dto.MooringPlanView]{}, err
	}
	ids := make([]uint, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	active, err := s.occupancyRepository.ListActiveByPlans(ctx, ids)
	if err != nil {
		return repository.Page[dto.MooringPlanView]{}, err
	}
	byPlan := make(map[uint]model.BerthOccupancy, len(active))
	for _, occupancy := range active {
		byPlan[occupancy.PlanID] = occupancy
	}
	views := make([]dto.MooringPlanView, 0, len(page.Items))
	for _, item := range page.Items {
		view := dto.MooringPlanView{MooringPlan: item}
		if occupancy, ok := byPlan[item.ID]; ok {
			occupancy := occupancy
			view.Occupancy = &occupancy
		}
		views = append(views, view)
	}
	return repository.Page[dto.MooringPlanView]{Items: views, Total: page.Total, Page: page.Page, PageSize: page.PageSize}, nil
}

// appendAuditTx writes an audit row inside the occupancy transaction so a
// released occupancy and its audit conclusion cannot diverge.
func (s *mooringPlanService) appendAuditTx(ctx context.Context, tx *gorm.DB, actor, requestID, action string, entityID uint, before, after, detail string, windowVersion uint) error {
	if strings.TrimSpace(actor) == "" {
		actor = "system"
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = "untracked"
	}
	return tx.WithContext(ctx).Create(&model.AuditLog{
		Actor: actor, RequestID: requestID, Action: action, EntityType: "MooringPlan",
		EntityID: entityID, BeforeState: before, AfterState: after,
		Detail: detail, WindowVersion: windowVersion, CreatedAt: time.Now().UTC(),
	}).Error
}
