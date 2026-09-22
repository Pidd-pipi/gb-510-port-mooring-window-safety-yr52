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
	ErrWeatherUnsafe     = errors.New("the linked weather window is not safe for approval")
	ErrBerthBusy         = errors.New("the berth is being changed by another approval; refresh and retry")
)

// SlotConflictError describes a rejected approval because the same berth time
// slot is already occupied by another approved mooring plan.
type SlotConflictError struct {
	Berth     string
	Conflicts []ConflictSlot
}

// ConflictSlot is the minimal view of an overlapping occupancy shown to the
// caller when an approval must leave no partial occupancy behind.
type ConflictSlot struct {
	PlanCode string `json:"planCode"`
	Berth    string `json:"berth"`
	StartAt  string `json:"startAt"`
	EndAt    string `json:"endAt"`
}

func (e *SlotConflictError) Error() string {
	return "the berth time slot overlaps with another approved mooring plan"
}

// Is reports all slot conflicts as the same business condition so handlers can
// map them via errors.Is.
func (e *SlotConflictError) Is(target error) bool {
	_, ok := target.(*SlotConflictError)
	return ok
}
