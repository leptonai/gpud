package fabricmanager

import "regexp"

const (
	// e.g.,
	// [Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33
	eventNVSwitchFatalSXid   = "fabricmanager_nvswitch_fatal_error"
	regexNVSwitchFatalSXid   = `.+detected NVSwitch fatal error (\d+)`
	messageNVSwitchFatalSXid = "NVSwitch fatal error detected"

	// e.g.,
	// [Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61
	eventNVSwitchNonFatalSXid   = "fabricmanager_nvswitch_non_fatal_error"
	regexNVSwitchNonFatalSXid   = `.+detected NVSwitch non-fatal error (\d+)`
	messageNVSwitchNonFatalSXid = "NVSwitch non-fatal error detected"

	// e.g.,
	// [Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.
	eventNVSwitchNVLinkFailure   = "fabricmanager_nvswitch_nvlink_failure"
	regexNVSwitchNVLinkFailure   = `.+failed to find the GPU handle \d+ in the multicast team .*`
	messageNVSwitchNVLinkFailure = "NVSwitch NVLink failure detected"

	// e.g.,
	// detected number of NVSwitches don't match with any supported system topology, aborting fabric manager
	// This occurs when fabric manager fails to start due to topology mismatch (missing/failed NVSwitch devices).
	eventNVSwitchTopologyMismatch   = "fabricmanager_nvswitch_topology_mismatch"
	regexNVSwitchTopologyMismatch   = `.*detected number of NVSwitches don't match with any supported system topology.*`
	messageNVSwitchTopologyMismatch = "NVSwitch topology mismatch detected - fabric manager failed to start"

	// e.g.,
	// request to query NVSwitch device information from NVSwitch driver failed with error:WARNING Nothing to do [NV_WARN_NOTHING_TO_DO]
	// This occurs when fabric manager starts but finds no NVSwitch devices to manage.
	// This is NOT an error condition - it indicates the system does not need fabric manager.
	// Common scenarios: GH200 standalone (no NVSwitch), simple NVLink bridges, PCIe GPUs.
	// Ref: https://forums.developer.nvidia.com/t/nvidia-fabricmanager-running-error-with-nv-warn-nothing-to-do/272899
	// Ref: https://github.com/NVIDIA/gpu-operator/issues/610
	EventNVSwitchNothingToDo   = "fabricmanager_nvswitch_nothing_to_do"
	regexNVSwitchNothingToDo   = `.*NV_WARN_NOTHING_TO_DO.*`
	messageNVSwitchNothingToDo = "Fabric manager has nothing to do - no NVSwitch devices to manage"
)

var (
	compiledNVSwitchFatalSXid        = regexp.MustCompile(regexNVSwitchFatalSXid)
	compiledNVSwitchNonFatalSXid     = regexp.MustCompile(regexNVSwitchNonFatalSXid)
	compiledNVSwitchNVLinkFailure    = regexp.MustCompile(regexNVSwitchNVLinkFailure)
	compiledNVSwitchTopologyMismatch = regexp.MustCompile(regexNVSwitchTopologyMismatch)
	compiledNVSwitchNothingToDo      = regexp.MustCompile(regexNVSwitchNothingToDo)
)

func HasNVSwitchFatalSXid(line string) bool {
	if match := compiledNVSwitchFatalSXid.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasNVSwitchNonFatalSXid(line string) bool {
	if match := compiledNVSwitchNonFatalSXid.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasNVSwitchNVLinkFailure(line string) bool {
	if match := compiledNVSwitchNVLinkFailure.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasNVSwitchTopologyMismatch(line string) bool {
	if match := compiledNVSwitchTopologyMismatch.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasNVSwitchNothingToDo(line string) bool {
	if match := compiledNVSwitchNothingToDo.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (eventName string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasNVSwitchFatalSXid, eventName: eventNVSwitchFatalSXid, regex: regexNVSwitchFatalSXid, message: messageNVSwitchFatalSXid},
		{check: HasNVSwitchNonFatalSXid, eventName: eventNVSwitchNonFatalSXid, regex: regexNVSwitchNonFatalSXid, message: messageNVSwitchNonFatalSXid},
		{check: HasNVSwitchNVLinkFailure, eventName: eventNVSwitchNVLinkFailure, regex: regexNVSwitchNVLinkFailure, message: messageNVSwitchNVLinkFailure},
		{check: HasNVSwitchTopologyMismatch, eventName: eventNVSwitchTopologyMismatch, regex: regexNVSwitchTopologyMismatch, message: messageNVSwitchTopologyMismatch},
		{check: HasNVSwitchNothingToDo, eventName: EventNVSwitchNothingToDo, regex: regexNVSwitchNothingToDo, message: messageNVSwitchNothingToDo},
	}
}
