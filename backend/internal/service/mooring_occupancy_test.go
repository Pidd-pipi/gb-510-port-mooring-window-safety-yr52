package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/config"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type berthFixture struct {
	db        *gorm.DB
	svc       MooringPlanService
	plans     []model.MooringPlan
	windows   []model.WeatherWindow
	security  SecurityService
	occupancy repository.BerthOccupancyRepository
}

func newBerthFixture(t *testing.T) berthFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:berth-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	// Single connection pool serializes writes exactly like the SQLite file lock
	// and makes the concurrent-approval assertion deterministic.
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.MooringPlan{}, &model.WeatherWindow{}, &model.BerthOccupancy{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	planRepo := repository.NewMooringPlanRepository(db)
	windowRepo := repository.NewWeatherWindowRepository(db)
	occupancyRepo := repository.NewBerthOccupancyRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewMooringPlanService(db, planRepo, occupancyRepo, windowRepo, security)

	now := time.Now().UTC().Truncate(time.Hour)
	windows := []model.WeatherWindow{
		{BaseModel: model.BaseModel{Code: "WW-SAFE", Name: "safe window", Status: "safe", Version: 1}, EffectiveAt: now},
		{BaseModel: model.BaseModel{Code: "WW-REST", Name: "restricted window", Status: "restricted", Version: 1}, EffectiveAt: now},
	}
	if err := db.Create(&windows).Error; err != nil {
		t.Fatalf("create windows: %v", err)
	}
	plans := []model.MooringPlan{
		{BaseModel: model.BaseModel{Code: "MP-A", Name: "plan alpha", Status: "review", Version: 1}, Facility: "zone", Owner: "ops"},
		{BaseModel: model.BaseModel{Code: "MP-B", Name: "plan bravo", Status: "review", Version: 1}, Facility: "zone", Owner: "ops"},
		{BaseModel: model.BaseModel{Code: "MP-C", Name: "plan charlie", Status: "draft", Version: 1}, Facility: "zone", Owner: "ops"},
	}
	if err := db.Create(&plans).Error; err != nil {
		t.Fatalf("create plans: %v", err)
	}
	return berthFixture{db: db, svc: svc, plans: plans, windows: windows, security: security, occupancy: occupancyRepo}
}

func approveInput(berth string, start time.Time, duration time.Duration) dto.ApproveMooringPlan {
	return dto.ApproveMooringPlan{
		BerthSlotFields: dto.BerthSlotFields{
			Berth: berth, StartAt: start, EndAt: start.Add(duration),
			WindowCode: "WW-SAFE", WindowVersion: 1,
		},
		ExpectedVersion: 1, Reason: "submit plan for approval with berth slot",
	}
}

func TestApproveAcquiresOccupancyWhenWindowSafeAndSlotFree(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)

	view, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-01", start, 4*time.Hour), "operator", "req-approve")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if view.Status != "approved" || view.Version != 2 {
		t.Fatalf("unexpected approved plan: %+v", view.MooringPlan)
	}
	if view.Occupancy == nil || view.Occupancy.Status != model.OccupancyActive || view.Occupancy.Berth != "B-01" {
		t.Fatalf("occupancy missing after approval: %+v", view.Occupancy)
	}
	if view.WindowCode != "WW-SAFE" || view.WindowVersion != 1 || view.Berth != "B-01" {
		t.Fatalf("plan did not freeze slot/window: %+v", view.MooringPlan)
	}

	// Refresh read path returns the same occupancy.
	reloaded, err := fixture.svc.Get(ctx, fixture.plans[0].ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Occupancy == nil || reloaded.Occupancy.ID != view.Occupancy.ID {
		t.Fatalf("occupancy not readable after refresh: %+v", reloaded.Occupancy)
	}
}

