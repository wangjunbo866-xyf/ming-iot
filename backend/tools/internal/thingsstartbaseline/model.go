// Package thingsstartbaseline defines the reproducible Things startup contract.
package thingsstartbaseline

// StageID identifies one ordered startup milestone.
type StageID string

const (
	StagePrecheck          StageID = "PRECHECK"
	StageDatabaseBootstrap StageID = "DATABASE_BOOTSTRAP"
	StageInfraReady        StageID = "INFRA_READY"
	StageTimedJobReady     StageID = "TIMEDJOB_READY"
	StageSysReady          StageID = "SYS_READY"
	StageDMReady           StageID = "DM_READY"
	StageDGProcessReady    StageID = "DG_PROCESS_READY"
	StageDGLinkReady       StageID = "DG_LINK_READY"
	StageAPIReady          StageID = "API_READY"
	StageVerified          StageID = "VERIFIED"
)

// ErrorCode is a stable machine-readable startup failure category.
type ErrorCode string

const (
	CodeBuildFailed                   ErrorCode = "BUILD_FAILED"
	CodeConfigInvalid                 ErrorCode = "CONFIG_INVALID"
	CodeExternalDependencyUnavailable ErrorCode = "EXTERNAL_DEPENDENCY_UNAVAILABLE"
	CodeDatabaseBootstrapFailed       ErrorCode = "DATABASE_BOOTSTRAP_FAILED"
	CodeDependencyNotReady            ErrorCode = "DEPENDENCY_NOT_READY"
	CodeRoleExited                    ErrorCode = "ROLE_EXITED"
	CodeRoleNotReady                  ErrorCode = "ROLE_NOT_READY"
	CodeContractMismatch              ErrorCode = "CONTRACT_MISMATCH"
	CodeTimeout                       ErrorCode = "TIMEOUT"
)

// Contract fixes every input used by the Things baseline startup workflow.
type Contract struct {
	SchemaVersion int              `yaml:"schemaVersion"`
	Profile       string           `yaml:"profile"`
	ProjectName   string           `yaml:"projectName"`
	Database      DatabaseContract `yaml:"database"`
	Roles         []RoleContract   `yaml:"roles"`
	Stages        []StageContract  `yaml:"stages"`
	Timeouts      TimeoutContract  `yaml:"timeouts"`
}

// DatabaseContract prevents a startup command from targeting legacy databases.
type DatabaseContract struct {
	RequiredName   string   `yaml:"requiredName"`
	ForbiddenNames []string `yaml:"forbiddenNames"`
}

// RoleContract specifies one runnable application role.
type RoleContract struct {
	ID         string `yaml:"id"`
	Module     string `yaml:"module"`
	Binary     string `yaml:"binary"`
	Image      string `yaml:"image"`
	Dockerfile string `yaml:"dockerfile"`
	Config     string `yaml:"config"`
	Endpoint   string `yaml:"endpoint"`
}

// StageContract assigns components to one ordered startup stage.
type StageContract struct {
	ID         StageID  `yaml:"id"`
	Components []string `yaml:"components"`
}

// TimeoutContract bounds individual probes and the complete startup run.
type TimeoutContract struct {
	ProbeSeconds int `yaml:"probeSeconds"`
	TotalSeconds int `yaml:"totalSeconds"`
}
