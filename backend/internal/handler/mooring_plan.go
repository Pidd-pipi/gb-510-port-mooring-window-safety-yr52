package handler

import (
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
	resource.DELETE("/:id", middleware.RequireRoles("admin"), h.remove)

	// 泊位时段占用闭环只读视图：当前/历史占用与释放结论。
	group.GET("/berth-occupancies", h.listOccupancies)
	// 批准前冲突预览：同一泊位时段是否空闲。
	group.GET("/berth-occupancies/conflicts", middleware.RequireMinimumRole("operator"), h.previewConflicts)
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
		handleError(c, err)
		return
	}
	util.NoContent(c)
}

func (h *MooringPlanHandler) listOccupancies(c *gin.Context) {
	var query dto.BerthOccupancyQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	result, err := h.service.ListOccupancies(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.Page(c, result.Items, result.Page, result.PageSize, result.Total)
}

func (h *MooringPlanHandler) previewConflicts(c *gin.Context) {
	var query dto.BerthConflictQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	preview, err := h.service.PreviewConflicts(c.Request.Context(), query)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, preview)
}
