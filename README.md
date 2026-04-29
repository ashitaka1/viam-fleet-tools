# avery:fleet-tools

Viam module providing fleet-operator tooling for Linux machines.

## Models

### `avery:fleet-tools:workload` (sensor)

Per-poll snapshot of system load — CPU, memory, disk I/O, network throughput,
RAPL package power, CPU temperatures, GPU (NVIDIA), top-N processes, system
info — rich enough to characterize any workload against any target Linux
machine. Reads `/proc`, `/sys`, gopsutil, and `nvidia-smi`; degrades silently
on hosts missing any of those sources (the corresponding keys are simply
omitted from the reading).

#### Configuration

```json
{
  "top_n_processes": 3,
  "extra_disk_devices": []
}
```

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `top_n_processes` | `int` | `3` | How many processes to include in `top_procs_by_cpu` and `top_procs_by_mem`. `0` keeps the default. |
| `extra_disk_devices` | `[]string` | `[]` | Reserved for future per-device opt-in; all gopsutil-reported devices are emitted by default. |

#### Readings

Floating-point values are rounded to 2 decimal places; `mem_*_mb` are rounded
to whole MB. Keys for thermal / RAPL / GPU are omitted on hosts without those
sources.

| Key | Type | Notes |
|-----|------|-------|
| `cpu_pct_avg`, `cpu_pct_max_core`, `cpu_pct_per_core` | float / []float | |
| `cpu_freq_mhz_avg`, `cpu_freq_mhz_max` | float | |
| `cpu_temp_max_c`, `cpu_temp_avg_c`, `cpu_temp_per_core_c` | float / []float | Source priority: `coretemp > zenpower > k10temp > thermal_zone(cpu_thermal) > thermal_zone(x86_pkg_temp)`. AMD `Tctl` is excluded (carries +20°C offset). |
| `mem_used_mb`, `mem_available_mb`, `mem_total_mb` | int | |
| `swap_used_mb`, `swap_in_mb_per_sec`, `swap_out_mb_per_sec` | float | |
| `disk_read_mbps`, `disk_write_mbps`, `disk_read_iops`, `disk_write_iops`, `disk_util_pct` | map[string]float | per device |
| `net_rx_mbps`, `net_tx_mbps`, `net_rx_pps`, `net_tx_pps` | map[string]float | per interface, `lo` skipped |
| `pkg_watts` | float | RAPL package power, sums all package domains |
| `gpu_util_pct`, `gpu_mem_mb`, `gpu_watts` | float | NVIDIA via `nvidia-smi`; omitted otherwise |
| `top_procs_by_cpu`, `top_procs_by_mem` | []object | `{pid, name, cpu_pct, mem_mb}`; ties broken by smaller pid |
| `load_avg_1`, `load_avg_5`, `load_avg_15` | float | |
| `uptime_sec` | int | |
| `timestamp` | string | RFC3339Nano UTC |

## Platforms

`linux/amd64`, `linux/arm64`.
