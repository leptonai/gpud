# SXid 聚合指标设计与实现

## 0. 一句话

给 sxid 补上 xid 已有的 Prometheus 计量出口：

```
accelerator_nvidia_sxid_errors_total{gpud_component, sxid, device_pci} 42
```

出口示例（某 H100 节点：两颗 switch 同报 12028，其一随后又报了 20034）：

```
accelerator_nvidia_sxid_errors_total{device_pci="PCI:0000:05:00.0", gpud_component="accelerator-nvidia-error-sxid", sxid="12028"} 1
accelerator_nvidia_sxid_errors_total{device_pci="PCI:0000:06:00.0", gpud_component="accelerator-nvidia-error-sxid", sxid="12028"} 1
accelerator_nvidia_sxid_errors_total{device_pci="PCI:0000:05:00.0", gpud_component="accelerator-nvidia-error-sxid", sxid="20034"} 3
```

正常态一个节点 0 条序列（无事件不创建）；报错时每个 (switch, 码) 组合一条，值为去重后事件的累计数。

与 xid 的 `accelerator_nvidia_xid_errors_total{gpud_component, uuid, xid}` 完全对等：
xid 每张 GPU 一条序列（8/16 张），sxid 每颗 NVSwitch 一条序列（4 颗）。

---

## 1. 当前 sxid 相比 xid 缺失的逻辑

### 1.1 两个组件是孪生结构

sxid 和 xid 组件的采集流水线一模一样：

```
/dev/kmsg → 逐行正则匹配 → eventBucket.Find 同事件去重
          → eventBucket.Insert（SQLite 事件库）
          → ┬─ xid:  metricXIDErrs.With(...).Inc()   ← 计数出口 ✅
            └─ sxid: 无                                ← 缺失 ❌
```

xid 的计数出口就五行（`components/accelerator/nvidia/xid/component.go`）：

```go
if err = c.eventBucket.Insert(c.ctx, event); err != nil { ... }
logger.Infow("inserted the event successfully")
metricXIDErrs.With(prometheus.Labels{
    "uuid": convertBusIDToUUID(xidErr.DeviceUUID, c.devices),
    "xid":  strconv.Itoa(xidErr.Xid),
}).Inc()
```

配合 `xid/metrics.go` 里的 `accelerator_nvidia_xid_errors_total` CounterVec 定义。
sxid 走到完全相同的位置——事件已入库、码值/时间全在手里——然后没有下一行。

**计数点与"秒折叠"**：事件库的去重键 = (内核日志秒, name, type, **ExtraInfo 全等**)，
sxid 事件带 `ExtraInfo{data, device_uuid}`，所以同秒同 switch 同码的重复行折叠成 1、
不同 switch/码各自计数（xid 同理——.241 的 +6 就是 6 张卡各一次）。本改动不改这部分，
把计数点放在 `Insert` 成功后即可继承该折叠（重启 lookback 重扫也不会重复计数）；
消费侧 `increase(...)[60s] ∈ [0, 60]` = "该 switch 该分钟里有几秒在报错"。

### 1.2 采集端不缺，缺的是出口

sxid 的采集端反而是最细的：

- 逐行读 `/dev/kmsg`，正则提取 SXid 码 + PCI 设备号 + 时间戳
- 查码表（`sxid.go` details catalog：名称、fatal 分类、修复建议）
- 与 NVSentinel fabric 事件交叉验证去重（v0.13.0）
- 事件连完整上下文存入 SQLite 事件库

但这些精细数据的出口只有三个，全部不通往 Prometheus：

| 出口 | 内容 | vmagent 能吃到吗 |
|---|---|---|
| `/v1/states` | 健康判断，`"matched %d sxid errors from %d kmsg(s)"`——只有个数，没有码值 | ❌ JSON，NPD 脚本消费 |
| `/v1/events` | 事件明细（码 + uuid + 时间） | ❌ JSON API，无人调用 |
| `gpud scan` | CLI 表格 | ❌ 人工排查用 |
| pod 日志 | 完整原始内核行 | ❌（除非进 Loki） |
| **`/metrics`** | **sxid = 0 条** | ✅ 唯一 vmagent 吃的出口 |

**结论：缺的不是采集能力，是"采集到的粒度没有一条通往 /metrics 的路"。**
本改动就是在采集已完成的位置（Insert 成功处）补上这条出口。

---

## 2. 维度选择的依据

### 2.1 两条判据

指标维度取舍只有两条判据：

1. **基数有界**——唯一标签组合数决定 TSDB 索引成本（样本量不是问题：counter 下
   一万次 Inc 只是一个序列的一次跳变）
2. **真的会按它查/告警/分组**——不为"以后可能"付永久的索引代价（死序列在索引里
   躺满整个 retention 期）

推论：**聚合可以交给上游做，维度上游造不出来。** 源头没导出的维度，以后想要就得
再改代码发版；而已导出的维度，recording rule / 查询时随时可以 sum/by 收敛。

### 2.2 与 xid 对等原则

xid 的设备维度是 `uuid`：每节点 8 或 16 张 GPU → 8/16 个序列。
sxid 的设备维度取 `device_pci`：每节点 4 颗 NVSwitch → 4 个序列。
**设备粒度对等，告警和看板的语义就对称。**

