// Package mlp implements a client for OMA Mobile Location Protocol (MLP)
// v3.5 over XML/HTTP, the Le interface a GMLC exposes for 4G LTE location
// requests. It speaks Standard Location Immediate Service (slir/slia) and
// Historic Location Immediate Service (hlir/hlia) only.
package mlp

import (
	"fmt"
	"time"
)

// Target identifies the subscriber being located. Exactly one of IMSI or
// MSISDN must be set.
type Target struct {
	IMSI   string
	MSISDN string
}

// QoS carries the subset of MLP's eqop element this client supports.
type QoS struct {
	HorizontalAccuracyMeters *float64
	// ResponseTime is "", "low_delay", or "delay_tolerant".
	ResponseTime string
}

// LocationType selects how MLP's loc_type element is set. "" and
// LocationTypeCurrent both mean CURRENT_POSITION (the field's default).
type LocationType string

const (
	LocationTypeCurrent            LocationType = ""
	LocationTypeCurrentOrLastKnown LocationType = "current_or_last_known"
)

// Result is a single resolved position.
type Result struct {
	Time              time.Time
	Latitude          float64
	Longitude         float64
	UncertaintyMeters float64
}

// RequestStatus is the outcome of a Submit call.
type RequestStatus struct {
	ID           string
	State        string // "completed" or "failed"
	FailureCode  string
	LocationType LocationType
	HighPriority bool
	Result       *Result
}

// HistoryPoint is one recorded fix returned by Client.History, oldest
// first.
type HistoryPoint struct {
	RecordedAt        time.Time
	Latitude          float64
	Longitude         float64
	UncertaintyMeters float64
}

// StatusError is returned when the GMLC answers with an MLP-level error
// (a whole-request rejection, not a per-target position failure).
type StatusError struct {
	Code   string
	Detail string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}
