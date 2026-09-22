package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/config"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/constants"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type mooringFixture struct {
	db       *gorm.DB
	svc      MooringPlanService
	security SecurityService
}

func newMooringFixture(t *testing.T) mooringFixture {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "mooring.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.MooringPlan{}, &model.WeatherWindow{}, &model.BerthOccupancy{},
		&model.BerthSerialLock{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	planRepo := repository.NewMooringPlanRepository(db)
	occupancyRepo := repository.NewBerthOccupancyRepository(db)
	windowRepo := repository.NewWeatherWindowRepository(db)
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	svc := NewMooringPlanService(planRepo, occupancyRepo, windowRepo, security)
	return mooringFixture{db: db, svc: svc, security: security}
}

func (f mooringFixture) seedWindow(t *testing.T, code, status string) {
	t.Helper()
	window := model.WeatherWindow{
		BaseModel: model.BaseModel{Code: code, Name: "window " + code, Status: status, Version: 1},
		Facility:  "Berth zone", Owner: "operations", Category: "test", RiskLevel: "low",
		EffectiveAt: time.Now().UTC(),
	}
	if err := f.db.Create(&window).Error; err != nil {
		t.Fatalf("seed window: %v", err)
	}
}

func (f mooringFixture) seedPlan(t *testing.T, code, status string) model.MooringPlan {
	t.Helper()
	plan := model.MooringPlan{
		BaseModel: model.BaseModel{Code: code, Name: "plan " + code, Status: status, Version: 1},
		Facility:  "Berth zone", Owner: "operations", Category: "test", RiskLevel: "medium",
		EffectiveAt: time.Now().UTC(),
	}
	if err := f.db.Create(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return plan
}

func approvalRequest(berth, window string, start, end time.Time, version uint) dto.TransitionRequest {
	return dto.TransitionRequest{
		Status: "approved", ExpectedVersion: version, Reason: "approve with berth occupancy",
		BerthCode: berth, StartAt: start, EndAt: end, WindowCode: window,
	}
}

func TestApprovalRequiresAllocationAndSafeWindow(t *testing.T) {
	f := newMooringFixture(t)
	ctx := context.Background()
	f.seedWindow(t, "WW-SAFE", "safe")
	f.seedWindow(t, "WW-BAD", "restricted")
	start := time.Now().UTC().Add(2 * time.Hour)
	end := start.Add(4 * time.Hour)
	plan := f.seedPlan(t, "MP-NOALLOC", "review")

	// Missing berth/time/window is rejected and leaves the plan untouched.
	_, err := f.svc.Transition(ctx, plan.ID, dto.TransitionRequest{
		Status: "approved", ExpectedVersion: 1, Reason: "missing allocation",
	}, "operator", "req-missing")
	if !errors.Is(err, ErrBerthMissing) {
		t.Fatalf("expected ErrBerthMissing, got %v", err)
	}
	after, _ := f.svc.Get(ctx, plan.ID)
	if after.Status != "review" || after.Version != 1 || after.CurrentOccupancyID != 0 {
		t.Fatalf("plan changed despite failed approval: %+v", after)
	}
	var occupancyCount int64
	f.db.Model(&model.BerthOccupancy{}).Count(&occupancyCount)
	if occupancyCount != 0 {
		t.Fatalf("expected zero occupancies, got %d", occupancyCount)
	}

	// Related window that is not safe blocks approval.
	_, err = f.svc.Transition(ctx, plan.ID, approvalRequest("B-01", "WW-BAD", start, end, 1), "operator", "req-unsafe")
	if !errors.Is(err, ErrBerthWindowUnsafe) {
		t.Fatalf("expected ErrBerthWindowUnsafe, got %v", err)
	}

	// Unknown window code behaves the same as an unsafe window.
	_, err = f.svc.Transition(ctx, plan.ID, approvalRequest("B-01", "WW-404", start, end, 1), "operator", "req-unknown")
	if !errors.Is(err, ErrBerthWindowUnsafe) {
		t.Fatalf("expected ErrBerthWindowUnsafe for missing window, got %v", err)
	}

	// Reversed interval is rejected.
	_, err = f.svc.Transition(ctx, plan.ID, approvalRequest("B-01", "WW-SAFE", end, start, 1), "operator", "req-range")
	if !errors.Is(err, ErrBerthTimeRange) {
		t.Fatalf("expected ErrBerthTimeRange, got %v", err)
	}
}

func TestApprovalAcquiresOccupancyAndReleaseClosesLoop(t *testing.T) {
	f := newMooringFixture(t)
	ctx := context.Background()
	f.seedWindow(t, "WW-SAFE", "safe")
	plan := f.seedPlan(t, "MP-OK", "review")
	start := time.Now().UTC().Add(2 * time.Hour)
	end := start.Add(4 * time.Hour)

	approved, err := f.svc.Transition(ctx, plan.ID, approvalRequest("B-02", "WW-SAFE", start, end, 1), "reviewer", "req-approve")
	if err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	if approved.Status != "approved" || approved.BerthCode != "B-02" || approved.WindowCode != "WW-SAFE" || approved.CurrentOccupancyID == 0 {
		t.Fatalf("approval did not pin occupancy fields: %+v", approved)
	}

	occupancies, err := f.svc.ListOccupancies(ctx, dto.BerthOccupancyQuery{Berth: "B-02"})
	if err != nil || len(occupancies.Items) != 1 {
		t.Fatalf("expected one occupancy, items=%v err=%v", occupancies.Items, err)
	}
	active := occupancies.Items[0]
	if active.Status != string(constants.OccupancyStateActive) || active.PlanID != plan.ID || active.AcquiredBy != "reviewer" {
		t.Fatalf("unexpected active occupancy: %+v", active)
	}

	// Overlapping approval on the same berth is rejected; touching endpoints free.
	other := f.seedPlan(t, "MP-DUP", "review")
	_, err = f.svc.Transition(ctx, other.ID, approvalRequest("B-02", "WW-SAFE", start.Add(time.Hour), end.Add(time.Hour), 1), "operator", "req-dup")
	if !errors.Is(err, ErrBerthOccupied) {
		t.Fatalf("expected ErrBerthOccupied, got %v", err)
	}
	otherAfter, _ := f.svc.Get(ctx, other.ID)
	if otherAfter.Status != "review" || otherAfter.CurrentOccupancyID != 0 {
		t.Fatalf("conflicting plan must stay unchanged: %+v", otherAfter)
	}

	// Exact endpoint touch (end == existing start) is free and approvable.
	touch := f.seedPlan(t, "MP-TOUCH", "review")
	if _, err := f.svc.Transition(ctx, touch.ID, approvalRequest("B-02", "WW-SAFE", end, end.Add(2*time.Hour), 1), "operator", "req-touch"); err != nil {
		t.Fatalf("back-to-back window should be free: %v", err)
	}

	// Withdraw (撤回, approved -> review) releases the occupancy.
	released, err := f.svc.Transition(ctx, plan.ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: approved.Version, Reason: "withdraw plan for reassessment",
	}, "admin", "req-withdraw")
	if err != nil {
		t.Fatalf("withdraw plan: %v", err)
	}
	if released.Status != "review" || released.CurrentOccupancyID != 0 {
		t.Fatalf("plan should be withdrawn without occupancy pointer: %+v", released)
	}
	updated, err := f.svc.ListOccupancies(ctx, dto.BerthOccupancyQuery{Berth: "B-02", Status: "released"})
	if err != nil || len(updated.Items) != 1 {
		t.Fatalf("expected one released occupancy, items=%v err=%v", updated.Items, err)
	}
	conclusion := updated.Items[0]
	if conclusion.Status != "released" || conclusion.ReleasedBy != "admin" || conclusion.ReleasedAt == nil {
		t.Fatalf("release conclusion missing: %+v", conclusion)
	}

	// The released interval is free again: re-approval succeeds.
	reapproved, err := f.svc.Transition(ctx, plan.ID, approvalRequest("B-02", "WW-SAFE", start, end, released.Version), "reviewer", "req-reapprove")
	if err != nil {
		t.Fatalf("re-approve after release: %v", err)
	}
	if reapproved.Status != "approved" || reapproved.CurrentOccupancyID == 0 {
		t.Fatalf("re-approved plan must hold a fresh occupancy: %+v", reapproved)
	}

	// Supersede (替代) releases the new active occupancy too.
	superseded, err := f.svc.Transition(ctx, plan.ID, dto.TransitionRequest{
		Status: "superseded", ExpectedVersion: reapproved.Version, Reason: "replaced by a newer plan",
	}, "admin", "req-supersede")
	if err != nil {
		t.Fatalf("supersede plan: %v", err)
	}
	if superseded.Status != "superseded" || superseded.CurrentOccupancyID != 0 {
		t.Fatalf("superseded plan should release occupancy: %+v", superseded)
	}
	allOccupancies, _ := f.svc.ListOccupancies(ctx, dto.BerthOccupancyQuery{PageSize: 100})
	activeCount, releasedCount := 0, 0
	for _, item := range allOccupancies.Items {
		switch item.Status {
		case "active":
			activeCount++
		case "released":
			releasedCount++
		}
	}
	if activeCount != 1 || releasedCount != 2 {
		t.Fatalf("expected 1 active (touch plan) and 2 released occupancies, got active=%d released=%d", activeCount, releasedCount)
	}

	// Audit trail covers acquire, withdrawal release, re-acquire, supersede release.
	logs, _, err := f.security.ListAudits(ctx, 1, 100, "BerthOccupancy")
	if err != nil {
		t.Fatalf("list occupancy audits: %v", err)
	}
	actions := map[string]int{}
	for _, entry := range logs {
		if entry.EntityType == "BerthOccupancy" {
			actions[entry.Action]++
			if entry.RequestID == "" || entry.Actor == "" {
				t.Fatalf("occupancy audit lost actor/request context: %+v", entry)
			}
		}
	}
	// 3 acquires: initial MP-OK, the back-to-back MP-TOUCH plan, and MP-OK re-approval.
	if actions["occupancy_acquire"] != 3 || actions["occupancy_release"] != 1 || actions["occupancy_release_supersede"] != 1 {
		t.Fatalf("unexpected occupancy audit actions: %+v", actions)
	}
}

func TestConcurrentApprovalSameSlotOnlyOneSucceeds(t *testing.T) {
	f := newMooringFixture(t)
	ctx := context.Background()
	f.seedWindow(t, "WW-SAFE", "safe")
	plans := make([]model.MooringPlan, 0, 8)
	for i := 0; i < 8; i++ {
		plans = append(plans, f.seedPlan(t, fmt.Sprintf("MP-C%d", i), "review"))
	}
	start := time.Now().UTC().Add(24 * time.Hour)
	end := start.Add(3 * time.Hour)

	var wg sync.WaitGroup
	results := make([]error, len(plans))
	for i, plan := range plans {
		wg.Add(1)
		go func(index int, p model.MooringPlan) {
			defer wg.Done()
			_, results[index] = f.svc.Transition(ctx, p.ID, approvalRequest("B-RACE", "WW-SAFE", start, end, 1), "operator", fmt.Sprintf("req-race-%d", index))
		}(i, plan)
	}
	wg.Wait()

	successes, occupied := 0, 0
	for _, resultErr := range results {
		switch {
		case resultErr == nil:
			successes++
		case errors.Is(resultErr, ErrBerthOccupied):
			occupied++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful approval, got %d (errors=%v)", successes, results)
	}
	if occupied != len(plans)-1 {
		t.Fatalf("expected all losing approvals to report berth occupied, got %d of %d", occupied, len(plans)-1)
	}

	page, err := f.svc.ListOccupancies(ctx, dto.BerthOccupancyQuery{Berth: "B-RACE", Status: "active"})
	if err != nil {
		t.Fatalf("list occupancies: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected a single active occupancy, got %d", page.Total)
	}

	approvedCount := int64(0)
	f.db.Model(&model.MooringPlan{}).Where("status = ? AND berth_code = ?", "approved", "B-RACE").Count(&approvedCount)
	if approvedCount != 1 {
		t.Fatalf("expected exactly one approved plan, got %d", approvedCount)
	}
}

func TestConflictPreview(t *testing.T) {
	f := newMooringFixture(t)
	ctx := context.Background()
	f.seedWindow(t, "WW-SAFE", "safe")
	plan := f.seedPlan(t, "MP-PREVIEW", "review")
	start := time.Now().UTC().Add(2 * time.Hour)
	end := start.Add(4 * time.Hour)
	if _, err := f.svc.Transition(ctx, plan.ID, approvalRequest("B-09", "WW-SAFE", start, end, 1), "operator", "req-seed"); err != nil {
		t.Fatalf("seed occupancy: %v", err)
	}

	free, err := f.svc.PreviewConflicts(ctx, dto.BerthConflictQuery{Berth: "B-09", StartAt: end, EndAt: end.Add(time.Hour)})
	if err != nil || !free.Free || len(free.Conflicts) != 0 {
		t.Fatalf("touching interval should be free: %+v err=%v", free, err)
	}
	busy, err := f.svc.PreviewConflicts(ctx, dto.BerthConflictQuery{Berth: "B-09", StartAt: start.Add(-time.Hour), EndAt: start.Add(time.Hour)})
	if err != nil || busy.Free || len(busy.Conflicts) != 1 {
		t.Fatalf("overlapping interval should report one conflict: %+v err=%v", busy, err)
	}
}
