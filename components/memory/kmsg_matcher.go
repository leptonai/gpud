// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// ref. https://github.com/google/cadvisor/blob/master/utils/oomparser/oomparser.go

package memory

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
)

const OOMEventName = "OOM"

func createMatchFunc() func(line string) (eventName string, message string) {
	// below match function is called for each incoming kmsg message
	// so we track the progress outside of the function
	readingOOMMessages := false
	var oomCurrentInstance *OOMInstance

	// this match function is to be run in sequence for each incoming message
	return func(line string) (eventName string, message string) {
		// Always check if the current line is the start of a new OOM sequence.
		// This handles cases where a new OOM event begins before the previous one
		// was fully parsed (e.g., incomplete legacy event or missing lines).
		isNewOomStart := checkIfStartOfOomMessages(line)

		if isNewOomStart {
			// A new OOM event has started. This could be the first one, or it could
			// interrupt a previous, incomplete event. Reset and start fresh.
			readingOOMMessages = true
			oomCurrentInstance = &OOMInstance{
				ContainerName:       "/",
				VictimContainerName: "/",
			}
			// The "invoked oom-killer" line itself contains no victim data,
			// so we can end processing for this line and wait for the next one.
			return "", ""
		}

		// If we're not in the middle of reading OOM messages, we can ignore this line.
		if !readingOOMMessages {
			return "", ""
		}

		// Try to extract container information from the current line
		containerFound, err := getContainerName(line, oomCurrentInstance)
		if err != nil {
			// If there's an error, reset and skip
			readingOOMMessages = false
			oomCurrentInstance = nil
			return "", ""
		}

		// Modern kernel format: containerRegexp extracts all info in one match
		if containerFound && oomCurrentInstance.Pid != 0 {
			// Generate the event message
			eventName = OOMEventName
			message = oomCurrentInstance.Summary()

			// Reset state for next OOM event
			readingOOMMessages = false
			oomCurrentInstance = nil

			return eventName, message
		}

		// Legacy format: container info was not found, try to get process info
		if !containerFound {
			processFound, err := getProcessNamePid(line, oomCurrentInstance)
			if err != nil {
				// If there's an error, reset and skip
				readingOOMMessages = false
				oomCurrentInstance = nil
				return "", ""
			}

			// If we found process info, the OOM event is complete
			if processFound {
				// Generate the event message
				eventName = OOMEventName
				message = oomCurrentInstance.Summary()

				// Reset state for next OOM event
				readingOOMMessages = false
				oomCurrentInstance = nil

				return eventName, message
			}
		}

		// Still collecting information, return empty
		return "", ""
	}
}

// struct that contains information related to an OOM kill instance
type OOMInstance struct {
	// process id of the killed process
	Pid int
	// the name of the killed process
	ProcessName string
	// the absolute name of the container that OOMed
	ContainerName string
	// the absolute name of the container that was killed
	// due to the OOM.
	VictimContainerName string
	// the constraint that triggered the OOM.  One of CONSTRAINT_NONE,
	// CONSTRAINT_CPUSET, CONSTRAINT_MEMORY_POLICY, CONSTRAINT_MEMCG
	Constraint string
}

func (o *OOMInstance) Summary() string {
	if o == nil {
		return ""
	}
	eventMsg := "OOM encountered"
	if o.ProcessName != "" && o.Pid != 0 {
		eventMsg = fmt.Sprintf("%s, victim process: %s, pid: %d", eventMsg, o.ProcessName, o.Pid)
	}
	return eventMsg
}

var (
	legacyContainerRegexp = regexp.MustCompile(`Task in (.*) killed as a result of limit of (.*)`)

	// Starting in 5.0 linux kernels, the OOM message changed
	// e.g.,
	// oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	containerRegexp = regexp.MustCompile(`oom-kill:constraint=(.*),nodemask=(.*),cpuset=(.*),mems_allowed=(.*),oom_memcg=(.*),task_memcg=(.*),task=(.*),pid=(.*),uid=(.*)`)

	// e.g.,
	// Out of memory: Killed process 123, UID 48, (httpd).
	lastLineRegexp = regexp.MustCompile(`Killed process ([0-9]+) \((.+)\)`)

	// e.g.,
	// postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	firstLineRegexp = regexp.MustCompile(`invoked oom-killer:`)
)

// gets the container name from a line and adds it to the oomInstance.
func getLegacyContainerName(line string, currentOomInstance *OOMInstance) error {
	parsedLine := legacyContainerRegexp.FindStringSubmatch(line)
	if parsedLine == nil {
		return nil
	}
	currentOomInstance.ContainerName = path.Join("/", parsedLine[1])
	currentOomInstance.VictimContainerName = path.Join("/", parsedLine[2])
	return nil
}

// gets the container name from a line and adds it to the oomInstance.
func getContainerName(line string, currentOomInstance *OOMInstance) (bool, error) {
	parsedLine := containerRegexp.FindStringSubmatch(line)
	if parsedLine == nil {
		// Fall back to the legacy format if it isn't found here.
		return false, getLegacyContainerName(line, currentOomInstance)
	}
	currentOomInstance.ContainerName = parsedLine[6]
	currentOomInstance.VictimContainerName = parsedLine[5]
	currentOomInstance.Constraint = parsedLine[1]
	pid, err := strconv.Atoi(parsedLine[8])
	if err != nil {
		return false, err
	}
	currentOomInstance.Pid = pid
	currentOomInstance.ProcessName = parsedLine[7]
	return true, nil
}

// gets the pid, name, and date from a line and adds it to oomInstance
func getProcessNamePid(line string, currentOomInstance *OOMInstance) (bool, error) {
	reList := lastLineRegexp.FindStringSubmatch(line)

	if reList == nil {
		return false, nil
	}

	pid, err := strconv.Atoi(reList[1])
	if err != nil {
		return false, err
	}
	currentOomInstance.Pid = pid
	currentOomInstance.ProcessName = reList[2]
	return true, nil
}

// uses regex to see if line is the start of a kernel oom log
func checkIfStartOfOomMessages(line string) bool {
	potentialOomStart := firstLineRegexp.MatchString(line)
	return potentialOomStart
}
