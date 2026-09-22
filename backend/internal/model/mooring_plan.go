package model

import "time"

// MooringPlan models 系泊方案 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
//
// Berthing fields (BerthCode/StartAt/EndAt/WindowCode/CurrentOccupancyID) are
// only ever written together with approval: approval requires a berth, a
// mooring interval and a safe 关联风浪窗口, and succeeds only when the berth
// interval is free. CurrentOccupancyID points at the active BerthOccupancy and
// is cleared back to zero when the occupancy is released by 撤回 or 替代.
type MooringPlan struct {
	BaseModel
	Facility    string    `json:"facility" gorm:"size:120;index"`
	Owner       string    `json:"owner" gorm:"size:120;index"`
	Category    string    `json:"category" gorm:"size:80;index"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time `json:"effectiveAt"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64;index"`

	BerthCode          string     `json:"berthCode" gorm:"size:64;index"`
	BerthStartAt       *time.Time `json:"berthStartAt"`
	BerthEndAt         *time.Time `json:"berthEndAt"`
	WindowCode         string     `json:"windowCode" gorm:"size:64;index"`
	CurrentOccupancyID uint       `json:"currentOccupancyId" gorm:"index"`
}

func (item *MooringPlan) GetBase() *BaseModel { return &item.BaseModel }

func (item MooringPlan) TableName() string { return "mooring_plans" }

var MooringPlanInitialStatus = "draft"
