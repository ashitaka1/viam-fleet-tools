package workload

import "math"

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// floatsRound2ToAny rounds each element to 2 dp and packs them into []any —
// google.protobuf.Struct's ListValue encoding rejects typed []float64.
func floatsRound2ToAny(v []float64) []any {
	out := make([]any, len(v))
	for i, x := range v {
		out[i] = round2(x)
	}
	return out
}
