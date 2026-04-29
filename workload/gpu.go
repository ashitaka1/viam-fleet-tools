package workload

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

func sampleGPU(ctx context.Context) map[string]any {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return map[string]any{}
	}
	cmd := exec.CommandContext(
		ctx, "nvidia-smi",
		"--query-gpu=utilization.gpu,memory.used,power.draw",
		"--format=csv,noheader,nounits",
	)
	out, err := cmd.Output()
	if err != nil {
		return map[string]any{}
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if line == "" {
		return map[string]any{}
	}
	parts := strings.Split(line, ",")
	if len(parts) != 3 {
		return map[string]any{}
	}
	util, errUtil := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	memMB, errMem := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	watts, errW := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if errUtil != nil || errMem != nil || errW != nil {
		return map[string]any{}
	}
	return map[string]any{
		"gpu_util_pct": util,
		"gpu_mem_mb":   memMB,
		"gpu_watts":    watts,
	}
}
