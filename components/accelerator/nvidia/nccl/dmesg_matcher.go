package nccl

import (
	"regexp"
)

const (
	// repeated messages may indicate GPU communication issues, which may happen due to fabric manager issues
	// e.g.,
	// [Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]
	eventNCCLSegfaultInLibnccl   = "nvidia_nccl_segfault_in_libnccl"
	regexNCCLSegfaultInLibnccl   = `.*segfault at.*in libnccl\.so.*`
	messageNCCLSegfaultInLibnccl = `NCCL communication error (segfault in libnccl.so)`
)

var (
	compiledNCCLSegfaultInLibnccl = regexp.MustCompile(regexNCCLSegfaultInLibnccl)
)

func HasNCCLSegfaultInLibnccl(line string) bool {
	if match := compiledNCCLSegfaultInLibnccl.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (eventName string, regex string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.regex, m.message
		}
	}
	return "", "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasNCCLSegfaultInLibnccl, eventName: eventNCCLSegfaultInLibnccl, regex: regexNCCLSegfaultInLibnccl, message: messageNCCLSegfaultInLibnccl},
	}
}
