package model

import "time"

// BerthOccupancy is the closed-loop record of a berth time-slot taken by an
// approved 系泊方案. It is created only when the related 风浪窗口 is safe and the
// berth interval is free, and retained (never deleted) when the plan is
// 撤回 or 替代: status moves active -> released together with release actor,
// reason and timestamp, giving an auditable 释放结论.
type BerthOccupancy struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	Code          string     `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Status        string     `json:"status" gorm:"size:40;index;not null"`
	Version       uint       `json:"version" gorm:"not null;default:1"`
	BerthCode     string     `json:"berthCode" gorm:"size:64;index;not null"`
	StartAt       time.Time  `json:"startAt" gorm:"not null;index:idx_berth_interval,priority:2"`
	EndAt         time.Time  `json:"endAt" gorm:"not null;index:idx_berth_interval,priority:3"`
	PlanID        uint       `json:"planId" gorm:"index;not null"`
	PlanCode      string     `json:"planCode" gorm:"size:64;index;not null"`
	WindowCode    string     `json:"windowCode" gorm:"size:64;index;not null"`
	AcquiredBy    string     `json:"acquiredBy" gorm:"size:80;index;not null"`
	AcquiredAt    time.Time  `json:"acquiredAt" gorm:"not null"`
	ReleasedBy    string     `json:"releasedBy" gorm:"size:80;index"`
	ReleasedAt    *time.Time `json:"releasedAt"`
	ReleaseReason string     `json:"releaseReason" gorm:"size:500"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

func (item BerthOccupancy) TableName() string { return "berth_occupancies" }

var BerthOccupancyInitialStatus = "active"

// BerthSerialLock has one row per berth. Approving a plan locks that row inside
// a transaction before overlap checking, so concurrent approvals for the same
// berth serialize and exactly one can win. Different berths keep independent
// rows and never block each other.
type BerthSerialLock struct {
	BerthCode string    `json:"berthCode" gorm:"primaryKey;size:64"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (item BerthSerialLock) TableName() string { return "berth_serial_locks" }