func TestApproveRejectedWhenWindowUnsafeLeavesNoOccupancy(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)
	input := approveInput("B-01", start, 4*time.Hour)
	input.WindowCode = "WW-REST"
	before, _ := fixture.svc.Get(ctx, fixture.plans[0].ID)

	_, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, input, "operator", "req-unsafe")
	if !errors.Is(err, ErrWeatherUnsafe) {
		t.Fatalf("expected unsafe window error, got %v", err)
	}
	after, _ := fixture.svc.Get(ctx, fixture.plans[0].ID)
	if after.Status != before.Status || after.Version != before.Version {
		t.Fatalf("plan changed after failed approval: before=%+v after=%+v", before.MooringPlan, after.MooringPlan)
	}
	if after.Occupancy != nil {
		t.Fatalf("occupancy must not exist after unsafe-window rejection: %+v", after.Occupancy)
	}
	var count int64
	if err := fixture.db.Model(&model.BerthOccupancy{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expected zero occupancy rows, count=%d err=%v", count, err)
	}
}

func TestApproveRejectedWhenWindowVersionChanged(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)
	input := approveInput("B-01", start, 4*time.Hour)
	input.WindowVersion = 99
	_, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, input, "operator", "req-version")
	if !errors.Is(err, ErrWindowVersion) {
		t.Fatalf("expected window version error, got %v", err)
	}
}

func TestApproveRejectsInvalidTimeRange(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)
	input := approveInput("B-01", start, 0)
	input.EndAt = start.Add(-time.Hour)
	_, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, input, "operator", "req-range")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for inverted range, got %v", err)
	}
}

func TestApproveRejectedOnOverlappingSlotWithConflictDetails(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)

	if _, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-01", start, 6*time.Hour), "operator", "req-first"); err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Adjacent (touching) slot is allowed; overlapping slot is not.
	overlap := approveInput("B-01", start.Add(5*time.Hour), 3*time.Hour)
	_, err := fixture.svc.Approve(ctx, fixture.plans[1].ID, overlap, "operator", "req-conflict")
	var conflict *SlotConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected slot conflict, got %v", err)
	}
	if conflict.Berth != "B-01" || len(conflict.Conflicts) != 1 || conflict.Conflicts[0].PlanCode != "MP-A" {
		t.Fatalf("unexpected conflict details: %+v", conflict)
	}
	second, _ := fixture.svc.Get(ctx, fixture.plans[1].ID)
	if second.Status != "review" || second.Occupancy != nil {
		t.Fatalf("rejected plan must stay review with no occupancy: %+v", second)
	}

	// Same time window on a different berth is free.
	other := approveInput("B-02", start.Add(5*time.Hour), 3*time.Hour)
	if _, err := fixture.svc.Approve(ctx, fixture.plans[2].ID, other, "operator", "req-other-berth"); err != nil {
		t.Fatalf("different berth should approve, got %v", err)
	}

	// Half-open intervals: exactly touching ranges do not overlap.
	touch := dto.ApproveMooringPlan{
		BerthSlotFields: dto.BerthSlotFields{Berth: "B-01", StartAt: start.Add(6 * time.Hour), EndAt: start.Add(8 * time.Hour), WindowCode: "WW-SAFE", WindowVersion: 1},
		ExpectedVersion: 1, Reason: "adjacent slot",
	}
	if _, err := fixture.svc.Approve(ctx, fixture.plans[1].ID, touch, "operator", "req-touch"); err != nil {
		t.Fatalf("adjacent (non-overlapping) slot should approve, got %v", err)
	}
}

func TestConcurrentApprovalsSameSlotOnlyOneSucceeds(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)

	var wg sync.WaitGroup
	startGate := make(chan struct{})
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-startGate
		_, results[0] = fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-09", start, 4*time.Hour), "operator", "req-c1")
	}()
	go func() {
		defer wg.Done()
		<-startGate
		_, results[1] = fixture.svc.Approve(ctx, fixture.plans[1].ID, approveInput("B-09", start, 4*time.Hour), "operator", "req-c2")
	}()
	close(startGate)
	wg.Wait()

	successes, conflicts := 0, 0
	for _, result := range results {
		var conflict *SlotConflictError
		switch {
		case result == nil:
			successes++
		case errors.As(result, &conflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected exactly one success and one conflict, successes=%d conflicts=%d (%v %v)", successes, conflicts, results[0], results[1])
	}
	var active int64
	if err := fixture.db.Model(&model.BerthOccupancy{}).Where("status = ?", model.OccupancyActive).Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("expected one active occupancy, got %d (err=%v)", active, err)
	}
}

