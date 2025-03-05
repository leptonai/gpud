package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// GPU object from the nvidia-smi query.
// ref. "nvidia-smi --help-query-gpu"
type NvidiaSMIGPU struct {
	// The original GPU identifier from the nvidia-smi query output.
	// e.g., "GPU 00000000:53:00.0"
	ID string `json:"ID"`

	ProductName         string `json:"Product Name"`
	ProductBrand        string `json:"Product Brand"`
	ProductArchitecture string `json:"Product Architecture"`

	ClockEventReasons *SMIClockEventReasons `json:"Clocks Event Reasons,omitempty"`
	Temperature       *SMIGPUTemperature    `json:"Temperature,omitempty"`
	GPUPowerReadings  *SMIGPUPowerReadings  `json:"GPU Power Readings,omitempty"`
}

type SMIClockEventReasons struct {
	SWPowerCap           string `json:"SW Power Cap"`
	SWThermalSlowdown    string `json:"SW Thermal Slowdown"`
	HWSlowdown           string `json:"HW Slowdown"`
	HWThermalSlowdown    string `json:"HW Thermal Slowdown"`
	HWPowerBrakeSlowdown string `json:"HW Power Brake Slowdown"`
}

// If any field shows "Unknown Error", it means GPU has some issues.
type SMIGPUTemperature struct {
	ID string `json:"id"`

	Current string `json:"GPU Current Temp"`
	Limit   string `json:"GPU T.Limit Temp"`

	// Shutdown limit for older drivers (e.g., 535.129.03).
	Shutdown      string `json:"GPU Shutdown Temp"`
	ShutdownLimit string `json:"GPU Shutdown T.Limit Temp"`

	// Slowdown limit for older drivers (e.g., 535.129.03).
	Slowdown      string `json:"GPU Slowdown Temp"`
	SlowdownLimit string `json:"GPU Slowdown T.Limit Temp"`

	MaxOperatingLimit string `json:"GPU Max Operating T.Limit Temp"`

	// this value is not reliable to monitor as it's often N/A
	Target string `json:"GPU Target Temperature"`

	MemoryCurrent           string `json:"Memory Current Temp"`
	MemoryMaxOperatingLimit string `json:"Memory Max Operating T.Limit Temp"`
}

