# Why GPUd

GPU operation is hard -- see [*Reliability and Operational Challenges by Meta Llama team (2024)*](https://ai.meta.com/research/publications/the-llama-3-herd-of-models/).

Complexity increases with the number of GPUs. Existing tools often fail to manage this at scale, thus a new approach for large-scale GPU operations. GPUd is designed to address several key challenges in GPU management:

1. **Automated Error Detection**: GPUd provides informational alerts that can surface bugs on a console before they become critical, reducing reliance on experienced technicians to identify errors.

2. **Simplified Workflows**: By reexamining and simplifying systems before automation, GPUd helps overcome the complexity of scenarios to be automated.

3. **Modular Design**: Each GPUd component handles a distinct and well-defined task. This approach allows for easy reuse and adaptation of key components across different GPU infrastructures.

4. **Efficient Diagnostics**: GPUd provides clear distinctions between software errors (fixable by reboots) and hardware errors (requiring component replacement), as well as identifying errors that impact performance versus those that do not.

5. **Automated Verification**: After hardware changes occur, GPUd runs automated verification processes to ensure system integrity.

6. **Comprehensive Monitoring**: GPUd actively monitors GPUs and effectively manages AI/ML workloads to ensure GPU efficiency and reliability.

7. **Data Collection for Analysis**: GPUd records raw metric data in a separate system for offline analysis, enabling weekly or monthly reports and more intricate calculations that are too complex to compute in real-time.

By addressing these challenges, GPUd simplifies GPU management, reduces human error, and improves overall system reliability and efficiency.

## Use cases

- [Lepton AI](https://lepton.ai): Collect GPU metrics and run automated verification and alerts.

## Features

- Metrics: supports time series metrics data in the custom format, in addition to the Prometheus format.
- NVIDIA GPU errors: scans dmesg, NVML, and nvidia-smi for identifying the real-time and historical GPU errors.
- NVIDIA GPU ECC errors: queries nvidia-smi and NVML APIs.
- NVIDIA GPU clock: scans nvidia-smi and NVML for hardware slowdown.
- NVIDIA GPU temperature: scans nvidia-smi and NVML for critical temperature thresholds and data.
- NVIDIA GPU power: scans nvidia-smi and NVML for current power draw and limits.
- NVIDIA GPU processes: uses NVML to list running processes.
- NVIDIA NVLink & NVSwitch: scans dmesg for any issues, NVML for status and errors.
- NVIDIA fabric manager: checks nvidia-fabricmanager unit status.
- NVIDIA InfiniBand: checks ibstat.
- NVIDIA direct RDMA (Remote Direct Memory Access): check lsmod, peermem.
- CPU, OS, memory, disk, file descriptor usage monitoring.
- Regex-based dmesg streaming and scanning.
- Workloads monitoring: supports containerd, docker, kubelet.
