package os

import "testing"

func TestHasVFSFileMaxLimitReached(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "exact match",
			line: "VFS: file-max limit 1000000 reached",
			want: true,
		},
		{
			name: "with timestamp prefix",
			line: "[Sun Dec  1 14:54:40 2024] VFS: file-max limit 1000000 reached",
			want: true,
		},
		{
			name: "with different number",
			line: "VFS: file-max limit 500000 reached",
			want: true,
		},
		{
			name: "with facility and level",
			line: "kern  :warn  : VFS: file-max limit 1000000 reached",
			want: true,
		},
		{
			name: "with ISO timestamp",
			line: "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 VFS: file-max limit 1000000 reached",
			want: true,
		},
		{
			name: "some other log message",
			line: "some other log message",
			want: false,
		},
		{
			name: "empty string",
			line: "",
			want: false,
		},
		{
			name: "partial match - missing reached",
			line: "VFS: file-max limit 1000000",
			want: false,
		},
		{
			name: "partial match - missing number",
			line: "VFS: file-max limit reached",
			want: false,
		},
		{
			name: "case mismatch",
			line: "vfs: File-max Limit 1000000 Reached",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasVFSFileMaxLimitReached(tt.line); got != tt.want {
				t.Errorf("HasVFSFileMaxLimitReached(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantName    string
		wantMessage string
	}{
		{
			name:        "VFS file-max limit reached",
			line:        "VFS: file-max limit 1000000 reached",
			wantName:    eventNameVFSFileMaxLimitReached,
			wantMessage: messageVFSFileMaxLimitReached,
		},
		{
			name:        "VFS file-max with timestamp",
			line:        "[Sun Dec  1 14:54:40 2024] VFS: file-max limit 1000000 reached",
			wantName:    eventNameVFSFileMaxLimitReached,
			wantMessage: messageVFSFileMaxLimitReached,
		},
		{
			name:        "VFS file-max with ISO timestamp and facility",
			line:        "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 VFS: file-max limit 1000000 reached",
			wantName:    eventNameVFSFileMaxLimitReached,
			wantMessage: messageVFSFileMaxLimitReached,
		},
		{
			name:        "no match",
			line:        "some random log message",
			wantName:    "",
			wantMessage: "",
		},
		{
			name:        "empty string",
			line:        "",
			wantName:    "",
			wantMessage: "",
		},
		{
			name:        "partial match",
			line:        "VFS: file-max limit 1000000",
			wantName:    "",
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotMessage := Match(tt.line)
			if gotName != tt.wantName {
				t.Errorf("Match() name = %v, want %v", gotName, tt.wantName)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Match() message = %v, want %v", gotMessage, tt.wantMessage)
			}
		})
	}
}

func TestGetMatches(t *testing.T) {
	matches := getMatches()

	// Verify we have the expected number of matchers
	if len(matches) != 1 {
		t.Errorf("getMatches() returned %d matches, want 1", len(matches))
	}

	// Verify the VFS file-max matcher
	vfsMatch := matches[0]
	if vfsMatch.eventName != eventNameVFSFileMaxLimitReached {
		t.Errorf("first match name = %v, want %v", vfsMatch.eventName, eventNameVFSFileMaxLimitReached)
	}
	if vfsMatch.regex != regexVFSFileMaxLimitReached {
		t.Errorf("first match regex = %v, want %v", vfsMatch.regex, regexVFSFileMaxLimitReached)
	}
	if vfsMatch.message != messageVFSFileMaxLimitReached {
		t.Errorf("first match message = %v, want %v", vfsMatch.message, messageVFSFileMaxLimitReached)
	}

	// Test the check function
	if !vfsMatch.check("VFS: file-max limit 1000000 reached") {
		t.Error("check function failed to match valid input")
	}
	if vfsMatch.check("invalid input") {
		t.Error("check function matched invalid input")
	}
}

// TestMatch_ComprehensiveInvalidInputs tests the Match function with various invalid inputs
func TestMatch_ComprehensiveInvalidInputs(t *testing.T) {
	invalidInputs := []string{
		"",
		" ",
		"VFS: file-max limit",
		"file-max limit reached",
		"VFS file-max limit reached",          // Missing colon
		"vfs: file-max limit 1000000 reached", // Lowercase VFS
	}

	for _, input := range invalidInputs {
		t.Run("Invalid: "+input, func(t *testing.T) {
			name, message := Match(input)
			if name != "" || message != "" {
				t.Errorf("Match(%q) = (%q, %q), want empty strings", input, name, message)
			}
		})
	}
}

// TestMatch_WithMultipleMatchers tests the behavior if we were to have multiple matchers
func TestMatch_WithMultipleMatchers(t *testing.T) {
	// This test verifies that the first match is returned
	matches := getMatches()
	if len(matches) == 0 {
		t.Skip("No matches defined, skipping test")
	}

	// Test that the first match takes precedence
	match := matches[0]
	testInput := "VFS: file-max limit 1000000 reached"

	name, message := Match(testInput)
	if name != match.eventName {
		t.Errorf("Match(%q) name = %q, want %q", testInput, name, match.eventName)
	}
	if message != match.message {
		t.Errorf("Match(%q) message = %q, want %q", testInput, message, match.message)
	}
}

