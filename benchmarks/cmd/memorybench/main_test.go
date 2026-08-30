package main

import "testing"

func TestRunScenarioReportsEveryMemoryPhase(t *testing.T) {
	pool, err := newMemoryPool("project-bounded")
	if err != nil {
		t.Fatalf("newMemoryPool(): %v", err)
	}
	measurement, err := runScenario(options{
		contender:       "project-bounded",
		smallSize:       64,
		smallIterations: 10,
		peakSize:        1 << 20,
		peakCount:       2,
		runIndex:        1,
	}, pool)
	if err != nil {
		t.Fatalf("runScenario(): %v", err)
	}
	wantPhases := []string{"steady-small", "peak-held", "peak-released", "recovered-small", "gc-1", "gc-2"}
	if len(measurement.Samples) != len(wantPhases) {
		t.Fatalf("sample count = %d; want %d", len(measurement.Samples), len(wantPhases))
	}
	for i, want := range wantPhases {
		if measurement.Samples[i].Phase != want {
			t.Fatalf("sample %d phase = %q; want %q", i, measurement.Samples[i].Phase, want)
		}
		if measurement.Samples[i].RetainedCapacity > 32<<20 {
			t.Fatalf("sample %s RetainedCapacity = %d; exceeds Bounded budget", want, measurement.Samples[i].RetainedCapacity)
		}
	}
}

func TestSummarizeAggregatesRepeatedSamples(t *testing.T) {
	results := []result{
		{Contender: "test", Samples: []sample{{Phase: "steady", HeapAlloc: 10, HeapInuse: 20, RetainedCapacity: 30}}},
		{Contender: "test", Samples: []sample{{Phase: "steady", HeapAlloc: 30, HeapInuse: 40, RetainedCapacity: 50}}},
	}
	summaries := summarize(results)
	if len(summaries) != 1 {
		t.Fatalf("summary count = %d; want 1", len(summaries))
	}
	got := summaries[0]
	if got.Runs != 2 || got.HeapAllocMin != 10 || got.HeapAllocMean != 20 || got.HeapAllocMax != 30 {
		t.Fatalf("HeapAlloc summary = %+v; want runs 2/min 10/mean 20/max 30", got)
	}
	if got.HeapInuseMin != 20 || got.HeapInuseMean != 30 || got.HeapInuseMax != 40 || got.RetainedCapacityMean != 40 {
		t.Fatalf("remaining summary = %+v; want HeapInuse 20/30/40 and retained mean 40", got)
	}
}
