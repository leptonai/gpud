package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
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

	PersistenceMode string `json:"Persistence Mode"`
	AddressingMode  string `json:"Addressing Mode"`

	GPUResetStatus    *SMIGPUResetStatus    `json:"GPU Reset Status,omitempty"`
	ClockEventReasons *SMIClockEventReasons `json:"Clocks Event Reasons,omitempty"`

	ECCMode      *SMIECCMode      `json:"ECC Mode,omitempty"`
	ECCErrors    *SMIECCErrors    `json:"ECC Errors,omitempty"`
	RemappedRows *SMIRemappedRows `json:"Remapped Rows,omitempty"`

	Temperature      *SMIGPUTemperature   `json:"Temperature,omitempty"`
	GPUPowerReadings *SMIGPUPowerReadings `json:"GPU Power Readings,omitempty"`
	Processes        *SMIProcesses        `json:"Processes,omitempty"`

	FanSpeed string `json:"Fan Speed"`
}

func (gpu *NvidiaSMIGPU) GetSMIGPUPersistenceMode() SMIGPUPersistenceMode {
	if gpu.PersistenceMode == "Enabled" {
		return SMIGPUPersistenceMode{
			ID:      gpu.ID,
			Enabled: true,
		}
	}
	return SMIGPUPersistenceMode{
		ID:      gpu.ID,
		Enabled: false,
	}
}

type SMIGPUPersistenceMode struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type SMIGPUResetStatus struct {
	ResetRequired            string `json:"Reset Required"`
	DrainAndResetRecommended string `json:"Drain and Reset Recommended"`
}

type SMIClockEventReasons struct {
	SWPowerCap           string `json:"SW Power Cap"`
	SWThermalSlowdown    string `json:"SW Thermal Slowdown"`
	HWSlowdown           string `json:"HW Slowdown"`
	HWThermalSlowdown    string `json:"HW Thermal Slowdown"`
	HWPowerBrakeSlowdown string `json:"HW Power Brake Slowdown"`
}

type SMIECCMode struct {
	Current string `json:"Current"`
	Pending string `json:"Pending"`
}

type SMIECCErrors struct {
	ID string `json:"id"`

	Aggregate                         *SMIECCErrorAggregate                         `json:"Aggregate,omitempty"`
	AggregateUncorrectableSRAMSources *SMIECCErrorAggregateUncorrectableSRAMSources `json:"Aggregate Uncorrectable SRAM Sources,omitempty"`
	Volatile                          *SMIECCErrorVolatile                          `json:"Volatile,omitempty"`
}

type SMIRemappedRows struct {
	ID string `json:"id"`

	CorrectableError   string `json:"Correctable Error,omitempty"`
	UncorrectableError string `json:"Uncorrectable Error,omitempty"`

	// Yes/No.
	// If uncorrectable error is >0, this pending field is set to "Yes".
	// For a100/h100, it requires a GPU reset to actually remap the row.
	// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#rma-policy-thresholds
	Pending string `json:"Pending,omitempty"`

	// Yes/No
	RemappingFailureOccurred string `json:"Remapping Failure Occurred,omitempty"`
}

type ParsedSMIRemappedRows struct {
	ID string `json:"id"`

	RemappedDueToCorrectableErrors   string `json:"remapped_due_to_correctable_errors"`
	RemappedDueToUncorrectableErrors string `json:"remapped_due_to_uncorrectable_errors"`
	RemappingPending                 string `json:"remapping_pending"`
	RemappingFailed                  string `json:"remapping_failed"`
}

func (rw *SMIRemappedRows) Parse() (ParsedSMIRemappedRows, error) {
	return ParsedSMIRemappedRows{
		ID:                               rw.ID,
		RemappedDueToCorrectableErrors:   rw.CorrectableError,
		RemappedDueToUncorrectableErrors: rw.UncorrectableError,
		RemappingPending:                 rw.Pending,
		RemappingFailed:                  rw.RemappingFailureOccurred,
	}, nil
}

func (rw ParsedSMIRemappedRows) GetRemappedDueToCorrectableError() (int64, error) {
	return strconv.ParseInt(rw.RemappedDueToCorrectableErrors, 10, 64)
}

func (rw ParsedSMIRemappedRows) GetRemappedDueToUncorrectableError() (int64, error) {
	return strconv.ParseInt(rw.RemappedDueToUncorrectableErrors, 10, 64)
}

func (rw ParsedSMIRemappedRows) GetRemappingPending() (bool, error) {
	if rw.RemappingPending == "Yes" || rw.RemappingPending == "true" {
		return true, nil
	}
	if rw.RemappingPending == "No" || rw.RemappingPending == "false" {
		return false, nil
	}
	return false, fmt.Errorf("invalid pending value: %q", rw.RemappingPending)
}

func (rw ParsedSMIRemappedRows) GetRemappingFailured() (bool, error) {
	if rw.RemappingFailed == "Yes" || rw.RemappingFailed == "true" {
		return true, nil
	}
	if rw.RemappingFailed == "No" || rw.RemappingFailed == "false" {
		return false, nil
	}
	return false, fmt.Errorf("invalid remapping failure occurred value: %q", rw.RemappingFailed)
}

