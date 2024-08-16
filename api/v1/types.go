package v1

import (
	"time"

	"github.com/leptonai/gpud/components"
)

type LeptonEvents []LeptonComponentEvents
type LeptonStates []LeptonComponentStates
type LeptonMetrics []LeptonComponentMetrics
type LeptonInfo []LeptonComponentInfo

type LeptonComponentEvents struct {
	Component string             `json:"component"`
	StartTime time.Time          `json:"startTime"`
	EndTime   time.Time          `json:"endTime"`
	Events    []components.Event `json:"events"`
}

type LeptonComponentStates struct {
	Component string             `json:"component"`
	States    []components.State `json:"states"`
}

type LeptonComponentMetrics struct {
	Component string              `json:"component"`
	Metrics   []components.Metric `json:"metrics"`
}

type LeptonComponentInfo struct {
	Component string          `json:"component"`
	StartTime time.Time       `json:"startTime"`
	EndTime   time.Time       `json:"endTime"`
	Info      components.Info `json:"info"`
}
