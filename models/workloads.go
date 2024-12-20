package models

const (
	WorkloadRunTypeService  = "service"
	WorkloadRunTypeFunction = "function"
	WorkloadRunTypeJob      = "job"

	WorkloadStateStarting = "starting"
	WorkloadStateRunning  = "running"
	WorkloadStateStopped  = "stopped"
	WorkloadStateError    = "error"
	WorkloadStateWarm     = "warm"
)
