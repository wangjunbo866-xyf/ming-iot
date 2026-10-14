package thingsstartbaseline

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

var requiredRoles = []RoleContract{
	{ID: "sys", Module: "saas", Binary: "syssvr", Image: "ming-iot-sys:baseline", Dockerfile: "deploy/images/Dockerfile.sys", Config: "deploy/profiles/things-baseline/config/saas/sys.yaml", Endpoint: "sys:9180"},
	{ID: "timedjob", Module: "saas", Binary: "timedjobsvr", Image: "ming-iot-timedjob:baseline", Dockerfile: "deploy/images/Dockerfile.timedjob", Config: "deploy/profiles/things-baseline/config/saas/timedjob.yaml", Endpoint: "timedjob:3256"},
	{ID: "dm", Module: "things", Binary: "dmsvr", Image: "ming-iot-dm:baseline", Dockerfile: "deploy/images/Dockerfile.dm", Config: "deploy/profiles/things-baseline/config/things/dm.yaml", Endpoint: "dm:9081"},
	{ID: "dg", Module: "things", Binary: "dgsvr", Image: "ming-iot-dg:baseline", Dockerfile: "deploy/images/Dockerfile.dg", Config: "deploy/profiles/things-baseline/config/things/dg.yaml", Endpoint: "dg:6166"},
	{ID: "api", Module: "things", Binary: "thingsvr", Image: "ming-iot-api:baseline", Dockerfile: "deploy/images/Dockerfile.api", Config: "deploy/profiles/things-baseline/config/things/api.yaml", Endpoint: "api:7788"},
}

var requiredStages = []StageID{
	StagePrecheck, StageDatabaseBootstrap, StageInfraReady, StageTimedJobReady, StageSysReady,
	StageDMReady, StageDGProcessReady, StageDGLinkReady, StageAPIReady, StageVerified,
}

var requiredStageComponents = [][]string{
	{"docker", "compose", "contract", "secrets", "postgres"},
	{"postgres"},
	{"redis", "etcd", "nats", "tdengine", "emqx"},
	{"timedjob"},
	{"sys"},
	{"dm"},
	{"dg"},
	{"emqx", "dg.rpc"},
	{"api"},
	{"all"},
}

// LoadContract decodes precisely one strict YAML contract document.
func LoadContract(filename string) (*Contract, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open contract %q: %w", filename, err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return nil, fmt.Errorf("decode contract %q: %w", filename, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("decode contract %q: multiple YAML documents", filename)
	} else if err != io.EOF {
		return nil, fmt.Errorf("decode contract %q: trailing YAML: %w", filename, err)
	}
	if err := ValidateContract(&contract); err != nil {
		return nil, fmt.Errorf("validate contract %q: %w", filename, err)
	}
	return &contract, nil
}

// ValidateContract rejects all deviations from the reproducible baseline.
func ValidateContract(contract *Contract) error {
	if contract == nil {
		return fmt.Errorf("contract is nil")
	}
	if contract.SchemaVersion != 1 {
		return fmt.Errorf("schemaVersion must be 1")
	}
	if contract.Profile != "things-baseline" {
		return fmt.Errorf("profile must be things-baseline")
	}
	if contract.ProjectName != "ming-iot-things-baseline" {
		return fmt.Errorf("projectName must be ming-iot-things-baseline")
	}
	if contract.Database.RequiredName != "ming_iot_things_baseline" {
		return fmt.Errorf("database.requiredName must be ming_iot_things_baseline")
	}
	if !contains(contract.Database.ForbiddenNames, "iThings") {
		return fmt.Errorf("database.forbiddenNames must protect iThings")
	}
	if contains(contract.Database.ForbiddenNames, contract.Database.RequiredName) {
		return fmt.Errorf("database.forbiddenNames must not include database.requiredName")
	}
	if len(contract.Roles) != len(requiredRoles) {
		return fmt.Errorf("roles must contain exactly %d entries", len(requiredRoles))
	}
	if err := validateRoleUnique(contract.Roles); err != nil {
		return err
	}
	for index, required := range requiredRoles {
		if err := compareRole(index, contract.Roles[index], required); err != nil {
			return err
		}
	}
	if len(contract.Stages) != len(requiredStages) {
		return fmt.Errorf("stages must contain exactly %d entries", len(requiredStages))
	}
	for index, stage := range contract.Stages {
		if stage.ID != requiredStages[index] {
			return fmt.Errorf("stages[%d].id must be %s", index, requiredStages[index])
		}
		if !sameStrings(stage.Components, requiredStageComponents[index]) {
			return fmt.Errorf("stages[%d].components do not match %s", index, stage.ID)
		}
	}
	if contract.Timeouts.ProbeSeconds != 90 {
		return fmt.Errorf("timeouts.probeSeconds must be 90")
	}
	if contract.Timeouts.TotalSeconds != 900 {
		return fmt.Errorf("timeouts.totalSeconds must be 900")
	}
	return nil
}

func validateRoleUnique(roles []RoleContract) error {
	for _, field := range []struct {
		name  string
		value func(RoleContract) string
	}{
		{"id", func(role RoleContract) string { return role.ID }},
		{"binary", func(role RoleContract) string { return role.Binary }},
		{"image", func(role RoleContract) string { return role.Image }},
		{"config", func(role RoleContract) string { return role.Config }},
	} {
		seen := make(map[string]int, len(roles))
		for index, role := range roles {
			value := field.value(role)
			if previous, exists := seen[value]; exists {
				return fmt.Errorf("roles.%s must be unique: roles[%d].%s duplicates roles[%d].%s", field.name, index, field.name, previous, field.name)
			}
			seen[value] = index
		}
	}
	return nil
}

func compareRole(index int, got, want RoleContract) error {
	for _, field := range []struct {
		name string
		got  string
		want string
	}{
		{"id", got.ID, want.ID},
		{"module", got.Module, want.Module},
		{"binary", got.Binary, want.Binary},
		{"image", got.Image, want.Image},
		{"dockerfile", got.Dockerfile, want.Dockerfile},
		{"config", got.Config, want.Config},
		{"endpoint", got.Endpoint, want.Endpoint},
	} {
		if field.got != field.want {
			return fmt.Errorf("roles[%d].%s must be %s", index, field.name, field.want)
		}
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