// Returns true if a GPU qualifies for RMA.
// ref. https://docs.nvidia.com/deploy/a100-gpu-mem-error-mgmt/index.html#rma-policy-thresholds-for-row-remapping
func (rw ParsedSMIRemappedRows) QualifiesForRMA() (bool, error) {
	failureOccurred, err := rw.GetRemappingFailured()
	if err != nil {
		return false, err
	}

	uncorrectableErrors, err := rw.GetRemappedDueToUncorrectableError()
	if err != nil {
		return false, err
	}

	// "remapping attempt for an uncorrectable memory error on a bank that already has eight uncorrectable error rows remapped."
	// "uncorrectableErrors >= 8" was dropped since it is also possible that:
	// "A remapping attempt for an uncorrectable memory error on a row that was already remapped and can occur with less than eight total remaps to the same bank."
	//
	// NVIDIA DCGM also checks for this condition (only check row remapping failure but not the uncorrectable error count)
	// ref. https://github.com/NVIDIA/DCGM/blob/b0ec3c624ea21e688b0d93cf9b214ae0eeb6fe52/nvvs/plugin_src/software/Software.cpp#L718-L736
	if failureOccurred && uncorrectableErrors < 8 {
		log.Logger.Debugw("uncorrectable error count <8 but still qualifies for RMA since remapping failed", "uncorrectableErrors", uncorrectableErrors)
	}

	return failureOccurred, nil
}

func (rw ParsedSMIRemappedRows) RequiresReset() (bool, error) {
	pending, err := rw.GetRemappingPending()
	if err != nil {
		return false, err
	}
	return pending, nil
}

type SMIECCErrorAggregate struct {
	DRAMCorrectable   string `json:"DRAM Correctable"`
	DRAMUncorrectable string `json:"DRAM Uncorrectable"`

	SRAMCorrectable       string `json:"SRAM Correctable"`
	SRAMThresholdExceeded string `json:"SRAM Threshold Exceeded"`

	SRAMUncorrectable       string `json:"SRAM Uncorrectable"`
	SRAMUncorrectableParity string `json:"SRAM Uncorrectable Parity"`  // for newer driver versions
	SRAMUncorrectableSECDED string `json:"SRAM Uncorrectable SEC-DED"` // for newer driver versions
}

type SMIECCErrorAggregateUncorrectableSRAMSources struct {
	SRAML2              string `json:"SRAM L2"`
	SRAMMicrocontroller string `json:"SRAM Microcontroller"`
	SRAMOther           string `json:"SRAM Other"`
	SRAMPCIE            string `json:"SRAM PCIE"`
	SRAMSM              string `json:"SRAM SM"`
}

type SMIECCErrorVolatile struct {
	DRAMCorrectable   string `json:"DRAM Correctable"`
	DRAMUncorrectable string `json:"DRAM Uncorrectable"`

	SRAMCorrectable   string `json:"SRAM Correctable"`
	SRAMUncorrectable string `json:"SRAM Uncorrectable"`

	SRAMUncorrectableParity string `json:"SRAM Uncorrectable Parity"`  // for newer driver versions
	SRAMUncorrectableSECDED string `json:"SRAM Uncorrectable SEC-DED"` // for newer driver versions
}

func (eccErrs SMIECCErrors) FindVolatileUncorrectableErrs() []string {
	errs := []string{}

	if eccErrs.Volatile != nil {
		if eccErrs.Volatile.DRAMUncorrectable != "" && eccErrs.Volatile.DRAMUncorrectable != "0" {
			errs = append(errs, fmt.Sprintf("GPU %s: Volatile DRAMUncorrectable: %s", eccErrs.ID, eccErrs.Volatile.DRAMUncorrectable))
		}
		if eccErrs.Volatile.SRAMUncorrectable != "" && eccErrs.Volatile.SRAMUncorrectable != "0" {
			errs = append(errs, fmt.Sprintf("GPU %s: Volatile SRAMUncorrectable: %s", eccErrs.ID, eccErrs.Volatile.SRAMUncorrectable))
		}
		if eccErrs.Volatile.SRAMUncorrectableParity != "" && eccErrs.Volatile.SRAMUncorrectableParity != "0" {
			errs = append(errs, fmt.Sprintf("GPU %s: Volatile SRAMUncorrectableParity: %s", eccErrs.ID, eccErrs.Volatile.SRAMUncorrectable))
		}
		if eccErrs.Volatile.SRAMUncorrectableSECDED != "" && eccErrs.Volatile.SRAMUncorrectableSECDED != "0" {
			errs = append(errs, fmt.Sprintf("GPU %s: Volatile SRAMUncorrectableSECDED: %s", eccErrs.ID, eccErrs.Volatile.SRAMUncorrectable))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
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

type SMIProcesses struct {
	GPUInstanceID        string `json:"GPU instance ID"`
	ComputeInstanceID    string `json:"Compute instance ID"`
	ProcessID            int64  `json:"Process ID"`
	ProcessType          string `json:"Process Type"`
	ProcessName          string `json:"Process Name"`
	ProcessUsedGPUMemory string `json:"Process Used GPU Memory"`
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
	if err := g.FindAddressingModeErr(); err != nil {
		errs = append(errs, err.Error())
	}
	if strings.Contains(g.FanSpeed, "Unknown Error") {
		errs = append(errs, fmt.Sprintf("%s: FanSpeed Unknown Error", g.ID))
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

// Returns the Address Mode error if any of the GPU has "Unknown Error" Addressing Mode.
// It may indicate Xid 31 "GPU memory page fault", where the application crashes with:
// e.g., RuntimeError: CUDA unknown error - this may be due to an incorrectly set up environment, e.g. changing env variable CUDA_VISIBLE_DEVICES after program start. Setting the available devices to be zero.
func (g NvidiaSMIGPU) FindAddressingModeErr() error {
	if strings.Contains(g.AddressingMode, "Unknown Error") {
		return fmt.Errorf("%s: AddressingMode %s", g.ID, g.AddressingMode)
	}
	return nil
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