func TestWithdrawAndSupersedeReleaseOccupancyWithAudit(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)

	approved, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-05", start, 4*time.Hour), "operator", "req-app")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Withdraw approved -> review releases the slot.
	withdrawn, err := fixture.svc.Transition(ctx, fixture.plans[0].ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: approved.Version, Reason: "withdraw plan for revision",
	}, "reviewer", "req-withdraw")
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if withdrawn.Status != "review" || withdrawn.Occupancy != nil {
		t.Fatalf("withdrawn plan should have no active occupancy: %+v", withdrawn)
	}
	var releasedOccupancy model.BerthOccupancy
	if err := fixture.db.Where("plan_id = ?", fixture.plans[0].ID).First(&releasedOccupancy).Error; err != nil {
		t.Fatalf("released occupancy row missing: %v", err)
	}
	if releasedOccupancy.Status != model.OccupancyReleased || releasedOccupancy.ReleasedBy != "reviewer" || releasedOccupancy.ReleaseReason != "withdrawn" || releasedOccupancy.ReleasedAt == nil {
		t.Fatalf("unexpected released occupancy: %+v", releasedOccupancy)
	}

	// The freed slot can be acquired by another plan now.
	if _, err := fixture.svc.Approve(ctx, fixture.plans[1].ID, approveInput("B-05", start, 4*time.Hour), "operator", "req-takeover"); err != nil {
		t.Fatalf("released slot should be approvable, got %v", err)
	}

	// Supersede an approved plan and verify release reason.
	second, _ := fixture.svc.Get(ctx, fixture.plans[1].ID)
	superseded, err := fixture.svc.Transition(ctx, fixture.plans[1].ID, dto.TransitionRequest{
		Status: "superseded", ExpectedVersion: second.Version, Reason: "replaced by newer plan",
	}, "admin", "req-supersede")
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if superseded.Status != "superseded" {
		t.Fatalf("unexpected status: %s", superseded.Status)
	}
	var supersededOccupancy model.BerthOccupancy
	if err := fixture.db.Where("plan_id = ? AND status = ?", fixture.plans[1].ID, model.OccupancyReleased).First(&supersededOccupancy).Error; err != nil {
		t.Fatalf("superseded release row missing: %v", err)
	}
	if supersededOccupancy.ReleaseReason != "superseded" {
		t.Fatalf("expected superseded release reason, got %s", supersededOccupancy.ReleaseReason)
	}

	logs, total, err := fixture.security.ListAudits(ctx, 1, 50, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if total < 5 {
		t.Fatalf("expected approval/release audits, got total=%d", total)
	}
	actions := map[string]bool{}
	for _, log := range logs {
		actions[log.Action] = true
		if log.RequestID == "" || log.Actor == "" {
			t.Fatalf("audit missing context: %+v", log)
		}
	}
	for _, action := range []string{"plan_approve", "plan_withdraw", "occupancy_release", "plan_supersede"} {
		if !actions[action] {
			t.Fatalf("missing audit action %s in %v", action, actions)
		}
	}
}

func TestCheckOccupancyPreviewsConflictAndAvailability(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)
	fields := dto.BerthSlotFields{Berth: "B-07", StartAt: start, EndAt: start.Add(3 * time.Hour), WindowCode: "WW-SAFE", WindowVersion: 1}

	free, err := fixture.svc.CheckOccupancy(ctx, dto.CheckOccupancyRequest{BerthSlotFields: fields})
	if err != nil || !free.Available || !free.WindowSafe || len(free.Conflicts) != 0 {
		t.Fatalf("expected free preview, view=%+v err=%v", free, err)
	}
	if _, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-07", start, 5*time.Hour), "operator", "req-seed"); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	busy, err := fixture.svc.CheckOccupancy(ctx, dto.CheckOccupancyRequest{BerthSlotFields: fields})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if busy.Available || len(busy.Conflicts) != 1 {
		t.Fatalf("expected busy preview with one conflict: %+v", busy)
	}
	restricted := fields
	restricted.Berth = "B-08"
	restricted.WindowCode = "WW-REST"
	unsafe, err := fixture.svc.CheckOccupancy(ctx, dto.CheckOccupancyRequest{BerthSlotFields: restricted})
	if err != nil {
		t.Fatalf("check restricted: %v", err)
	}
	if unsafe.Available || unsafe.WindowSafe {
		t.Fatalf("restricted window must be unsafe: %+v", unsafe)
	}
}

