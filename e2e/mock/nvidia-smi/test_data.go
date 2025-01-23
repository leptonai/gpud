package nvidia_smi

var NormalSMIOutput = `Wed Jan 22 04:28:25 2025
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 550.127.05             Driver Version: 550.127.05     CUDA Version: 12.4     |
|-----------------------------------------+------------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id          Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |           Memory-Usage | GPU-Util  Compute M. |
|                                         |                        |               MIG M. |
|=========================================+========================+======================|
|   0  NVIDIA A10                     On  |   00000000:06:00.0 Off |                    0 |
|  0%   32C    P8             21W /  150W |       1MiB /  23028MiB |      0%      Default |
|                                         |                        |                  N/A |
+-----------------------------------------+------------------------+----------------------+

+-----------------------------------------------------------------------------------------+
| Processes:                                                                              |
|  GPU   GI   CI        PID   Type   Process name                              GPU Memory |
|        ID   ID                                                               Usage      |
|=========================================================================================|
|  No running processes found                                                             |
+-----------------------------------------------------------------------------------------+
`

var NormalQueryOutput = `
==============NVSMI LOG==============

Timestamp                                 : Wed Jan 22 04:28:41 2025
Driver Version                            : 550.127.05
CUDA Version                              : 12.4

Attached GPUs                             : 1
GPU 00000000:06:00.0
    Product Name                          : NVIDIA A10
    Product Brand                         : NVIDIA
    Product Architecture                  : Ampere
    Display Mode                          : Enabled
    Display Active                        : Disabled
    Persistence Mode                      : Enabled
    Addressing Mode                       : None
    MIG Mode
        Current                           : N/A
        Pending                           : N/A
    Accounting Mode                       : Disabled
    Accounting Mode Buffer Size           : 4000
    Driver Model
        Current                           : N/A
        Pending                           : N/A
    Serial Number                         : 1324722021038
    GPU UUID                              : GPU-51ae0086-e1ac-4a20-f062-9caa44e4df6d
    Minor Number                          : 0
    VBIOS Version                         : 94.02.91.00.03
    MultiGPU Board                        : No
    Board ID                              : 0x600
    Board Part Number                     : 900-2G133-0020-100
    GPU Part Number                       : 2236-890-A1
    FRU Part Number                       : N/A
    Module ID                             : 1
    Inforom Version
        Image Version                     : G133.0215.00.06
        OEM Object                        : 2.0
        ECC Object                        : 6.16
        Power Management Object           : N/A
    Inforom BBX Object Flush
        Latest Timestamp                  : N/A
        Latest Duration                   : N/A
    GPU Operation Mode
        Current                           : N/A
        Pending                           : N/A
    GPU C2C Mode                          : N/A
    GPU Virtualization Mode
        Virtualization Mode               : Pass-Through
        Host VGPU Mode                    : N/A
        vGPU Heterogeneous Mode           : N/A
    GPU Reset Status
        Reset Required                    : No
        Drain and Reset Recommended       : No
    GSP Firmware Version                  : 550.127.05
    IBMNPU
        Relaxed Ordering Mode             : N/A
    PCI
        Bus                               : 0x06
        Device                            : 0x00
        Domain                            : 0x0000
        Base Classcode                    : 0x3
        Sub Classcode                     : 0x2
        Device Id                         : 0x223610DE
        Bus Id                            : 00000000:06:00.0
        Sub System Id                     : 0x148210DE
        GPU Link Info
            PCIe Generation
                Max                       : 4
                Current                   : 1
                Device Current            : 1
                Device Max                : 4
                Host Max                  : N/A
            Link Width
                Max                       : 16x
                Current                   : 16x
        Bridge Chip
            Type                          : N/A
            Firmware                      : N/A
        Replays Since Reset               : 0
        Replay Number Rollovers           : 0
        Tx Throughput                     : 1200 KB/s
        Rx Throughput                     : 500 KB/s
        Atomic Caps Inbound               : N/A
        Atomic Caps Outbound              : N/A
    Fan Speed                             : 0 %
    Performance State                     : P8
    Clocks Event Reasons
        Idle                              : Active
        Applications Clocks Setting       : Not Active
        SW Power Cap                      : Not Active
        HW Slowdown                       : Not Active
            HW Thermal Slowdown           : Not Active
            HW Power Brake Slowdown       : Not Active
        Sync Boost                        : Not Active
        SW Thermal Slowdown               : Not Active
        Display Clock Setting             : Not Active
    Sparse Operation Mode                 : N/A
    FB Memory Usage
        Total                             : 23028 MiB
        Reserved                          : 526 MiB
        Used                              : 1 MiB
        Free                              : 22503 MiB
    BAR1 Memory Usage
        Total                             : 32768 MiB
        Used                              : 1 MiB
        Free                              : 32767 MiB
    Conf Compute Protected Memory Usage
        Total                             : 0 MiB
        Used                              : 0 MiB
        Free                              : 0 MiB
    Compute Mode                          : Default
    Utilization
        Gpu                               : 0 %
        Memory                            : 0 %
        Encoder                           : 0 %
        Decoder                           : 0 %
        JPEG                              : 0 %
        OFA                               : 0 %
    Encoder Stats
        Active Sessions                   : 0
        Average FPS                       : 0
        Average Latency                   : 0
    FBC Stats
        Active Sessions                   : 0
        Average FPS                       : 0
        Average Latency                   : 0
    ECC Mode
        Current                           : Enabled
        Pending                           : Enabled
    ECC Errors
        Volatile
            SRAM Correctable              : 0
            SRAM Uncorrectable Parity     : 0
            SRAM Uncorrectable SEC-DED    : 0
            DRAM Correctable              : 0
            DRAM Uncorrectable            : 0
        Aggregate
            SRAM Correctable              : 0
            SRAM Uncorrectable Parity     : 0
            SRAM Uncorrectable SEC-DED    : 0
            DRAM Correctable              : 0
            DRAM Uncorrectable            : 0
            SRAM Threshold Exceeded       : No
        Aggregate Uncorrectable SRAM Sources
            SRAM L2                       : 0
            SRAM SM                       : 0
            SRAM Microcontroller          : 0
            SRAM PCIE                     : 0
            SRAM Other                    : 0
    Retired Pages
        Single Bit ECC                    : N/A
        Double Bit ECC                    : N/A
        Pending Page Blacklist            : N/A
    Remapped Rows
        Correctable Error                 : 0
        Uncorrectable Error               : 0
        Pending                           : No
        Remapping Failure Occurred        : No
        Bank Remap Availability Histogram
            Max                           : 192 bank(s)
            High                          : 0 bank(s)
            Partial                       : 0 bank(s)
            Low                           : 0 bank(s)
            None                          : 0 bank(s)
    Temperature
        GPU Current Temp                  : 33 C
        GPU T.Limit Temp                  : N/A
        GPU Shutdown Temp                 : 101 C
        GPU Slowdown Temp                 : 98 C
        GPU Max Operating Temp            : 91 C
        GPU Target Temperature            : N/A
        Memory Current Temp               : N/A
        Memory Max Operating Temp         : N/A
    GPU Power Readings
        Power Draw                        : 21.30 W
        Current Power Limit               : 150.00 W
        Requested Power Limit             : 150.00 W
        Default Power Limit               : 150.00 W
        Min Power Limit                   : 100.00 W
        Max Power Limit                   : 150.00 W
    GPU Memory Power Readings
        Power Draw                        : N/A
    Module Power Readings
        Power Draw                        : N/A
        Current Power Limit               : N/A
        Requested Power Limit             : N/A
        Default Power Limit               : N/A
        Min Power Limit                   : N/A
        Max Power Limit                   : N/A
    Clocks
        Graphics                          : 210 MHz
        SM                                : 210 MHz
        Memory                            : 405 MHz
        Video                             : 555 MHz
    Applications Clocks
        Graphics                          : 1695 MHz
        Memory                            : 6251 MHz
    Default Applications Clocks
        Graphics                          : 1695 MHz
        Memory                            : 6251 MHz
    Deferred Clocks
        Memory                            : N/A
    Max Clocks
        Graphics                          : 1695 MHz
        SM                                : 1695 MHz
        Memory                            : 6251 MHz
        Video                             : 1575 MHz
    Max Customer Boost Clocks
        Graphics                          : 1695 MHz
    Clock Policy
        Auto Boost                        : N/A
        Auto Boost Default                : N/A
    Voltage
        Graphics                          : 662.500 mV
    Fabric
        State                             : N/A
        Status                            : N/A
        CliqueId                          : N/A
        ClusterUUID                       : N/A
        Health
            Bandwidth                     : N/A
    Processes                             : None
`

