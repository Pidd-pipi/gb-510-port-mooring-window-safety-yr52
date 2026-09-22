package dto

import (
	"time"

	"github.com/blueship581/port-mooring-window-safety/backend/internal/model"
)

// BerthSlotFields 是批准方案与占用预检共用的泊位时段输入契约。
type BerthSlotFields struct {
	Berth         string    `json:"berth" binding:"required,min=2,max=120"`
	StartAt       time.Time `json:"startAt" binding:"required"`
	EndAt         time.Time `json:"endAt" binding:"required"`
	WindowCode    string    `json:"windowCode" binding:"required,min=2,max=64"`
	WindowVersion uint      `json:"windowVersion" binding:"required"`
}

// ApproveMooringPlan 是系泊方案提交批准时的专用契约：必须填写泊位、靠泊起止
// 时间和关联风浪窗口（编码与当前版本）。状态机仍只允许 draft/review -> approved。
type ApproveMooringPlan struct {
	BerthSlotFields
	ExpectedVersion uint   `json:"expectedVersion" binding:"required"`
	Reason          string `json:"reason" binding:"required,min=3,max=500"`
}

// CheckOccupancyRequest 用于在批准前预检同一泊位的时段冲突，不产生任何占用。
type CheckOccupancyRequest struct {
	BerthSlotFields
}

// OccupancyQuery 是泊位占用看板的查询条件。
type OccupancyQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Berth    string `form:"berth"`
	Status   string `form:"status"`
}

// MooringPlanView 在系泊方案实体之外附带当前占用与释放结论，列表和详情共用，
// 保证页面刷新后仍能回读占用闭环状态。
type MooringPlanView struct {
	model.MooringPlan
	Occupancy *model.BerthOccupancy `json:"occupancy"`
}

// OccupancyCheckView 是占用预检结论。仅当窗口安全且无同泊位时段重叠时
// available 为 true；冲突列表给出占用同一泊位时段的已批准方案。
type OccupancyCheckView struct {
	Available     bool                   `json:"available"`
	Berth         string                 `json:"berth"`
	StartAt       time.Time              `json:"startAt"`
	EndAt         time.Time              `json:"endAt"`
	WindowCode    string                 `json:"windowCode"`
	WindowStatus  string                 `json:"windowStatus"`
	WindowVersion uint                   `json:"windowVersion"`
	WindowSafe    bool                   `json:"windowSafe"`
	Conflicts     []model.BerthOccupancy `json:"conflicts"`
}
