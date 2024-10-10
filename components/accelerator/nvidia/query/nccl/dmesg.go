// Package nccl contains the implementation of the NCCL (NVIDIA Collective Communications Library) query for NVIDIA GPUs.
package nccl

// repeated messages may indicate GPU communication issues, which may happen due to fabric manager issues
// e.g.,
// [Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]
const RegexSegfaultInLibnccl = `.*segfault at.*in libnccl\.so.*`
