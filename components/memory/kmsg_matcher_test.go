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

// ref. https://github.com/google/cadvisor/blob/master/utils/oomparser/oomparser_test.go

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMatchFunc(t *testing.T) {
	tests := []struct {
		name           string
		messages       []string
		expectedEvents []struct {
			eventName string
			message   string
		}
	}{
		{
			name: "Modern kernel OOM format (5.0+)",
			messages: []string{
				"postgres invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=/docker/container123,task_memcg=/docker/container123,task=postgres,pid=12345,uid=1000",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: postgres, pid: 12345",
				},
			},
		},
		{
			name: "Legacy kernel OOM format",
			messages: []string{
				"apache2 invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
				"Task in /docker/container456 killed as a result of limit of /docker/container456",
				"Killed process 9876 (apache2)",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: apache2, pid: 9876",
				},
			},
		},
		{
			name: "Multiple OOM events in sequence",
			messages: []string{
				// First OOM (modern format)
				"mysql invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=/system.slice,task_memcg=/system.slice,task=mysql,pid=11111,uid=27",
				// Second OOM (legacy format)
				"nginx invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
				"Out of memory: Killed process 22222 (nginx).",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: mysql, pid: 11111",
				},
				{
					eventName: "OOM",
					message:   "System OOM encountered, victim process: nginx, pid: 22222",
				},
			},
		},
		{
			name: "Non-OOM messages should be ignored",
			messages: []string{
				"kernel: [12345.678] Some other kernel message",
				"systemd[1]: Started Session 123 of user root.",
				"kernel: TCP: request_sock_TCP: Possible SYN flooding",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{},
		},
		{
			name: "Incomplete OOM sequence (no completion)",
			messages: []string{
				"java invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"kernel: [12345.678] Some unrelated message",
				"another unrelated log line",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{},
		},
		{
			name: "OOM with constraint types",
			messages: []string{
				"stress invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"oom-kill:constraint=CONSTRAINT_CPUSET,nodemask=0,cpuset=/restricted,mems_allowed=0,oom_memcg=/user.slice,task_memcg=/user.slice,task=stress,pid=33333,uid=1001",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: stress, pid: 33333",
				},
			},
		},
		{
			name: "cadvisor test - legacy container format",
			messages: []string{
				"ruby invoked oom-killer: gfp_mask=0x201da, order=0, oom_score_adj=0",
				"Task in /mem2 killed as a result of limit of /mem3",
				"Memory cgroup out of memory: Kill process 19636 (evil-program2) score 1923 or sacrifice child",
				"Killed process 19636 (evil-program2) total-vm:1460016kB, anon-rss:1414008kB, file-rss:4kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: evil-program2, pid: 19636",
				},
			},
		},
		{
			name: "cadvisor test - kubernetes container",
			messages: []string{
				"ruby invoked oom-killer: gfp_mask=0x201da, order=0, oom_score_adj=0",
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,mems_allowed=0,oom_memcg=/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,task_memcg=/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,task=evil-program2,pid=19667,uid=0",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: evil-program2, pid: 19667",
				},
			},
		},
		{
			name: "cadvisor test - memorymonster OOM",
			messages: []string{
				"memorymonster invoked oom-killer: gfp_mask=0xd0, order=0, oom_score_adj=0",
				"memorymonster cpuset=/ mems_allowed=0",
				"Out of memory: Kill process 13536 (memorymonster) score 1000 or sacrifice child",
				"Killed process 13536 (memorymonster) total-vm:33558652kB, anon-rss:920kB, file-rss:452kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "System OOM encountered, victim process: memorymonster, pid: 13536",
				},
			},
		},
		{
			name: "cadvisor test - process with spaces in name",
			messages: []string{
				"Plex Media Server invoked oom-killer: gfp_mask=0x201da, order=0, oom_score_adj=0",
				"Out of memory: Kill process 1234 (Plex Media Server) score 1000 or sacrifice child",
				"Killed process 1234 (Plex Media Server) total-vm:1000000kB, anon-rss:900000kB, file-rss:100kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "System OOM encountered, victim process: Plex Media Server, pid: 1234",
				},
			},
		},
		{
			name: "cadvisor test - process with special characters",
			messages: []string{
				"python3.4 invoked oom-killer: gfp_mask=0x201da, order=0, oom_score_adj=0",
				"Killed process 5678 (x86_64-pc-linux-gnu-c++-5.4.0) total-vm:500000kB, anon-rss:450000kB, file-rss:50kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "System OOM encountered, victim process: x86_64-pc-linux-gnu-c++-5.4.0, pid: 5678",
				},
			},
		},
		{
			name: "cadvisor test - docker container complex",
			messages: []string{
				"badsysprogram invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"Task in /docker/6562b1be2e55 killed as a result of limit of /docker/6562b1be2e55",
				"memory: usage 524288kB, limit 524288kB, failcnt 330",
				"Memory cgroup stats for /docker/6562b1be2e55: cache:0KB rss:524096KB rss_huge:0KB mapped_file:0KB writeback:0KB inactive_anon:0KB active_anon:524092KB inactive_file:0KB active_file:0KB unevictable:0KB",
				"[ pid ]   uid  tgid total_vm      rss nr_ptes nr_pmds swapents oom_score_adj name",
				"[11494]     0 11494   410346   131071     257       5        0             0 badsysprogram",
				"Memory cgroup out of memory: Kill process 11499 (badsysprogram) score 1000 or sacrifice child",
				"Killed process 11499 (badsysprogram) total-vm:1641384kB, anon-rss:524092kB, file-rss:192kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: badsysprogram, pid: 11499",
				},
			},
		},
		{
			name: "interleaved OOM events - new OOM interrupts incomplete one",
			messages: []string{
				// First OOM starts but is incomplete
				"first-process invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"Task in /docker/container1 killed as a result of limit of /docker/container1",
				"some unrelated kernel message",
				// Second OOM starts before first completes - should reset state
				"second-process invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=/docker/container2,task_memcg=/docker/container2,task=second-process,pid=5555,uid=1000",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: second-process, pid: 5555",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchFunc := createMatchFunc()
			eventIndex := 0

			for _, msg := range tt.messages {
				eventName, message := matchFunc(msg)

				if eventName != "" || message != "" {
					require.Less(t, eventIndex, len(tt.expectedEvents), "Got unexpected event: eventName=%q, message=%q", eventName, message)

					expected := tt.expectedEvents[eventIndex]
					assert.Equal(t, expected.eventName, eventName, "Event %d: eventName mismatch", eventIndex)
					assert.Equal(t, expected.message, message, "Event %d: message mismatch", eventIndex)
					eventIndex++
				}
			}

			assert.Equal(t, len(tt.expectedEvents), eventIndex, "Expected %d events, but got %d", len(tt.expectedEvents), eventIndex)
		})
	}
}

