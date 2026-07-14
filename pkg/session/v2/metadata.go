// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package sessionv2

const (
	ProtocolRevision = 1

	MetadataAuthorization = "authorization"
	MetadataMachineID     = "x-gpud-machine-id"
	MetadataMachineProof  = "x-gpud-machine-proof"
)

const (
	DefaultMaxMessageBytes       = 8 * 1024 * 1024
	DefaultHeartbeatIntervalSecs = 30
)