### 2.3 逐维度决策

| 维度 | 判决 | 依据 |
|---|---|---|
| `sxid`（码值） | ✅ 保留 | 告警按码分级的基础，内核行直接可提 |
| `severity` | ❌ 排除 | 内核行缺 Non-fatal/Fatal 关键字的情况不少（如 `20034, Data {0x...}` 载荷行），缺省填 non_fatal 等于编造数据；且严重级别本来就是码的函数——指标里已有 `sxid` 码，消费侧按码查 FM guide 码表即可定级，severity 不是独立信息 |
| `device_pci` | ✅ 保留 | switch 身份，值保留内核行原文含 `PCI:` 前缀（如 `PCI:0000:05:00.0`）——自描述、与事件库 `device_uuid`/内核日志同形。注意 link_id 是**每 switch 的本地编号**——switch0 的 Link 30 和 switch1 的 Link 30 是两根不同的线，没有 switch 身份，链路级分析会把 4 颗 switch 混在一起 |
| `gpud_component` | ✅ 保留 | gpud 全局约定（`pkg/metrics/types.go:9`），与 xid 计数器一致 |
| `link_id` | ❌ 排除 → 日志 | 有界（每 switch ≤64）但与 device_pci 强相关；跨链路成簇分析属下一层分辨率，完整内核行已在 pod 日志（"got sxid event"），取证去日志 |
| `engine_id` | ❌ 排除 → 日志 | egress/link engine 与端口基本一一对应，是 link_id 的冗余投影；对外无解码表 |
| `register_data` | ❌ 排除，绝不进指标 | **无界基数工厂**：`Data {0x...}` 寄存器快照随 HW 现场千变万化，进 label 意味着一次风暴制造数千死序列躺满 retention。它是"现场照片"不是"位置"，归日志 |

### 2.4 无事件 = 无序列

sxid 稀有：正常态指标基数是**零**（没有事件时序列根本不创建）；风暴期单节点序列数
= 真实报错的 (switch, 码) 组合数，通常个位数，理论封顶 4×码数 量级。

---

## 3. 具体代码

本次改动就两件事：**增加 sxid 的 metric 统计，并把指标注册上去**。

- 新增 `metrics.go`：定义 `accelerator_nvidia_sxid_errors_total{gpud_component, sxid, device_pci}`，
  在 `init()` 里注册进 Prometheus registry——注册之后 `/metrics` 端点自动暴露
- `component.go` +11 行（含注释）：两个采集入口（kmsg、NVSentinel）在事件 `Insert` 成功后各调一行
  `recordSXIDErrsMetric(...)`

除此之外不动任何现有逻辑：事件构造、去重、kmsg 解析均与 main 一致（`metrics_test.go` 为
配套新测试；`kmsg.go`/`kmsg_test.go` 零改动，severity 维度不进指标，见 §2.3）。

### 3.1 `metrics.go`（新增文件）

```go
var (
    componentLabel = prometheus.Labels{
        pkgmetrics.MetricComponentLabelKey: Name,
    }

    // Severity is intentionally not a label: kernel lines often lack the
    // Non-fatal/Fatal keyword (e.g. "SXid ...: 20034, Data {0x...}"), so a
    // per-line severity would be partly invented. Severity is a function of
    // the code anyway — grade by the sxid label against the fabric-manager
    // guide matrix.
    metricSXIDErrs = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "",
            Subsystem: SubSystem, // "accelerator_nvidia_sxid"
            Name:      "errors_total",
            Help: "counts SXID error events per SXID code and reporting NVSwitch " +
                "(device_pci). Repeats of the same switch and code within the same " +
                "kernel-log second collapse into one count. Link- and engine-level details " +
                "stay in the raw kernel log line, which GPUd logs on every match.",
        },
        []string{pkgmetrics.MetricComponentLabelKey,
            "sxid",       // label is the SXID error code
            "device_pci", // label is the PCI address of the reporting NVSwitch
        },
    ).MustCurryWith(componentLabel)
)

func recordSXIDErrsMetric(sxid int, devicePCI string) {
    metricSXIDErrs.With(prometheus.Labels{
        "sxid":       strconv.Itoa(sxid),
        "device_pci": devicePCI,
    }).Inc()
}
```

### 3.2 `component.go`（两处计数调用，+7 行）

```go
// 路径一：kmsg。Insert 成功后计数——xid 的 metricXIDErrs.With(...).Inc() 就在同一位置。
// DeviceUUID 原样进标签：保留内核行的 "PCI:" 前缀，与事件库 device_uuid 同形、自描述
// （devicePCI = sxidErr.DeviceUUID）
logger.Infow("inserted the event successfully")
recordSXIDErrsMetric(sxidErr.SXid, devicePCI)   // ← 新增

// 路径二：NVSentinel fabric 事件。switchPCI 已归一化为裸地址，补回内核同款前缀。
if sxidNum, _, switchPCI, ok := c.matchNVSentinelSXid(*pending.nvsEvent); ok {
    recordSXIDErrsMetric(sxidNum, "PCI:"+switchPCI)   // ← 新增
}
```
