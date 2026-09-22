package handler

import (
	"errors"
	"net/http"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/dto"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/middleware"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/service"
	"github.com/blueship581/port-mooring-window-safety/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type MooringPlanHandler struct{ service service.MooringPlanService }

func NewMooringPlanHandler(s service.MooringPlanService) *MooringPlanHandler {
	return &MooringPlanHandler{service: s}
}

func (h *MooringPlanHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/plans")
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("", middleware.RequireMinimumRole("operator"), h.create)
	resource.PUT("/:id", middleware.RequireMinimumRole("operator"), h.update)
	resource.POST("/:id/transition", middleware.RequireMinimumRole("operator"), h.transition)
	resource.POST("/:id/approve", middleware.RequireMinimumRole("operator"), h.approve)
	resource.POST("/:id/occupancy-check", middleware.RequireMinimumRole("operator"), h.checkOccupancy)
	resource.DELETE("/:id", middleware.RequireRoles("admin"), h.remove)

	group.GET("/berth-occupancies", h.listOccupancies)
}

func (h *MooringPlanHandler) list(c *gin.Context) {
	query := bindPage(c)
	result, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

func (h *MooringPlanHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *MooringPlanHandler) create(c *gin.Context) {
	var input dto.CreateMooringPlan
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Create(c.Request.Context(), input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, item)
}

func (h *MooringPlanHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.UpdateMooringPlan
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *MooringPlanHandler) transition(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.TransitionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Transition(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *MooringPlanHandler) approve(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.ApproveMooringPlan
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.service.Approve(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleSlotConflict(c, err)
		if c.IsAborted() {
			return
		}
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *MooringPlanHandler) checkOccupancy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.CheckOccupancyRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service.CheckOccupancy(c.Request.Context(), input)
	if err != nil {
		handleError(c, err)
		return
	}
	// Echo the plan id so the frontend can correlate the preview row.
	util.OK(c, gin.H{"planId": id, "result": view})
}

func (h *MooringPlanHandler) listOccupancies(c *gin.Context) {
	var query dto.OccupancyQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service.Occupancies(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

func (h *MooringPlanHandler) remove(c *gin.Context) {
	if roleFromContext(c) != "admin" {
		util.Fail(c, http.StatusForbidden, "forbidden", "admin role is required")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, actorFromContext(c), requestIDFromContext(c)); err != nil {
		handleSlotConflict(c, err)
		if c.IsAborted() {
			return
		}
		handleError(c, err)
		return
	}
	util.NoContent(c)
}

// handleSlotConflict maps the typed slot conflict to a 409 with the
// overlapping slots so the plan page can show 冲突时段 without any partial
// occupancy being persisted.
func handleSlotConflict(c *gin.Context, err error) {
	var conflict *service.SlotConflictError
	if errors.As(err, &conflict) {
		util.FailDetails(c, http.StatusConflict, "berth_slot_conflict", err.Error(), gin.H{
			"berth":     conflict.Berth,
			"conflicts": conflict.Conflicts,
		})
	}
}
