package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrSelfApproval      = errors.New("the submitter cannot approve the same safety clearance")
	ErrReviewerRequired  = errors.New("a reviewer or administrator must perform the second confirmation")
	ErrWindowVersion     = errors.New("the weather window version is missing or changed")
	ErrBerthWindowUnsafe = errors.New("the related weather window must exist and be safe before approval")
	ErrBerthOccupied     = errors.New("the berth time slot overlaps an active occupancy")
	ErrBerthMissing      = errors.New("berth code, mooring start/end time and related weather window are required for approval")
	ErrBerthTimeRange    = errors.New("berthing end time must be after start time")
)