func (tm *SMIGPUTemperature) GetCurrentCelsius() (float64, error) {
	v := tm.Current
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU current temperature: %s (expected celsius)", tm.Current)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) GetLimitCelsius() (float64, error) {
	v := tm.Limit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU t.limit temperature: %s (expected celsius)", tm.Limit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) GetShutdownCelsius() (float64, error) {
	v := tm.Shutdown
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU t.shutdown temperature: %s (expected celsius)", tm.Limit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) GetShutdownLimitCelsius() (float64, error) {
	v := tm.ShutdownLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU t.shutdown limit temperature: %s (expected celsius)", tm.Limit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) GetSlowdownCelsius() (float64, error) {
	v := tm.Slowdown
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU t.slowdown temperature: %s (expected celsius)", tm.Limit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) GetSlowdownLimitCelsius() (float64, error) {
	v := tm.SlowdownLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " C") {
		v = strings.TrimSuffix(v, " C")
	} else {
		return 0.0, fmt.Errorf("invalid GPU t.slowdown limit temperature: %s (expected celsius)", tm.Limit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (tm *SMIGPUTemperature) Parse() (ParsedTemperature, error) {
	temp := ParsedTemperature{}
	temp.ID = tm.ID

	temp.CurrentHumanized = tm.Current
	cur, err := tm.GetCurrentCelsius()
	if err != nil {
		return ParsedTemperature{}, err
	}
	temp.CurrentCelsius = fmt.Sprintf("%.2f", cur)

	temp.ShutdownHumanized = tm.Shutdown
	temp.ShutdownLimit = tm.ShutdownLimit
	shutdownLimit, err := tm.GetShutdownCelsius()
	if err != nil {
		shutdownLimit, err = tm.GetShutdownLimitCelsius()
	}
	if err == nil {
		temp.ShutdownCelsius = fmt.Sprintf("%.2f", shutdownLimit)
	}

	temp.SlowdownHumanized = tm.Slowdown
	temp.SlowdownLimit = tm.SlowdownLimit
	slowdown, err := tm.GetSlowdownCelsius()
	if err != nil {
		slowdown, err = tm.GetSlowdownLimitCelsius()
	}
	if err == nil {
		temp.SlowdownCelsius = fmt.Sprintf("%.2f", slowdown)
	}

	temp.LimitHumanized = tm.Limit
	limit, err := tm.GetLimitCelsius()
	if err == nil {
		temp.LimitCelsius = fmt.Sprintf("%.2f", limit)
	} else {
		if shutdownLimit > 0 {
			limit = shutdownLimit
		}
		if limit == 0 && slowdown > 0 {
			limit = slowdown
		}
	}

	temp.UsedPercent = "0.0"
	if limit > 0 {
		temp.UsedPercent = fmt.Sprintf("%.2f", cur/limit*100)
	}

	temp.MaxOperatingLimit = tm.MaxOperatingLimit

	temp.Target = tm.Target
	temp.MemoryCurrent = tm.MemoryCurrent
	temp.MemoryMaxOperatingLimit = tm.MemoryMaxOperatingLimit

	return temp, nil
}

type ParsedTemperature struct {
	ID string `json:"id"`

	CurrentHumanized string `json:"current_humanized"`
	CurrentCelsius   string `json:"current_celsius"`

	LimitHumanized string `json:"limit_humanized"`
	LimitCelsius   string `json:"limit_celsius"`

	UsedPercent string `json:"used_percent"`

	ShutdownHumanized string `json:"shutdown_humanized"`
	ShutdownLimit     string `json:"shutdown_limit"`
	ShutdownCelsius   string `json:"shutdown_celsius"`

	SlowdownHumanized string `json:"slowdown_humanized"`
	SlowdownLimit     string `json:"slowdown_limit"`
	SlowdownCelsius   string `json:"slowdown_celsius"`

	MaxOperatingLimit string `json:"max_operating_limit"`

	Target                  string `json:"target"`
	MemoryCurrent           string `json:"memory_current"`
	MemoryMaxOperatingLimit string `json:"memory_max_operating_limit"`
}

func (temp ParsedTemperature) GetCurrentCelsius() (float64, error) {
	return strconv.ParseFloat(temp.CurrentCelsius, 64)
}

func (temp ParsedTemperature) GetLimitCelsius() (float64, error) {
	return strconv.ParseFloat(temp.LimitCelsius, 64)
}

func (temp ParsedTemperature) GetShutdownCelsius() (float64, error) {
	return strconv.ParseFloat(temp.ShutdownCelsius, 64)
}

func (temp ParsedTemperature) GetSlowdownCelsius() (float64, error) {
	return strconv.ParseFloat(temp.SlowdownCelsius, 64)
}

func (temp ParsedTemperature) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(temp.UsedPercent, 64)
}

type SMIGPUPowerReadings struct {
	ID string `json:"id"`

	PowerDraw           string `json:"Power Draw"`
	CurrentPowerLimit   string `json:"Current Power Limit"`
	RequestedPowerLimit string `json:"Requested Power Limit"`
	DefaultPowerLimit   string `json:"Default Power Limit"`
	MinPowerLimit       string `json:"Min Power Limit"`
	MaxPowerLimit       string `json:"Max Power Limit"`
}

func (g *SMIGPUPowerReadings) GetPowerDrawW() (float64, error) {
	v := g.PowerDraw
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid power draw: %s (expected watts)", g.PowerDraw)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) GetCurrentPowerLimitW() (float64, error) {
	v := g.CurrentPowerLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid current power limit: %s (expected watts)", g.CurrentPowerLimit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) GetRequestedPowerLimitW() (float64, error) {
	v := g.RequestedPowerLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid current power limit: %s (expected watts)", g.RequestedPowerLimit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) GetDefaultPowerLimitW() (float64, error) {
	v := g.DefaultPowerLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid current power limit: %s (expected watts)", g.DefaultPowerLimit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) GetMinPowerLimitW() (float64, error) {
	v := g.MinPowerLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid current power limit: %s (expected watts)", g.MinPowerLimit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) GetMaxPowerLimitW() (float64, error) {
	v := g.MaxPowerLimit
	if v == "N/A" {
		return 0.0, errors.New("N/A")
	}

	if strings.HasSuffix(v, " W") {
		v = strings.TrimSuffix(v, " W")
	} else {
		return 0.0, fmt.Errorf("invalid current power limit: %s (expected watts)", g.MaxPowerLimit)
	}

	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func (g *SMIGPUPowerReadings) Parse() (ParsedSMIPowerReading, error) {
	pr := ParsedSMIPowerReading{}
	pr.ID = g.ID

	pr.PowerDrawHumanized = g.PowerDraw
	cur, err := g.GetPowerDrawW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.PowerDrawW = fmt.Sprintf("%.2f", cur)

	pr.CurrentPowerLimitHumanized = g.CurrentPowerLimit
	limit, err := g.GetCurrentPowerLimitW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.CurrentPowerLimitW = fmt.Sprintf("%.2f", limit)

	pr.UsedPercent = "0.0"
	if limit > 0 {
		pr.UsedPercent = fmt.Sprintf("%.2f", cur/limit*100)
	}

	pr.RequestedPowerLimitHumanized = g.RequestedPowerLimit
	v, err := g.GetRequestedPowerLimitW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.RequestedPowerLimitW = fmt.Sprintf("%.2f", v)

	pr.DefaultPowerLimitHumanized = g.DefaultPowerLimit
	v, err = g.GetDefaultPowerLimitW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.DefaultPowerLimitW = fmt.Sprintf("%.2f", v)

	pr.MinPowerLimitHumanized = g.MinPowerLimit
	v, err = g.GetMinPowerLimitW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.MinPowerLimitW = fmt.Sprintf("%.2f", v)

	pr.MaxPowerLimitHumanized = g.MaxPowerLimit
	v, err = g.GetMaxPowerLimitW()
	if err != nil {
		return ParsedSMIPowerReading{}, err
	}
	pr.MaxPowerLimitW = fmt.Sprintf("%.2f", v)

	return pr, nil
}

type ParsedSMIPowerReading struct {
	ID string `json:"id"`

	PowerDrawW         string `json:"power_draw_w"`
	PowerDrawHumanized string `json:"power_draw_humanized"`

	CurrentPowerLimitW         string `json:"current_power_limit_w"`
	CurrentPowerLimitHumanized string `json:"current_power_limit_humanized"`

	UsedPercent string `json:"used_percent"`

	RequestedPowerLimitW         string `json:"requested_power_limit_w"`
	RequestedPowerLimitHumanized string `json:"requested_power_limit_humanized"`

	DefaultPowerLimitW         string `json:"default_power_limit_w"`
	DefaultPowerLimitHumanized string `json:"default_power_limit_humanized"`

	MinPowerLimitW         string `json:"min_power_limit_w"`
	MinPowerLimitHumanized string `json:"min_power_limit_humanized"`

	MaxPowerLimitW         string `json:"max_power_limit_w"`
	MaxPowerLimitHumanized string `json:"max_power_limit_humanized"`
}

func (pr ParsedSMIPowerReading) GetPowerDrawW() (float64, error) {
	return strconv.ParseFloat(pr.PowerDrawW, 64)
}

func (pr ParsedSMIPowerReading) GetCurrentPowerLimitW() (float64, error) {
	return strconv.ParseFloat(pr.CurrentPowerLimitW, 64)
}

func (pr ParsedSMIPowerReading) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(pr.UsedPercent, 64)
}

func (pr ParsedSMIPowerReading) GetRequestedPowerLimitW() (float64, error) {
	return strconv.ParseFloat(pr.RequestedPowerLimitW, 64)
}

func (pr ParsedSMIPowerReading) GetDefaultPowerLimitW() (float64, error) {
	return strconv.ParseFloat(pr.DefaultPowerLimitW, 64)
}

func (pr ParsedSMIPowerReading) GetMinPowerLimitW() (float64, error) {
	return strconv.ParseFloat(pr.MinPowerLimitW, 64)
}

func (pr ParsedSMIPowerReading) GetMaxPowerLimitW() (float64, error) {
	return strconv.ParseFloat(pr.MaxPowerLimitW, 64)
}

// Returns true if the GPU has any errors.
// ref. https://forums.developer.nvidia.com/t/nvidia-smi-q-shows-several-unknown-error-gpu-ignored-by-pytorch/263881
func (g NvidiaSMIGPU) FindErrs() []string {
	errs := make([]string, 0)
	if g.Temperature != nil {
		if strings.Contains(g.Temperature.Current, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.Current Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.Limit, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.Limit Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.ShutdownLimit, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.ShutdownLimit Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.SlowdownLimit, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.SlowdownLimit Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.MaxOperatingLimit, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.MaxOperatingLimit Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.Target, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.Target Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.MemoryCurrent, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.MemoryCurrent Unknown Error", g.ID))
		}
		if strings.Contains(g.Temperature.MemoryMaxOperatingLimit, "Unknown Error") {
			errs = append(errs, fmt.Sprintf("%s: Temperature.MemoryMaxOperatingLimit Unknown Error", g.ID))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

const (
	ClockEventsActive    = "Active"
	ClockEventsNotActive = "Not Active"
)

func (g NvidiaSMIGPU) FindHWSlowdownErrs() []string {
	errs := make([]string, 0)
	if g.ClockEventReasons != nil && g.ClockEventReasons.HWSlowdown == ClockEventsActive {
		if g.ClockEventReasons.HWThermalSlowdown == ClockEventsActive {
			errs = append(errs, fmt.Sprintf("%s: ClockEventReasons.HWSlowdown.ThermalSlowdown %s", g.ID, ClockEventsActive))
		}
		if g.ClockEventReasons.HWPowerBrakeSlowdown == ClockEventsActive {
			errs = append(errs, fmt.Sprintf("%s: ClockEventReasons.HWSlowdown.PowerBrakeSlowdown %s", g.ID, ClockEventsActive))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

const AddressingModeNone = "None"
