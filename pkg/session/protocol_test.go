// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import "testing"

func TestParseProtocol(t *testing.T) {
	tests := []struct {
		value string
		want  Protocol
		ok    bool
	}{
		{value: "", want: ProtocolV1, ok: true},
		{value: "v1", want: ProtocolV1, ok: true},
		{value: "v2", want: ProtocolV2, ok: true},
		{value: "auto", want: ProtocolAuto, ok: true},
		{value: "future", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseProtocol(tt.value)
			if (err == nil) != tt.ok {
				t.Fatalf("parseProtocol(%q) error = %v, ok = %v", tt.value, err, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("parseProtocol(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
