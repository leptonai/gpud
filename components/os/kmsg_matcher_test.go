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
		{
			name: "Test Case 4 - VFS Unable to mount root fs",
			logLines: []string{
				"[   0.123456] Kernel panic - not syncing: VFS: Unable to mount root fs on unknown-block(0,0)",
				"[   0.123457] CPU: 0 PID: 1 Comm: swapper/0 Not tainted 5.15.0-1053-gcp #61-Ubuntu",
				"[   0.123458] Hardware name: Google Compute Engine/Google Compute Engine, BIOS Google 02/16/2023",
				"[   0.123459] Call Trace:",
				"[   0.123460]  <TASK>",
				"[   0.123461]  dump_stack_lvl+0x48/0x70",
				"[   0.123462]  panic+0x118/0x2d2",
				"[   0.123463]  mount_block_root+0x2a2/0x2c0",
				"[   0.123464]  mount_root+0x9e/0xa8",
				"[   0.123465]  prepare_namespace+0x14d/0x17c",
				"[   0.123466]  kernel_init_freeable+0x201/0x234",
				"[   0.123467]  ? rest_init+0xb0/0xb0",
				"[   0.123468]  kernel_init+0x16/0x120",
				"[   0.123469]  ret_from_fork+0x1f/0x30",
				"[   0.123470]  </TASK>",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 0, PID: 1, Process: swapper/0",
			wantPID:       1,
			wantCPU:       0,
			wantProcess:   "swapper/0",
		},
		{
			name: "Test Case 5 - Kernel Panic with capital P (VFS)",
			logLines: []string{
				"[   0.123456] Kernel Panic - not syncing: VFS: Unable to mount root fs on unknown-block(8,1)",
				"[   0.123457] CPU: 2 PID: 15 Comm: init Not tainted 5.4.0-74-generic #83-Ubuntu",
				"[   0.123458] Hardware name: Dell Inc. PowerEdge R640/02CYK9, BIOS 2.10.2 11/15/2021",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 2, PID: 15, Process: init",
			wantPID:       15,
			wantCPU:       2,
			wantProcess:   "init",
		},
		{
			name: "Test Case 6 - NMI Not continuing",
			logLines: []string{
				"[  123.456789] Kernel panic - not syncing: NMI: Not continuing",
				"[  123.456790] CPU: 8 PID: 0 Comm: swapper/8 Not tainted 5.15.0-56-generic #62-Ubuntu",
				"[  123.456791] Hardware name: Supermicro SYS-2029P-C1R/X11DDW-L, BIOS 3.2 01/15/2020",
				"[  123.456792] Call Trace:",
				"[  123.456793]  <NMI>",
				"[  123.456794]  dump_stack_lvl+0x58/0x7a",
				"[  123.456795]  panic+0x107/0x294",
				"[  123.456796]  nmi_panic+0x40/0x40",
				"[  123.456797]  io_check_error+0x3c/0x50",
				"[  123.456798]  default_do_nmi+0x40/0x100",
				"[  123.456799]  exc_nmi+0x7c/0xa0",
				"[  123.456800]  end_repeat_nmi+0x16/0x1e",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 8, PID: 0, Process: swapper/8",
			wantPID:       0,
			wantCPU:       8,
			wantProcess:   "swapper/8",
		},
		{
			name: "Test Case 7 - Out of memory panic_on_oom",
			logLines: []string{
				"[  789.012345] Kernel panic - not syncing: out of memory. panic_on_oom is selected",
				"[  789.012346] CPU: 16 PID: 42 Comm: kswapd0 Not tainted 5.15.0-72-generic #79-Ubuntu",
				"[  789.012347] Hardware name: ASUSTeK COMPUTER INC. ESC8000G4/Z11PA-D8, BIOS 6101 05/23/2018",
				"[  789.012348] Call Trace:",
				"[  789.012349]  <TASK>",
				"[  789.012350]  dump_stack_lvl+0x58/0x7a",
				"[  789.012351]  panic+0x107/0x294",
				"[  789.012352]  out_of_memory+0x608/0x6a0",
				"[  789.012353]  __alloc_pages_slowpath.constprop.0+0xd06/0xd40",
				"[  789.012354]  __alloc_pages+0x2d2/0x300",
				"[  789.012355]  alloc_pages+0x8a/0x110",
				"[  789.012356]  allocate_slab+0x27b/0x3a0",
				"[  789.012357]  ___slab_alloc+0x3fb/0x5c0",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 16, PID: 42, Process: kswapd0",
			wantPID:       42,
			wantCPU:       16,
			wantProcess:   "kswapd0",
		},
		{
			name: "Test Case 8 - Fatal Machine check",
			logLines: []string{
				"[  456.789012] Kernel panic - not syncing: Fatal Machine check",
				"[  456.789013] CPU: 3 PID: 1234 Comm: systemd Tainted: G   M       5.15.0-58-generic #64-Ubuntu",
				"[  456.789014] Hardware name: HP ProLiant DL380 Gen10/ProLiant DL380 Gen10, BIOS U30 06/20/2018",
				"[  456.789015] Call Trace:",
				"[  456.789016]  <TASK>",
				"[  456.789017]  dump_stack_lvl+0x58/0x7a",
				"[  456.789018]  panic+0x107/0x294",
				"[  456.789019]  mce_panic+0x269/0x2a0",
				"[  456.789020]  mce_reign+0x2e5/0x340",
				"[  456.789021]  mce_end+0x104/0x3a0",
				"[  456.789022]  __machine_check_poll+0x339/0x340",
				"[  456.789023]  machine_check_poll+0x47/0x60",
			},
			wantEventName: eventNameKernelPanic,
			wantMessage:   "Kernel panic detected - CPU: 3, PID: 1234, Process: systemd",
			wantPID:       1234,
			wantCPU:       3,
			wantProcess:   "systemd",
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
			// hung_task pattern
			{"Kernel panic - not syncing: hung_task: blocked tasks", true},
			{"[228678.723352] Kernel panic - not syncing: hung_task: blocked tasks", true},
			{"Oct 16 20:47:19 V09 kernel: Kernel panic - not syncing: hung_task: blocked tasks", true},

			// VFS pattern (lowercase kernel)
			{"Kernel panic - not syncing: VFS: Unable to mount root fs on unknown-block(0,0)", true},
			{"[   0.123456] Kernel panic - not syncing: VFS: Unable to mount root fs on unknown-block(8,1)", true},

			// VFS pattern (uppercase Kernel Panic)
			{"Kernel Panic - not syncing: VFS: Unable to mount root fs on unknown-block(0,0)", true},
			{"[   0.123456] Kernel Panic - not syncing: VFS: Unable to mount root fs on unknown-block(8,1)", true},

			// NMI pattern
			{"Kernel panic - not syncing: NMI: Not continuing", true},
			{"[  123.456789] Kernel panic - not syncing: NMI: Not continuing", true},

			// OOM pattern
			{"Kernel panic - not syncing: out of memory. panic_on_oom is selected", true},
			{"[  789.012345] Kernel panic - not syncing: out of memory. panic_on_oom is selected", true},

			// Fatal Machine check pattern
			{"Kernel panic - not syncing: Fatal Machine check", true},
			{"[  456.789012] Kernel panic - not syncing: Fatal Machine check", true},

			// Non-matching patterns
			{"Kernel panic - not syncing: something else", true}, // matches Kernel [Pp]anic regex
			{"Some other log message", false},
			{"panic: something", false},
			{"VFS: Unable to mount root fs", false},                    // missing "Kernel panic" prefix
			{"Kernel panic - syncing: hung_task: blocked tasks", true}, // matches Kernel [Pp]anic regex
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
			// Valid CPU/PID lines
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
				line:        "CPU: 0 PID: 1 Comm: swapper/0 Not tainted 5.15.0-1053-gcp #61-Ubuntu",
				wantFound:   true,
				wantCPU:     0,
				wantPID:     1,
				wantProcess: "swapper/0",
			},
			{
				line:        "CPU: 8 PID: 0 Comm: swapper/8 Not tainted 5.15.0-56-generic #62-Ubuntu",
				wantFound:   true,
				wantCPU:     8,
				wantPID:     0,
				wantProcess: "swapper/8",
			},
			{
				line:        "CPU: 16 PID: 42 Comm: kswapd0 Not tainted 5.15.0-72-generic #79-Ubuntu",
				wantFound:   true,
				wantCPU:     16,
				wantPID:     42,
				wantProcess: "kswapd0",
			},
			{
				line:        "CPU: 3 PID: 1234 Comm: systemd Tainted: G   M       5.15.0-58-generic #64-Ubuntu",
				wantFound:   true,
				wantCPU:     3,
				wantPID:     1234,
				wantProcess: "systemd",
			},
			// Process names with special characters
			{
				line:        "CPU: 1 PID: 210 Comm: khungtaskd Tainted: P           OE     5.4.0-149-generic #166~18.04.1-Ubuntu",
				wantFound:   true,
				wantCPU:     1,
				wantPID:     210,
				wantProcess: "khungtaskd",
			},
			{
				line:        "CPU: 2 PID: 15 Comm: init Not tainted 5.4.0-74-generic #83-Ubuntu",
				wantFound:   true,
				wantCPU:     2,
				wantPID:     15,
				wantProcess: "init",
			},
			// Invalid lines - missing required parts
			{
				line:      "Some other log message",
				wantFound: false,
			},
			{
				line:      "CPU: 24 Comm: khungtaskd", // Missing PID
				wantFound: false,
			},
			{
				line:      "PID: 1364 Comm: khungtaskd", // Missing CPU
				wantFound: false,
			},
			{
				line:      "CPU: 24 PID: 1364", // Missing Comm
				wantFound: false,
			},
			// Invalid number formats
			{
				line:      "CPU: abc PID: 1364 Comm: khungtaskd",
				wantFound: false,
			},
			{
				line:      "CPU: 24 PID: xyz Comm: khungtaskd",
				wantFound: false,
			},
			// Edge cases with large numbers
			{
				line:        "CPU: 999 PID: 999999 Comm: test-process Not tainted 5.15.0",
				wantFound:   true,
				wantCPU:     999,
				wantPID:     999999,
				wantProcess: "test-process",
			},
		}

		for _, tt := range tests {
			instance := &KernelPanicInstance{}
			got := extractCPUandPID(tt.line, instance)

			if got != tt.wantFound {
				t.Errorf("extractCPUandPID(%q) found = %v, want %v", tt.line, got, tt.wantFound)
				continue
			}

			if tt.wantFound {
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

// TestKernelPanicInstanceSummary tests the Summary method of KernelPanicInstance
func TestKernelPanicInstanceSummary(t *testing.T) {
	tests := []struct {
		name     string
		instance *KernelPanicInstance
		want     string
	}{
		{
			name: "valid instance",
			instance: &KernelPanicInstance{
				CPU:         24,
				PID:         1364,
				ProcessName: "khungtaskd",
			},
			want: "Kernel panic detected - CPU: 24, PID: 1364, Process: khungtaskd",
		},
		{
			name: "instance with swapper process",
			instance: &KernelPanicInstance{
				CPU:         0,
				PID:         0,
				ProcessName: "swapper/0",
			},
			want: "Kernel panic detected - CPU: 0, PID: 0, Process: swapper/0",
		},
		{
			name:     "nil instance",
			instance: nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.instance.Summary()
			if got != tt.want {
				t.Errorf("KernelPanicInstance.Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestKernelPanicStatefulMatcher tests that the matcher correctly maintains state
func TestKernelPanicStatefulMatcher(t *testing.T) {
	tests := []struct {
		name     string
		scenario []struct {
			line          string
			wantEventName string
			wantMessage   string
		}
	}{
		{
			name: "Multiple panic events in sequence",
			scenario: []struct {
				line          string
				wantEventName string
				wantMessage   string
			}{
				{"Kernel panic - not syncing: hung_task: blocked tasks", "", ""},
				{"CPU: 1 PID: 100 Comm: test1", eventNameKernelPanic, "Kernel panic detected - CPU: 1, PID: 100, Process: test1"},
				{"Some other log", "", ""},
				{"Kernel panic - not syncing: NMI: Not continuing", "", ""},
				{"CPU: 2 PID: 200 Comm: test2", eventNameKernelPanic, "Kernel panic detected - CPU: 2, PID: 200, Process: test2"},
			},
		},
		{
			name: "Panic without CPU/PID info",
			scenario: []struct {
				line          string
				wantEventName string
				wantMessage   string
			}{
				{"Kernel panic - not syncing: Fatal Machine check", "", ""},
				{"Some other log without CPU/PID", "", ""},
				{"Another log", "", ""},
			},
		},
		{
			name: "Interrupted panic sequence",
			scenario: []struct {
				line          string
				wantEventName string
				wantMessage   string
			}{
				{"Kernel panic - not syncing: out of memory. panic_on_oom is selected", "", ""},
				{"Some log", "", ""},
				{"Kernel panic - not syncing: hung_task: blocked tasks", eventNameKernelPanic, "Kernel panic detected (no CPU/PID info found)"}, // New panic starts, returns previous incomplete panic
				{"CPU: 5 PID: 500 Comm: test3", eventNameKernelPanic, "Kernel panic detected - CPU: 5, PID: 500, Process: test3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the matcher for each test
			kernelPanicMatcher = createKernelPanicMatchFunc()

			for i, step := range tt.scenario {
				gotEventName, gotMessage := Match(step.line)
				if gotEventName != step.wantEventName {
					t.Errorf("Step %d: Match(%q) eventName = %q, want %q", i, step.line, gotEventName, step.wantEventName)
				}
				if gotMessage != step.wantMessage {
					t.Errorf("Step %d: Match(%q) message = %q, want %q", i, step.line, gotMessage, step.wantMessage)
				}
			}
		})
	}
}
