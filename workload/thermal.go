package workload

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type thermalSource struct {
	dir         string
	coreInputs  []string
	singleInput string
}

// resolveTempSource picks the most informative thermal source available,
// preferring per-core readings over package-level. Priority:
//
//  1. hwmon name=coretemp           (Intel)
//  2. hwmon name=zenpower           (AMD Zen2+ out-of-tree, exposes Tccd*)
//  3. hwmon name=k10temp            (AMD in-tree)
//  4. thermal_zone type=cpu_thermal (ARM SoCs)
//  5. thermal_zone type=x86_pkg_temp
//
// Returns nil with no error when no recognized source is present.
func resolveTempSource(rootSys string) (*thermalSource, error) {
	if src, err := scanHwmon(rootSys); err != nil {
		return nil, err
	} else if src != nil {
		return src, nil
	}
	return scanThermalZones(rootSys)
}

func scanHwmon(rootSys string) (*thermalSource, error) {
	type rule struct {
		name   string
		accept func(label string) bool
	}
	rules := []rule{
		{"coretemp", func(l string) bool { return strings.HasPrefix(l, "Core ") }},
		{"zenpower", func(l string) bool { return l == "Tdie" || strings.HasPrefix(l, "Tccd") }},
		{"k10temp", func(l string) bool { return l == "Tdie" || strings.HasPrefix(l, "Tccd") }},
	}
	hwmonRoot := filepath.Join(rootSys, "class", "hwmon")
	entries, _ := os.ReadDir(hwmonRoot)
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })

	for _, r := range rules {
		for _, e := range entries {
			dir := filepath.Join(hwmonRoot, e.Name())
			name, _ := readTrimmed(filepath.Join(dir, "name"))
			if name != r.name {
				continue
			}
			cores := selectLabeledInputs(dir, r.accept)
			if len(cores) == 0 {
				continue
			}
			return &thermalSource{dir: dir, coreInputs: cores}, nil
		}
	}
	return nil, nil
}

func scanThermalZones(rootSys string) (*thermalSource, error) {
	thermalRoot := filepath.Join(rootSys, "class", "thermal")
	entries, _ := os.ReadDir(thermalRoot)
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })

	for _, preferType := range []string{"cpu_thermal", "x86_pkg_temp"} {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "thermal_zone") {
				continue
			}
			dir := filepath.Join(thermalRoot, e.Name())
			ztype, _ := readTrimmed(filepath.Join(dir, "type"))
			if ztype != preferType {
				continue
			}
			return &thermalSource{dir: dir, singleInput: "temp"}, nil
		}
	}
	return nil, nil
}

// selectLabeledInputs always rejects the Tctl label: on some Ryzen chips
// it carries a +20°C offset that would silently inflate emitted temps.
func selectLabeledInputs(dir string, accept func(label string) bool) []string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type pair struct {
		idx   int
		input string
	}
	var picked []pair
	for _, f := range files {
		name := f.Name()
		if !strings.HasPrefix(name, "temp") || !strings.HasSuffix(name, "_label") {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(name, "temp"), "_label")
		idx, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		label, err := readTrimmed(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if label == "Tctl" || !accept(label) {
			continue
		}
		input := fmt.Sprintf("temp%d_input", idx)
		if _, err := os.Stat(filepath.Join(dir, input)); err != nil {
			continue
		}
		picked = append(picked, pair{idx, input})
	}
	slices.SortFunc(picked, func(a, b pair) int { return a.idx - b.idx })
	out := make([]string, 0, len(picked))
	for _, p := range picked {
		out = append(out, p.input)
	}
	return out
}

func sampleThermal(src *thermalSource) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	var temps []float64
	switch {
	case len(src.coreInputs) > 0:
		for _, input := range src.coreInputs {
			if v, ok := readMilliC(filepath.Join(src.dir, input)); ok {
				temps = append(temps, v)
			}
		}
	case src.singleInput != "":
		if v, ok := readMilliC(filepath.Join(src.dir, src.singleInput)); ok {
			temps = append(temps, v)
		}
	}
	if len(temps) == 0 {
		return map[string]any{}
	}
	return map[string]any{
		"cpu_temp_max_c":      round2(slices.Max(temps)),
		"cpu_temp_avg_c":      round2(meanFloat(temps)),
		"cpu_temp_per_core_c": floatsRound2ToAny(temps),
	}
}

func readTrimmed(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readMilliC(path string) (float64, bool) {
	s, err := readTrimmed(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return float64(n) / 1000.0, true
}

func meanFloat(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
