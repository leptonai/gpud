package fabricmanagerlog

const (
	// e.g.,
	// [Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33
	RegexNVSwitchFatalSXidFromLog = `.+detected NVSwitch fatal error (\d+)`

	// e.g.,
	// [Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61
	RegexNVSwitchNonFatalSXidFromLog = `.+detected NVSwitch non-fatal error (\d+)`

	// e.g.,
	// [Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.
	RegexNVSwitchNVLinkFailureFromLog = `.+failed to find the GPU handle \d+ in the multicast team .*`
)