func TestOomInstanceSummary(t *testing.T) {
	tests := []struct {
		name     string
		instance *OOMInstance
		expected string
	}{
		{
			name:     "nil instance",
			instance: nil,
			expected: "",
		},
		{
			name:     "empty instance",
			instance: &OOMInstance{},
			expected: "OOM encountered",
		},
		{
			name: "instance with process info",
			instance: &OOMInstance{
				ProcessName: "postgres",
				Pid:         12345,
			},
			expected: "OOM encountered, victim process: postgres, pid: 12345",
		},
		{
			name: "instance with only PID",
			instance: &OOMInstance{
				Pid: 12345,
			},
			expected: "OOM encountered",
		},
		{
			name: "instance with only process name",
			instance: &OOMInstance{
				ProcessName: "postgres",
			},
			expected: "OOM encountered",
		},
		{
			name: "system OOM without process info",
			instance: &OOMInstance{
				VictimContainerName: "/",
			},
			expected: "System OOM encountered",
		},
		{
			name: "system OOM with process info",
			instance: &OOMInstance{
				VictimContainerName: "/",
				ProcessName:         "apache2",
				Pid:                 54321,
			},
			expected: "System OOM encountered, victim process: apache2, pid: 54321",
		},
		{
			name: "container OOM (not system)",
			instance: &OOMInstance{
				VictimContainerName: "/docker/abc123",
				ProcessName:         "nginx",
				Pid:                 9999,
			},
			expected: "OOM encountered, victim process: nginx, pid: 9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.instance.Summary()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContainerName(t *testing.T) {
	tests := []struct {
		name               string
		line               string
		expectedFound      bool
		expectedError      bool
		expectedContainer  string
		expectedVictim     string
		expectedConstraint string
		expectedPid        int
		expectedProcess    string
	}{
		{
			name:               "modern kernel format",
			line:               "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=/docker/abc123,task_memcg=/docker/abc123,task=postgres,pid=12345,uid=1000",
			expectedFound:      true,
			expectedError:      false,
			expectedContainer:  "/docker/abc123",
			expectedVictim:     "/docker/abc123",
			expectedConstraint: "CONSTRAINT_MEMCG",
			expectedPid:        12345,
			expectedProcess:    "postgres",
		},
		{
			name:              "legacy format",
			line:              "Task in /docker/container123 killed as a result of limit of /docker/container456",
			expectedFound:     false,
			expectedError:     false,
			expectedContainer: "/docker/container123",
			expectedVictim:    "/docker/container456",
		},
		{
			name:          "invalid PID in modern format",
			line:          "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=/,mems_allowed=0,oom_memcg=/test,task_memcg=/test,task=test,pid=invalid,uid=1000",
			expectedFound: false,
			expectedError: true,
		},
		{
			name:          "unrelated line",
			line:          "some random kernel message",
			expectedFound: false,
			expectedError: false,
		},
		// Additional cadvisor test cases
		{
			name:              "cadvisor legacy format - mem2/mem3",
			line:              "Task in /mem2 killed as a result of limit of /mem3",
			expectedFound:     false,
			expectedError:     false,
			expectedContainer: "/mem2",
			expectedVictim:    "/mem3",
		},
		{
			name:               "cadvisor kubernetes format",
			line:               "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,mems_allowed=0,oom_memcg=/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,task_memcg=/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0,task=evil-program2,pid=19667,uid=0",
			expectedFound:      true,
			expectedError:      false,
			expectedContainer:  "/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0",
			expectedVictim:     "/kubepods/burstable/pod87e18f63-83cb-11e7-a01d-42010a8001db/ef807430361edd74442f7c5703c8273545c501e93a80e8e4542de37ad75153b0",
			expectedConstraint: "CONSTRAINT_MEMCG",
			expectedPid:        19667,
			expectedProcess:    "evil-program2",
		},
		{
			name:               "constraint types test",
			line:               "oom-kill:constraint=CONSTRAINT_CPUSET,nodemask=0,cpuset=/test,mems_allowed=0-1,oom_memcg=/cgroup/memory,task_memcg=/cgroup/memory,task=testapp,pid=9999,uid=500",
			expectedFound:      true,
			expectedError:      false,
			expectedContainer:  "/cgroup/memory",
			expectedVictim:     "/cgroup/memory",
			expectedConstraint: "CONSTRAINT_CPUSET",
			expectedPid:        9999,
			expectedProcess:    "testapp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &OOMInstance{}
			found, err := getContainerName(tt.line, instance)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedFound, found)

			if tt.expectedContainer != "" {
				assert.Equal(t, tt.expectedContainer, instance.ContainerName)
			}

			if tt.expectedVictim != "" {
				assert.Equal(t, tt.expectedVictim, instance.VictimContainerName)
			}

			if tt.expectedConstraint != "" {
				assert.Equal(t, tt.expectedConstraint, instance.Constraint)
			}

			if tt.expectedPid != 0 {
				assert.Equal(t, tt.expectedPid, instance.Pid)
			}

			if tt.expectedProcess != "" {
				assert.Equal(t, tt.expectedProcess, instance.ProcessName)
			}
		})
	}
}

