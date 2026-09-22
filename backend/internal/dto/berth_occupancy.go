package dto

import "time"

// BerthOccupancyQuery filters the 泊位时段占用 list. All fields are optional.
type BerthOccupancyQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Status   string `form:"status"`
	Berth    string `form:"berth"`
	PlanID   uint   `form:"planId"`
}

// BerthConflictQuery asks whether a proposed berth interval is free. Used by
// the plan approval form to show 冲突时段 before submission.
type BerthConflictQuery struct {
	Berth   string    `form:"berth" binding:"required,max=64"`
	StartAt time.Time `form:"startAt" binding:"required"`
	EndAt   time.Time `form:"endAt" binding:"required"`
}
