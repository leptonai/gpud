package xid

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestNVLinkRuleSubCodeCounts(t *testing.T) {
	type subCodeSummary struct {
		units          map[string]struct{}
		errorStatus    map[uint32]struct{}
		intrinfoSample map[uint32]struct{}
		severities     map[string]struct{}
		resolutions    map[string]struct{}
		ruleCount      int
	}

	subCodesByXID := make(map[int]map[int]*subCodeSummary)

	for _, rule := range nvlinkRules {
		if rule.Xid < 144 || rule.Xid > 150 {
			continue
		}
		subCode, ok := subCodeFromRule(rule)
		if !ok {
			continue
		}
		if _, ok := subCodesByXID[rule.Xid]; !ok {
			subCodesByXID[rule.Xid] = make(map[int]*subCodeSummary)
		}
		if _, ok := subCodesByXID[rule.Xid][subCode]; !ok {
			subCodesByXID[rule.Xid][subCode] = &subCodeSummary{
				units:          make(map[string]struct{}),
				errorStatus:    make(map[uint32]struct{}),
				intrinfoSample: make(map[uint32]struct{}),
				severities:     make(map[string]struct{}),
				resolutions:    make(map[string]struct{}),
			}
		}

		summary := subCodesByXID[rule.Xid][subCode]
		summary.ruleCount++
		summary.units[rule.Unit] = struct{}{}
		summary.errorStatus[rule.ErrorStatus] = struct{}{}
		if rule.Severity != "" {
			summary.severities[rule.Severity] = struct{}{}
		}
		if rule.Resolution != "" {
			summary.resolutions[rule.Resolution] = struct{}{}
		}
		if sample, ok := sampleFromPattern(rule.IntrinfoPatternV2); ok {
			summary.intrinfoSample[sample] = struct{}{}
		} else if sample, ok := sampleFromPattern(rule.IntrinfoPatternV1); ok {
			summary.intrinfoSample[sample] = struct{}{}
		}
	}

	if len(subCodesByXID) == 0 {
		t.Fatalf("nvlinkRules contained no parsable sub-codes")
	}

	xids := make([]int, 0, len(subCodesByXID))
	for xid := range subCodesByXID {
		xids = append(xids, xid)
	}
	sort.Ints(xids)

	var exampleLines []string

	for _, xid := range xids {
		subSummaries := subCodesByXID[xid]

		subCodes := make([]int, 0, len(subSummaries))
		for subCode := range subSummaries {
			subCodes = append(subCodes, subCode)
		}
		sort.Ints(subCodes)

		xidDesc := ""
		if detail, ok := GetDetail(xid); ok {
			xidDesc = detail.Description
		}
		t.Logf("Xid %d (%s): %d sub-codes (align with kmsg_extended_test expectations)", xid, xidDesc, len(subCodes))

		for _, subCode := range subCodes {
			summary := subSummaries[subCode]
			units := sortedKeys(summary.units)
			statuses := sortedUint32Keys(summary.errorStatus)
			samples := sortedUint32Keys(summary.intrinfoSample)
			severities := sortedKeys(summary.severities)
			resolutions := sortedKeys(summary.resolutions)

			exampleUnit := "UNKNOWN_UNIT"
			if len(units) > 0 {
				exampleUnit = units[0]
			}
			exampleIntr := uint32(0)
			if len(samples) > 0 {
				exampleIntr = samples[0]
			}
			exampleErrStatus := uint32(0)
			if len(statuses) > 0 {
				exampleErrStatus = statuses[0]
			}
			exampleSeverity := "Nonfatal/Fatal"
			if len(severities) > 0 {
				exampleSeverity = severities[0]
			}

			t.Logf("  sub-code %-2d (0x%02X) [%s]: rules=%d | units=%v | severities=%v | resolutions=%v | err_status=%#v | intrinfo_examples=%#v",
				subCode, subCode, exampleUnit, summary.ruleCount, units, severities, resolutions, statuses, samples)
			exampleLine := fmt.Sprintf("NVRM: Xid (PCI:0000:01:00): %d, %s %s XC0 i0 Link 00 (0x%08x 0x%08x 0x00000000 0x00000000 0x00000000 0x00000000)",
				xid, exampleUnit, exampleSeverity, exampleIntr, exampleErrStatus)
			t.Logf("    example kmsg: %s", exampleLine)
			exampleLines = append(exampleLines, exampleLine)
		}
	}

	if len(exampleLines) > 0 {
		t.Log("kmsg multi-line simulation:\n" + strings.Join(exampleLines, "\n"))
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedUint32Keys(m map[uint32]struct{}) []uint32 {
	out := make([]uint32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