var HWSlowdownQueryOutput = `
==============NVSMI LOG==============

Timestamp                                 : Wed Jan 22 04:28:41 2025
Driver Version                            : 550.127.05
CUDA Version                              : 12.4

Attached GPUs                             : 1
GPU 00000000:06:00.0
    Product Name                          : NVIDIA A10
    Product Brand                         : NVIDIA
    Product Architecture                  : Ampere
    Display Mode                          : Enabled
    Display Active                        : Disabled
    Persistence Mode                      : Enabled
    Addressing Mode                       : None
    MIG Mode
        Current                           : N/A
        Pending                           : N/A
    Accounting Mode                       : Disabled
    Accounting Mode Buffer Size           : 4000
    Driver Model
        Current                           : N/A
        Pending                           : N/A
    Serial Number                         : 1324722021038
    GPU UUID                              : GPU-51ae0086-e1ac-4a20-f062-9caa44e4df6d
    Minor Number                          : 0
    VBIOS Version                         : 94.02.91.00.03
    MultiGPU Board                        : No
    Board ID                              : 0x600
    Board Part Number                     : 900-2G133-0020-100
    GPU Part Number                       : 2236-890-A1
    FRU Part Number                       : N/A
    Module ID                             : 1
    Inforom Version
        Image Version                     : G133.0215.00.06
        OEM Object                        : 2.0
        ECC Object                        : 6.16
        Power Management Object           : N/A
    Inforom BBX Object Flush
        Latest Timestamp                  : N/A
        Latest Duration                   : N/A
    GPU Operation Mode
        Current                           : N/A
        Pending                           : N/A
    GPU C2C Mode                          : N/A
    GPU Virtualization Mode
        Virtualization Mode               : Pass-Through
        Host VGPU Mode                    : N/A
        vGPU Heterogeneous Mode           : N/A
    GPU Reset Status
        Reset Required                    : No
        Drain and Reset Recommended       : No
    GSP Firmware Version                  : 550.127.05
    IBMNPU
        Relaxed Ordering Mode             : N/A
    PCI
        Bus                               : 0x06
        Device                            : 0x00
        Domain                            : 0x0000
        Base Classcode                    : 0x3
        Sub Classcode                     : 0x2
        Device Id                         : 0x223610DE
        Bus Id                            : 00000000:06:00.0
        Sub System Id                     : 0x148210DE
        GPU Link Info
            PCIe Generation
                Max                       : 4
                Current                   : 1
                Device Current            : 1
                Device Max                : 4
                Host Max                  : N/A
            Link Width
                Max                       : 16x
                Current                   : 16x
        Bridge Chip
            Type                          : N/A
            Firmware                      : N/A
        Replays Since Reset               : 0
        Replay Number Rollovers           : 0
        Tx Throughput                     : 1200 KB/s
        Rx Throughput                     : 500 KB/s
        Atomic Caps Inbound               : N/A
        Atomic Caps Outbound              : N/A
    Fan Speed                             : 0 %
    Performance State                     : P8
    Clocks Event Reasons
        Idle                              : Active
        Applications Clocks Setting       : Not Active
        SW Power Cap                      : Not Active
        HW Slowdown                       : Active
            HW Thermal Slowdown           : Not Active
            HW Power Brake Slowdown       : Active
        Sync Boost                        : Not Active
        SW Thermal Slowdown               : Not Active
        Display Clock Setting             : Not Active
    Sparse Operation Mode                 : N/A
    FB Memory Usage
        Total                             : 23028 MiB
        Reserved                          : 526 MiB
        Used                              : 1 MiB
        Free                              : 22503 MiB
    BAR1 Memory Usage
        Total                             : 32768 MiB
        Used                              : 1 MiB
        Free                              : 32767 MiB
    Conf Compute Protected Memory Usage
        Total                             : 0 MiB
        Used                              : 0 MiB
        Free                              : 0 MiB
    Compute Mode                          : Default
    Utilization
        Gpu                               : 0 %
        Memory                            : 0 %
        Encoder                           : 0 %
        Decoder                           : 0 %
        JPEG                              : 0 %
        OFA                               : 0 %
    Encoder Stats
        Active Sessions                   : 0
        Average FPS                       : 0
        Average Latency                   : 0
    FBC Stats
        Active Sessions                   : 0
        Average FPS                       : 0
        Average Latency                   : 0
    ECC Mode
        Current                           : Enabled
        Pending                           : Enabled
    ECC Errors
        Volatile
            SRAM Correctable              : 0
            SRAM Uncorrectable Parity     : 0
            SRAM Uncorrectable SEC-DED    : 0
            DRAM Correctable              : 0
            DRAM Uncorrectable            : 0
        Aggregate
            SRAM Correctable              : 0
            SRAM Uncorrectable Parity     : 0
            SRAM Uncorrectable SEC-DED    : 0
            DRAM Correctable              : 0
            DRAM Uncorrectable            : 0
            SRAM Threshold Exceeded       : No
        Aggregate Uncorrectable SRAM Sources
            SRAM L2                       : 0
            SRAM SM                       : 0
            SRAM Microcontroller          : 0
            SRAM PCIE                     : 0
            SRAM Other                    : 0
    Retired Pages
        Single Bit ECC                    : N/A
        Double Bit ECC                    : N/A
        Pending Page Blacklist            : N/A
    Remapped Rows
        Correctable Error                 : 0
        Uncorrectable Error               : 0
        Pending                           : No
        Remapping Failure Occurred        : No
        Bank Remap Availability Histogram
            Max                           : 192 bank(s)
            High                          : 0 bank(s)
            Partial                       : 0 bank(s)
            Low                           : 0 bank(s)
            None                          : 0 bank(s)
    Temperature
        GPU Current Temp                  : 33 C
        GPU T.Limit Temp                  : N/A
        GPU Shutdown Temp                 : 101 C
        GPU Slowdown Temp                 : 98 C
        GPU Max Operating Temp            : 91 C
        GPU Target Temperature            : N/A
        Memory Current Temp               : N/A
        Memory Max Operating Temp         : N/A
    GPU Power Readings
        Power Draw                        : 21.30 W
        Current Power Limit               : 150.00 W
        Requested Power Limit             : 150.00 W
        Default Power Limit               : 150.00 W
        Min Power Limit                   : 100.00 W
        Max Power Limit                   : 150.00 W
    GPU Memory Power Readings
        Power Draw                        : N/A
    Module Power Readings
        Power Draw                        : N/A
        Current Power Limit               : N/A
        Requested Power Limit             : N/A
        Default Power Limit               : N/A
        Min Power Limit                   : N/A
        Max Power Limit                   : N/A
    Clocks
        Graphics                          : 210 MHz
        SM                                : 210 MHz
        Memory                            : 405 MHz
        Video                             : 555 MHz
    Applications Clocks
        Graphics                          : 1695 MHz
        Memory                            : 6251 MHz
    Default Applications Clocks
        Graphics                          : 1695 MHz
        Memory                            : 6251 MHz
    Deferred Clocks
        Memory                            : N/A
    Max Clocks
        Graphics                          : 1695 MHz
        SM                                : 1695 MHz
        Memory                            : 6251 MHz
        Video                             : 1575 MHz
    Max Customer Boost Clocks
        Graphics                          : 1695 MHz
    Clock Policy
        Auto Boost                        : N/A
        Auto Boost Default                : N/A
    Voltage
        Graphics                          : 662.500 mV
    Fabric
        State                             : N/A
        Status                            : N/A
        CliqueId                          : N/A
        ClusterUUID                       : N/A
        Health
            Bandwidth                     : N/A
    Processes                             : None
`