// TestKernelPanicDetection tests kernel panic detection with various log scenarios
func TestKernelPanicDetection(t *testing.T) {
	tests := []struct {
		name          string
		logLines      []string
		wantEventName string
		wantMessage   string
		wantPID       int
		wantCPU       int
		wantProcess   string
	}{
		{
			name: "Test Case 1 - Panic ending with Kernel Offset (no reboot)",
			logLines: []string{
				"<0>[2315424.098863] Kernel panic - not syncing: hung_task: blocked tasks",
				"<4>[2315424.099010] CPU: 24 PID: 1364 Comm: khungtaskd Tainted: P           OE     5.15.0-1053-nvidia #54-Ubuntu",
				"<4>[2315424.099185] Hardware name: ASUSTeK COMPUTER INC. ESC N8-E11/Z13PN-D32 Series, BIOS 2301 03/21/2024",
				"<4>[2315424.099382] Call Trace:",
				"<4>[2315424.099573]  <TASK>",
				"<4>[2315424.099762]  show_stack+0x52/0x5c",
				"<4>[2315424.099954]  dump_stack_lvl+0x4a/0x63",
				"<4>[2315424.100145]  dump_stack+0x10/0x16",
				"<4>[2315424.100333]  panic+0x15c/0x334",
				"<4>[2315424.100519]  check_hung_uninterruptible_tasks.cold+0x34/0x48",
				"<4>[2315424.100709]  watchdog+0xad/0xb0",
				"<4>[2315424.100891]  ? check_hung_uninterruptible_tasks+0x300/0x300",
				"<4>[2315424.101072]  kthread+0x127/0x150",
				"<4>[2315424.101251]  ? set_kthread_struct+0x50/0x50",
				"<4>[2315424.101425]  ret_from_fork+0x1f/0x30",
				"<4>[2315424.101602]  </TASK>",
				"<0>[2315424.102006] Kernel Offset: 0x1be00000 from 0xffffffff81000000 (relocation range: 0xffffffff80000000-0xffffffffbfffffff)",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 24, PID: 1364, Process: khungtaskd",
			wantPID:       1364,
			wantCPU:       24,
			wantProcess:   "khungtaskd",
		},
		{
			// ref. https://github.com/bottlerocket-os/bottlerocket/issues/3087
			name: "Test Case 2 - Panic ending with reboot message",
			logLines: []string{
				"[228678.600423] INFO: task containerd-shim:931150 blocked for more than 122 seconds.",
				"[228678.606308]       Not tainted 5.15.102 #1",
				"[228678.609450] \"echo 0 > /proc/sys/kernel/hung_task_timeout_secs\" disables this message.",
				"[228678.615481] task:containerd-shim state:D stack:    0 pid:931150 ppid:     1 flags:0x00004002",
				"[228678.723352] Kernel panic - not syncing: hung_task: blocked tasks",
				"[228678.727237] CPU: 4 PID: 112 Comm: khungtaskd Not tainted 5.15.102 #1",
				"[228678.731247] Hardware name: Amazon EC2 m6i.4xlarge/, BIOS 1.0 10/16/2017",
				"[228678.735331] Call Trace:",
				"[228678.737851]  ",
				"[228678.740256]  dump_stack_lvl+0x34/0x48",
				"[228678.743282]  panic+0x100/0x2c6",
				"[228678.746075]  check_hung_uninterruptible_tasks.cold+0xc/0xc",
				"[228678.749723]  watchdog+0x9c/0xa0",
				"[228678.752526]  ? check_hung_uninterruptible_tasks+0x2c0/0x2c0",
				"[228678.756250]  kthread+0x124/0x150",
				"[228678.759088]  ? set_kthread_struct+0x50/0x50",
				"[228678.762278]  ret_from_fork+0x1f/0x30",
				"[228678.765235]  ",
				"[228678.768127] Kernel Offset: 0x32000000 from 0xffffffff81000000 (relocation range: 0xffffffff80000000-0xffffffffbfffffff)",
				"[228678.775173] Rebooting in 10 seconds..",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 4, PID: 112, Process: khungtaskd",
			wantPID:       112,
			wantCPU:       4,
			wantProcess:   "khungtaskd",
		},
		{
			// ref. https://forum.rclone.org/t/rclone-generating-kernel-panics/42378
			name: "Test Case 3 - Incomplete panic interrupted by new event",
			logLines: []string{
				"Oct 16 20:47:19 V09 kernel: Kernel panic - not syncing: hung_task: blocked tasks",
				"Oct 16 20:47:19 V09 kernel: NMI backtrace for cpu 31 skipped: idling at intel_idle+0x87/0x130",
				"Oct 16 20:47:19 V09 kernel: Sending NMI from CPU 1 to CPUs 0,2-31:",
				"Oct 16 20:47:19 V09 kernel:  ret_from_fork+0x35/0x40",
				"Oct 16 20:47:19 V09 kernel:  ? kthread_park+0x90/0x90",
				"Oct 16 20:47:19 V09 kernel:  ? hungtask_pm_notify+0x50/0x50",
				"Oct 16 20:47:19 V09 kernel:  kthread+0x121/0x140",
				"Oct 16 20:47:19 V09 kernel:  watchdog+0x2c6/0x500",
				"Oct 16 20:47:19 V09 kernel: Call Trace:",
				"Oct 16 20:47:19 V09 kernel: Hardware name: Supermicro Super Server/X10DRL-i, BIOS 3.2 11/19/2019",
				"Oct 16 20:47:19 V09 kernel: CPU: 1 PID: 210 Comm: khungtaskd Tainted: P           OE     5.4.0-149-generic #166~18.04.1-Ubuntu",
				"Oct 16 20:47:19 V09 kernel: NMI backtrace for cpu 1",
				"Oct 16 20:47:19 V09 kernel: INFO: task php-fpm:2767319 blocked for more than 120 seconds.",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 1, PID: 210, Process: khungtaskd",
			wantPID:       210,
			wantCPU:       1,
			wantProcess:   "khungtaskd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the kernel panic matcher for each test
			kernelPanicMatcher = createKernelPanicMatchFunc()

			var gotEventName, gotMessage string

			// Process each line through the matcher
			for _, line := range tt.logLines {
				eventName, message := Match(line)
				if eventName != "" {
					gotEventName = eventName
					gotMessage = message
					break // Stop once we detect the event
				}
			}

			// Verify results
			if gotEventName != tt.wantEventName {
				t.Errorf("Match() eventName = %q, want %q", gotEventName, tt.wantEventName)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Match() message = %q, want %q", gotMessage, tt.wantMessage)
			}
		})
	}
}

// TestKernelPanicHelperFunctions tests individual helper functions
func TestKernelPanicHelperFunctions(t *testing.T) {
	t.Run("checkIfStartOfPanicMessages", func(t *testing.T) {
		tests := []struct {
			line string
			want bool
		}{
			{"Kernel panic - not syncing: hung_task: blocked tasks", true},
			{"[228678.723352] Kernel panic - not syncing: hung_task: blocked tasks", true},
			{"Oct 16 20:47:19 V09 kernel: Kernel panic - not syncing: hung_task: blocked tasks", true},
			{"Kernel panic - not syncing: something else", false},
			{"Some other log message", false},
		}

		for _, tt := range tests {
			if got := checkIfStartOfPanicMessages(tt.line); got != tt.want {
				t.Errorf("checkIfStartOfPanicMessages(%q) = %v, want %v", tt.line, got, tt.want)
			}
		}
	})

	t.Run("extractCPUandPID", func(t *testing.T) {
		tests := []struct {
			line        string
			wantFound   bool
			wantCPU     int
			wantPID     int
			wantProcess string
		}{
			{
				line:        "CPU: 24 PID: 1364 Comm: khungtaskd Tainted: P           OE     5.15.0-1053-nvidia #54-Ubuntu",
				wantFound:   true,
				wantCPU:     24,
				wantPID:     1364,
				wantProcess: "khungtaskd",
			},
			{
				line:        "[228678.727237] CPU: 4 PID: 112 Comm: khungtaskd Not tainted 5.15.102 #1",
				wantFound:   true,
				wantCPU:     4,
				wantPID:     112,
				wantProcess: "khungtaskd",
			},
			{
				line:        "Oct 16 20:47:19 V09 kernel: CPU: 1 PID: 210 Comm: khungtaskd Tainted: P           OE     5.4.0-149-generic",
				wantFound:   true,
				wantCPU:     1,
				wantPID:     210,
				wantProcess: "khungtaskd",
			},
			{
				line:      "Some other log message",
				wantFound: false,
			},
		}

		for _, tt := range tests {
			instance := &KernelPanicInstance{}
			got := extractCPUandPID(tt.line, instance)
			if got != tt.wantFound {
				t.Errorf("extractCPUandPID(%q) found = %v, want %v", tt.line, got, tt.wantFound)
			}
			if got && tt.wantFound {
				if instance.CPU != tt.wantCPU {
					t.Errorf("extractCPUandPID(%q) CPU = %d, want %d", tt.line, instance.CPU, tt.wantCPU)
				}
				if instance.PID != tt.wantPID {
					t.Errorf("extractCPUandPID(%q) PID = %d, want %d", tt.line, instance.PID, tt.wantPID)
				}
				if instance.ProcessName != tt.wantProcess {
					t.Errorf("extractCPUandPID(%q) ProcessName = %q, want %q", tt.line, instance.ProcessName, tt.wantProcess)
				}
			}
		}
	})

}