func TestApproveWithdrawReapproveKeepsFullReleaseHistory(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)

	first, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-21", start, 4*time.Hour), "operator", "req-cycle-a1")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}
	first, err = fixture.svc.Transition(ctx, fixture.plans[0].ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: first.Version, Reason: "first withdraw",
	}, "operator", "req-cycle-w1")
	if err != nil {
		t.Fatalf("first withdraw: %v", err)
	}

	second, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, dto.ApproveMooringPlan{
		BerthSlotFields: dto.BerthSlotFields{
			Berth: "B-21", StartAt: start.Add(8 * time.Hour), EndAt: start.Add(12 * time.Hour),
			WindowCode: "WW-SAFE", WindowVersion: 1,
		},
		ExpectedVersion: first.Version, Reason: "re-approve after revision",
	}, "operator", "req-cycle-a2")
	if err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	if _, err := fixture.svc.Transition(ctx, fixture.plans[0].ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: second.Version, Reason: "second withdraw",
	}, "reviewer", "req-cycle-w2"); err != nil {
		t.Fatalf("second withdraw: %v", err)
	}

	var released []model.BerthOccupancy
	if err := fixture.db.Where("plan_id = ? AND status = ?", fixture.plans[0].ID, model.OccupancyReleased).Find(&released).Error; err != nil {
		t.Fatalf("query released history: %v", err)
	}
	if len(released) != 2 {
		t.Fatalf("expected two released occupancy rows across the cycle, got %d", len(released))
	}
	var activeCount int64
	if err := fixture.db.Model(&model.BerthOccupancy{}).Where("plan_id = ? AND status = ?", fixture.plans[0].ID, model.OccupancyActive).Count(&activeCount).Error; err != nil || activeCount != 0 {
		t.Fatalf("expected zero active rows, count=%d err=%v", activeCount, err)
	}

	// Occupancies board exposes both release conclusions.
	page, err := fixture.svc.Occupancies(ctx, dto.OccupancyQuery{Status: model.OccupancyReleased, PageSize: 10})
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	if page.Total < 2 {
		t.Fatalf("expected released board to retain history, total=%d", page.Total)
	}
}

func TestGenericTransitionCannotApproveWithoutBerthSlot(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	_, err := fixture.svc.Transition(ctx, fixture.plans[0].ID, dto.TransitionRequest{
		Status: "approved", ExpectedVersion: 1, Reason: "trying shortcut approval",
	}, "operator", "req-shortcut")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for generic approval, got %v", err)
	}
}

func TestOccupanciesBoardListsActiveAndReleased(t *testing.T) {
	fixture := newBerthFixture(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Hour)
	approved, err := fixture.svc.Approve(ctx, fixture.plans[0].ID, approveInput("B-11", start, 4*time.Hour), "operator", "req-board-1")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := fixture.svc.Transition(ctx, fixture.plans[0].ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: approved.Version, Reason: "withdraw for board",
	}, "reviewer", "req-board-2"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}

	active, err := fixture.svc.Occupancies(ctx, dto.OccupancyQuery{Status: model.OccupancyActive})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if active.Total != 0 {
		t.Fatalf("expected no active rows, got %d", active.Total)
	}
	released, err := fixture.svc.Occupancies(ctx, dto.OccupancyQuery{Status: model.OccupancyReleased})
	if err != nil {
		t.Fatalf("list released: %v", err)
	}
	if released.Total != 1 || released.Items[0].Berth != "B-11" {
		t.Fatalf("expected one released row, got %+v", released)
	}
}
