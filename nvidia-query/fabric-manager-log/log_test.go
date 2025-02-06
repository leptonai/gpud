package fabricmanagerlog

import (
	"regexp"
	"testing"
)

func TestRegexNVSwitchFatalSXidFromLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		log     string
		matches bool
	}{
		{"[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33", true},
	}

	re, err := regexp.Compile(RegexNVSwitchFatalSXidFromLog)
	if err != nil {
		t.Fatalf("Error compiling regex: %v", err)
	}
	for _, test := range tests {
		matched := re.MatchString(test.log)
		if matched != test.matches {
			t.Errorf("Expected match: %v, got: %v for log: %s", test.matches, matched, test.log)
		}
	}
}

func TestRegexNVSwitchNonFatalSXidFromLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		log     string
		matches bool
	}{
		{"[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61", true},
	}

	re, err := regexp.Compile(RegexNVSwitchNonFatalSXidFromLog)
	if err != nil {
		t.Fatalf("Error compiling regex: %v", err)
	}
	for _, test := range tests {
		matched := re.MatchString(test.log)
		if matched != test.matches {
			t.Errorf("Expected match: %v, got: %v for log: %s", test.matches, matched, test.log)
		}
	}
}

func TestRegexNVSwitchNVLinkFailureFromLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		log     string
		matches bool
	}{
		{"[Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.", true},
	}

	re, err := regexp.Compile(RegexNVSwitchNVLinkFailureFromLog)
	if err != nil {
		t.Fatalf("Error compiling regex: %v", err)
	}
	for _, test := range tests {
		matched := re.MatchString(test.log)
		if matched != test.matches {
			t.Errorf("Expected match: %v, got: %v for log: %s", test.matches, matched, test.log)
		}
	}
}