func TestGetProcessNamePid(t *testing.T) {
	tests := []struct {
		name            string
		line            string
		expectedFound   bool
		expectedError   bool
		expectedPid     int
		expectedProcess string
	}{
		{
			name:            "standard killed process line",
			line:            "Killed process 12345 (postgres)",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     12345,
			expectedProcess: "postgres",
		},
		{
			name:            "out of memory killed process",
			line:            "Out of memory: Killed process 9876 (nginx).",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     9876,
			expectedProcess: "nginx",
		},
		{
			name:          "invalid PID",
			line:          "Killed process invalid (test)",
			expectedFound: false,
			expectedError: false,
		},
		{
			name:          "unrelated line",
			line:          "some other kernel message",
			expectedFound: false,
			expectedError: false,
		},
		// Additional cadvisor test cases
		{
			name:            "cadvisor evil-program2",
			line:            "Killed process 19636 (evil-program2) total-vm:1460016kB, anon-rss:1414008kB, file-rss:4kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     19636,
			expectedProcess: "evil-program2",
		},
		{
			name:            "cadvisor memorymonster",
			line:            "Killed process 13536 (memorymonster) total-vm:33558652kB, anon-rss:920kB, file-rss:452kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     13536,
			expectedProcess: "memorymonster",
		},
		{
			name:            "process with spaces",
			line:            "Killed process 1234 (Plex Media Server) total-vm:1000000kB, anon-rss:900000kB, file-rss:100kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     1234,
			expectedProcess: "Plex Media Server",
		},
		{
			name:            "process with special characters",
			line:            "Killed process 5678 (x86_64-pc-linux-gnu-c++-5.4.0) total-vm:500000kB, anon-rss:450000kB, file-rss:50kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     5678,
			expectedProcess: "x86_64-pc-linux-gnu-c++-5.4.0",
		},
		{
			name:            "process with version number",
			line:            "Killed process 9999 (python3.4) total-vm:100000kB, anon-rss:90000kB, file-rss:10kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     9999,
			expectedProcess: "python3.4",
		},
		{
			name:            "process with hyphen",
			line:            "Killed process 7777 (foo-bar) total-vm:200000kB, anon-rss:180000kB, file-rss:20kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     7777,
			expectedProcess: "foo-bar",
		},
		{
			name:            "badsysprogram docker container",
			line:            "Killed process 11499 (badsysprogram) total-vm:1641384kB, anon-rss:524092kB, file-rss:192kB",
			expectedFound:   true,
			expectedError:   false,
			expectedPid:     11499,
			expectedProcess: "badsysprogram",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &OOMInstance{}
			found, err := getProcessNamePid(tt.line, instance)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedFound, found)

			if tt.expectedPid != 0 {
				assert.Equal(t, tt.expectedPid, instance.Pid)
			}

			if tt.expectedProcess != "" {
				assert.Equal(t, tt.expectedProcess, instance.ProcessName)
			}
		})
	}
}

