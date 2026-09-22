package model

import "time"

// BerthOccupancy 是系泊方案批准后对具体泊位时段的占用闭环实体。一条 approved
// 方案在同一时间最多对应一条 active 记录；方案撤回或被替代时记录保留并标记为
// released，从而让释放结论与审计可以刷新后回读。
type BerthOccupancy struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	Berth         string     `json:"berth" gorm:"size:120;not null;index:idx_berth_occupancy_active,priority:1;index:idx_berth_occupancy_window,priority:1"`
	PlanID        uint       `json:"planId" gorm:"not null;index"`
	PlanCode      string     `json:"planCode" gorm:"size:64;not null;index"`
	StartAt       time.Time  `json:"startAt" gorm:"not null;index:idx_berth_occupancy_window,priority:2"`
	EndAt         time.Time  `json:"endAt" gorm:"not null;index:idx_berth_occupancy_window,priority:3"`
	WindowID      uint       `json:"windowId" gorm:"not null"`
	WindowCode    string     `json:"windowCode" gorm:"size:64;not null;index"`
	WindowVersion uint       `json:"windowVersion" gorm:"not null"`
	Status        string     `json:"status" gorm:"size:24;not null;index:idx_berth_occupancy_active,priority:2"`
	ReleasedAt    *time.Time `json:"releasedAt"`
	ReleasedBy    string     `json:"releasedBy" gorm:"size:80"`
	ReleaseReason string     `json:"releaseReason" gorm:"size:80"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (item BerthOccupancy) TableName() string { return "berth_occupancies" }

const (
	// OccupancyActive 表示泊位时段仍被已批准方案占用。
	OccupancyActive = "active"
	// OccupancyReleased 表示占用已随方案撤回或替代而释放。
	OccupancyReleased = "released"
)
