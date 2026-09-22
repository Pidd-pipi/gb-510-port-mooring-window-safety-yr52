package model

import "time"

// MooringPlan models 系泊方案 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
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

	// 批准时固化的泊位时段与关联风浪窗口；未批准或已撤回/替代前可能为空。
	Berth         string     `json:"berth" gorm:"size:120;index"`
	BerthStartAt  *time.Time `json:"berthStartAt"`
	BerthEndAt    *time.Time `json:"berthEndAt"`
	WindowID      uint       `json:"windowId" gorm:"index"`
	WindowCode    string     `json:"windowCode" gorm:"size:64;index"`
	WindowVersion uint       `json:"windowVersion"`
}

func (item *MooringPlan) GetBase() *BaseModel { return &item.BaseModel }

func (item MooringPlan) TableName() string { return "mooring_plans" }

var MooringPlanInitialStatus = "draft"