func TestCheckIfStartOfOomMessages(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "standard OOM start",
			line:     "postgres invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
			expected: true,
		},
		{
			name:     "OOM start with different process",
			line:     "mysqld invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
			expected: true,
		},
		{
			name:     "not OOM start",
			line:     "Out of memory: Killed process 12345 (postgres)",
			expected: false,
		},
		{
			name:     "unrelated message",
			line:     "kernel: TCP established hash table entries: 8192",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkIfStartOfOomMessages(tt.line)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCreateMatchFuncStreamOOMs tests sequential processing of realistic kernel log messages
// ported from cadvisor's TestStreamOOMs to verify the match function handles complex scenarios
func TestCreateMatchFuncStreamOOMs(t *testing.T) {
	tests := []struct {
		name           string
		messages       []string
		expectedEvents []struct {
			eventName string
			message   string
		}
	}{
		{
			name: "cadvisor stream test - memorymonster OOM",
			messages: []string{
				"memorymonster invoked oom-killer: gfp_mask=0xd0, order=0, oom_score_adj=0",
				"memorymonster cpuset=/ mems_allowed=0",
				"CPU: 23 PID: 13536 Comm: memorymonster Not tainted 3.13.0-24-generic #46-Ubuntu",
				"Hardware name: Google Google Compute Engine/Google Compute Engine, BIOS Google 01/01/2011",
				"ffff8803c7014b00 ffff8803c7012a28 ffffffff81637d5c 0000000000000000",
				"0000000000000000 ffff8803c7012ab8 ffffffff8162aeac ffff8803c7014b00",
				"ffff880362672a00 ffff8803c7014b00 ffff8803c7012a98 ffff8803c7014b00",
				"Call Trace:",
				"[<ffffffff81637d5c>] dump_stack+0x45/0x56",
				"[<ffffffff8162aeac>] dump_header+0x7f/0x1f1",
				"[<ffffffff8113b5b1>] oom_kill_process+0x201/0x3c0",
				"[<ffffffff811bb574>] ? oom_unkillable_task+0x124/0x140",
				"[<ffffffff8113b0fc>] oom_scan_process_thread+0x2c/0x50",
				"[<ffffffff8113bb31>] out_of_memory+0x421/0x4e0",
				"[<ffffffff8113bc05>] pagefault_out_of_memory+0x15/0x20",
				"[<ffffffff8104d168>] mm_fault_error+0x68/0x140",
				"[<ffffffff8104d4b2>] __do_page_fault+0x3a2/0x4e0",
				"[<ffffffff8104d5ff>] do_page_fault+0x9/0x10",
				"[<ffffffff81640e28>] page_fault+0x28/0x30",
				"Task in /mem2 killed as a result of limit of /mem2",
				"memory: usage 1048576kB, limit 1048576kB, failcnt 3",
				"memory+swap: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"kmem: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"Memory cgroup stats for /mem2: cache:0KB rss:1048572KB rss_huge:1046528KB mapped_file:0KB writeback:0KB inactive_anon:0KB active_anon:1048572KB inactive_file:0KB active_file:0KB unevictable:0KB",
				"[ pid ]   uid  tgid total_vm      rss nr_ptes nr_pmds swapents oom_score_adj name",
				"[13536]     0 13536  8389663   262143     517       9        0             0 memorymonster",
				"Memory cgroup out of memory: Kill process 13536 (memorymonster) score 1000 or sacrifice child",
				"Killed process 13536 (memorymonster) total-vm:33558652kB, anon-rss:920kB, file-rss:452kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: memorymonster, pid: 13536",
				},
			},
		},
		{
			name: "cadvisor stream test - badsysprogram OOM",
			messages: []string{
				"badsysprogram invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"badsysprogram cpuset=/ mems_allowed=0",
				"CPU: 14 PID: 1532 Comm: badsysprogram Not tainted 3.13.0-24-generic #46-Ubuntu",
				"Hardware name: Google Google Compute Engine/Google Compute Engine, BIOS Google 01/01/2011",
				"0000000000000000 ffff8803750c3a28 ffffffff81637d5c 0000000000000000",
				"0000000000000000 ffff8803750c3ab8 ffffffff8162aeac ffff880374c20800",
				"ffffffff81c134c0 ffff880374c20800 ffff8803750c3a98 ffff880374c20800",
				"Call Trace:",
				"[<ffffffff81637d5c>] dump_stack+0x45/0x56",
				"[<ffffffff8162aeac>] dump_header+0x7f/0x1f1",
				"[<ffffffff8113b5b1>] oom_kill_process+0x201/0x3c0",
				"[<ffffffff811bb574>] ? oom_unkillable_task+0x124/0x140",
				"[<ffffffff8113b0fc>] oom_scan_process_thread+0x2c/0x50",
				"[<ffffffff8113bb31>] out_of_memory+0x421/0x4e0",
				"[<ffffffff81141b96>] __alloc_pages_nodemask+0xa96/0xb10",
				"[<ffffffff8117a45a>] alloc_pages_current+0xaa/0x170",
				"[<ffffffff81134c50>] __page_cache_alloc+0x120/0x150",
				"[<ffffffff81137b8f>] __do_page_cache_readahead+0xef/0x200",
				"[<ffffffff81137e63>] ondemand_readahead+0x223/0x440",
				"[<ffffffff81137fe1>] page_cache_sync_readahead+0x61/0xa0",
				"[<ffffffff8112eb97>] generic_file_aio_read+0x4a7/0x770",
				"[<ffffffff81195a2c>] do_sync_read+0x5c/0x90",
				"[<ffffffff81196b19>] vfs_read+0xa9/0x160",
				"[<ffffffff81196c0f>] SyS_read+0x4f/0xa0",
				"[<ffffffff8163f329>] system_call_fastpath+0x16/0x1b",
				"Mem-Info:",
				"Node 0 DMA per-cpu:",
				"CPU    0: hi:    0, btch:   1 usd:   0",
				"CPU    1: hi:    0, btch:   1 usd:   0",
				"Node 0 DMA32 per-cpu:",
				"CPU    0: hi:  186, btch:  31 usd: 173",
				"CPU    1: hi:  186, btch:  31 usd:  27",
				"active_anon:909670 inactive_anon:60892 isolated_anon:0",
				" active_file:2 inactive_file:3 isolated_file:0",
				" unevictable:0 dirty:0 writeback:0 unstable:0",
				" free:42982 slab_reclaimable:17967 slab_unreclaimable:7438",
				" mapped:2 shmem:61 pagetables:1906 bounce:0",
				" free_cma:0",
				"Node 0 DMA free:15896kB min:36kB low:44kB high:52kB active_anon:0kB inactive_anon:0kB active_file:0kB inactive_file:0kB unevictable:0kB isolated(anon):0kB isolated(file):0kB present:15992kB managed:15896kB mlocked:0kB dirty:0kB writeback:0kB mapped:0kB shmem:0kB slab_reclaimable:0kB slab_unreclaimable:16kB kernel_stack:0kB pagetables:0kB unstable:0kB bounce:0kB free_cma:0kB writeback_tmp:0kB pages_scanned:0 all_unreclaimable? yes",
				"lowmem_reserve[]: 0 3932 3932 3932",
				"Node 0 DMS32 free:156032kB min:9328kB low:11660kB high:13992kB active_anon:3638680kB inactive_anon:243568kB active_file:8kB inactive_file:12kB unevictable:0kB isolated(anon):0kB isolated(file):0kB present:4193208kB managed:4025980kB mlocked:0kB dirty:0kB writeback:0kB mapped:8kB shmem:244kB slab_reclaimable:71868kB slab_unreclaimable:29736kB kernel_stack:2144kB pagetables:7624kB unstable:0kB bounce:0kB free_cma:0kB writeback_tmp:0kB pages_scanned:0 all_unreclaimable? no",
				"lowmem_reserve[]: 0 0 0 0",
				"Node 0 DMA: 3*4kB (U) 0*8kB 2*16kB (R) 1*32kB (R) 2*64kB (R) 1*128kB (R) 1*256kB (R) 0*512kB 1*1024kB (R) 1*2048kB (R) 3*4096kB (M) = 15896kB",
				"Node 0 DMA32: 19710*4kB (UEM) 0*8kB 0*16kB 0*32kB 0*64kB 0*128kB 0*256kB 0*512kB 0*1024kB 0*2048kB 14*4096kB (R) = 136392kB",
				"Node 0 hugepages_total=0 hugepages_free=0 hugepages_surp=0 hugepages_size=2048kB",
				"4030 total pagecache pages",
				"0 pages in swap cache",
				"Swap cache stats: add 0, delete 0, find 0/0",
				"Free swap  = 0kB",
				"Total swap = 0kB",
				"1048063 pages RAM",
				"0 pages HighMem/MovableOnly",
				"41828 pages reserved",
				"[ pid ]   uid  tgid total_vm      rss nr_ptes nr_pmds swapents oom_score_adj name",
				"[  389]     0   389     5416      835      15       3        0         -1000 systemd-udevd",
				"[  503]     0   503    13864      743      26       3        0         -1000 sshd",
				"[  509]     0   509     4635      363      13       3        0             0 rpcbind",
				"[ 1532]     0  1532   410347   398597     796       7        0             0 badsysprogram",
				"Out of memory: Kill process 1532 (badsysprogram) score 1000 or sacrifice child",
				"Killed process 1532 (badsysprogram) total-vm:1641388kB, anon-rss:1595164kB, file-rss:76kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "System OOM encountered, victim process: badsysprogram, pid: 1532",
				},
			},
		},
		{
			name: "cadvisor stream test - multiple docker container OOMs",
			messages: []string{
				// First Docker container OOM
				"gunpowder-memho invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"gunpowder-memho cpuset=/ mems_allowed=0",
				"CPU: 13 PID: 1381 Comm: gunpowder-memho Not tainted 3.13.0-24-generic #46-Ubuntu",
				"Hardware name: Google Google Compute Engine/Google Compute Engine, BIOS Google 01/01/2011",
				"0000000000000000 ffff8803ce36fa28 ffffffff81637d5c 0000000000000000",
				"0000000000000000 ffff8803ce36fab8 ffffffff8162aeac ffff8803c1e20800",
				"ffff8803b2f99800 ffff8803c1e20800 ffff8803ce36fa98 ffff8803c1e20800",
				"Call Trace:",
				"[<ffffffff81637d5c>] dump_stack+0x45/0x56",
				"[<ffffffff8162aeac>] dump_header+0x7f/0x1f1",
				"[<ffffffff8113b5b1>] oom_kill_process+0x201/0x3c0",
				"[<ffffffff811bb574>] ? oom_unkillable_task+0x124/0x140",
				"[<ffffffff8113b0fc>] oom_scan_process_thread+0x2c/0x50",
				"[<ffffffff8113bb31>] out_of_memory+0x421/0x4e0",
				"[<ffffffff81141b96>] __alloc_pages_nodemask+0xa96/0xb10",
				"[<ffffffff8117a45a>] alloc_pages_current+0xaa/0x170",
				"[<ffffffff81134c50>] __page_cache_alloc+0x120/0x150",
				"[<ffffffff81137b8f>] __do_page_cache_readahead+0xef/0x200",
				"[<ffffffff81137e63>] ondemand_readahead+0x223/0x440",
				"[<ffffffff81137fe1>] page_cache_sync_readahead+0x61/0xa0",
				"[<ffffffff8112eb97>] generic_file_aio_read+0x4a7/0x770",
				"[<ffffffff81195a2c>] do_sync_read+0x5c/0x90",
				"[<ffffffff81196b19>] vfs_read+0xa9/0x160",
				"[<ffffffff81196c0f>] SyS_read+0x4f/0xa0",
				"[<ffffffff8163f329>] system_call_fastpath+0x16/0x1b",
				"Task in /docker/2e088fe462e91068e24d500b1d1f16062e26532e80e14f4e8be2da9e0077d77a killed as a result of limit of /docker/2e088fe462e91068e24d500b1d1f16062e26532e80e14f4e8be2da9e0077d77a",
				"memory: usage 524288kB, limit 524288kB, failcnt 3",
				"memory+swap: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"kmem: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"Memory cgroup stats for /docker/2e088fe462e91068e24d500b1d1f16062e26532e80e14f4e8be2da9e0077d77a: cache:0KB rss:524284KB rss_huge:522240KB mapped_file:0KB writeback:0KB inactive_anon:0KB active_anon:524284KB inactive_file:0KB active_file:0KB unevictable:0KB",
				"[ pid ]   uid  tgid total_vm      rss nr_ptes nr_pmds swapents oom_score_adj name",
				"[ 1381]     0  1381   131097   131071     259       5        0             0 gunpowder-memho",
				"Memory cgroup out of memory: Kill process 1381 (gunpowder-memho) score 1000 or sacrifice child",
				"Killed process 1381 (gunpowder-memho) total-vm:524388kB, anon-rss:524284kB, file-rss:0kB",
				// Some unrelated kernel messages between OOMs
				"eth0: no IPv6 routers present",
				"device eth0 entered promiscuous mode",
				"device eth0 left promiscuous mode",
				// Second Docker container OOM
				"gunpowder-memho invoked oom-killer: gfp_mask=0x280da, order=0, oom_score_adj=0",
				"gunpowder-memho cpuset=/ mems_allowed=0",
				"CPU: 5 PID: 1667 Comm: gunpowder-memho Not tainted 3.13.0-24-generic #46-Ubuntu",
				"Hardware name: Google Google Compute Engine/Google Compute Engine, BIOS Google 01/01/2011",
				"0000000000000000 ffff880362a2fa28 ffffffff81637d5c 0000000000000000",
				"0000000000000000 ffff880362a2fab8 ffffffff8162aeac ffff8803c1e20800",
				"ffff8803b2f99800 ffff8803c1e20800 ffff880362a2fa98 ffff8803c1e20800",
				"Call Trace:",
				"[<ffffffff81637d5c>] dump_stack+0x45/0x56",
				"[<ffffffff8162aeac>] dump_header+0x7f/0x1f1",
				"[<ffffffff8113b5b1>] oom_kill_process+0x201/0x3c0",
				"[<ffffffff811bb574>] ? oom_unkillable_task+0x124/0x140",
				"[<ffffffff8113b0fc>] oom_scan_process_thread+0x2c/0x50",
				"[<ffffffff8113bb31>] out_of_memory+0x421/0x4e0",
				"[<ffffffff81141b96>] __alloc_pages_nodemask+0xa96/0xb10",
				"[<ffffffff8117a45a>] alloc_pages_current+0xaa/0x170",
				"[<ffffffff81134c50>] __page_cache_alloc+0x120/0x150",
				"[<ffffffff81137b8f>] __do_page_cache_readahead+0xef/0x200",
				"[<ffffffff81137e63>] ondemand_readahead+0x223/0x440",
				"[<ffffffff81137fe1>] page_cache_sync_readahead+0x61/0xa0",
				"[<ffffffff8112eb97>] generic_file_aio_read+0x4a7/0x770",
				"[<ffffffff81195a2c>] do_sync_read+0x5c/0x90",
				"[<ffffffff81196b19>] vfs_read+0xa9/0x160",
				"[<ffffffff81196c0f>] SyS_read+0x4f/0xa0",
				"[<ffffffff8163f329>] system_call_fastpath+0x16/0x1b",
				"Task in /docker/6c6fcab8562f9b82e4a52af3b46b8bf4b23810d2bc1ed5b7fa7b4e3b69ad6fb5 killed as a result of limit of /docker/6c6fcab8562f9b82e4a52af3b46b8bf4b23810d2bc1ed5b7fa7b4e3b69ad6fb5",
				"memory: usage 524288kB, limit 524288kB, failcnt 4",
				"memory+swap: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"kmem: usage 0kB, limit 9007199254740991kB, failcnt 0",
				"Memory cgroup stats for /docker/6c6fcab8562f9b82e4a52af3b46b8bf4b23810d2bc1ed5b7fa7b4e3b69ad6fb5: cache:0KB rss:524284KB rss_huge:522240KB mapped_file:0KB writeback:0KB inactive_anon:0KB active_anon:524284KB inactive_file:0KB active_file:0KB unevictable:0KB",
				"[ pid ]   uid  tgid total_vm      rss nr_ptes nr_pmds swapents oom_score_adj name",
				"[ 1667]     0  1667   131097   131071     259       5        0             0 gunpowder-memho",
				"Memory cgroup out of memory: Kill process 1667 (gunpowder-memho) score 1000 or sacrifice child",
				"Killed process 1667 (gunpowder-memho) total-vm:524388kB, anon-rss:524284kB, file-rss:0kB",
			},
			expectedEvents: []struct {
				eventName string
				message   string
			}{
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: gunpowder-memho, pid: 1381",
				},
				{
					eventName: "OOM",
					message:   "OOM encountered, victim process: gunpowder-memho, pid: 1667",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchFunc := createMatchFunc()
			eventIndex := 0

			for _, msg := range tt.messages {
				eventName, message := matchFunc(msg)

				if eventName != "" || message != "" {
					require.Less(t, eventIndex, len(tt.expectedEvents), "Got unexpected event: eventName=%q, message=%q", eventName, message)

					expected := tt.expectedEvents[eventIndex]
					assert.Equal(t, expected.eventName, eventName, "Event %d: eventName mismatch", eventIndex)
					assert.Equal(t, expected.message, message, "Event %d: message mismatch", eventIndex)
					eventIndex++
				}
			}

			assert.Equal(t, len(tt.expectedEvents), eventIndex, "Expected %d events, but got %d", len(tt.expectedEvents), eventIndex)
		})
	}
}
