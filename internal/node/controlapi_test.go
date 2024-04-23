package nexnode

import (
	"slices"
	"testing"

	controlapi "github.com/synadia-io/nex/control-api"
)

func TestSummarizeMachinesForPing(t *testing.T) {
	workloads := []controlapi.MachineSummary{
		{
			Id:        "bob",
			Healthy:   true,
			Uptime:    "1m",
			Namespace: "default",
			Workload: controlapi.WorkloadSummary{
				Name:         "test",
				Description:  "test",
				WorkloadType: "elf",
				Hash:         "browns",
			},
		},
		{
			Id:        "alice",
			Healthy:   true,
			Uptime:    "1m",
			Namespace: "default",
			Workload: controlapi.WorkloadSummary{
				Name:         "test2",
				Description:  "test2",
				WorkloadType: "elf",
				Hash:         "browns",
			},
		},
		{
			Id:        "jim",
			Healthy:   true,
			Uptime:    "1m",
			Namespace: "notdefault",
			Workload: controlapi.WorkloadSummary{
				Name:         "jim1",
				Description:  "jim1",
				WorkloadType: "elf",
				Hash:         "browns",
			},
		},
	}

	// searching without namespace or workload id returns everything
	results := summarizeMachinesForPing(workloads, "", "")
	if len(results) != 3 {
		t.Fatalf("Results size mismatch, actual %d", len(results))
	}
	if results[0].Id != "bob" ||
		results[0].Name != "test" ||
		results[0].Namespace != "default" ||
		results[0].WorkloadType != "elf" {
		t.Fatalf("Result data doesn't match expectations: %+v", results[0])
	}

	// search for namespace with no workload Id returns all in namespace
	results = summarizeMachinesForPing(workloads, "default", "")
	if len(results) != 2 {
		t.Fatalf("Results size mismatch, actual %d", len(results))
	}

	// supply workload name and namespace
	results = summarizeMachinesForPing(workloads, "default", "test")  // by name
	results2 := summarizeMachinesForPing(workloads, "default", "bob") // by workload ID

	if !slices.Equal(results, results2) || len(results) != 1 {
		t.Fatalf("Searching by namespace and workload ID returned invalid results: %v and %v", results, results2)
	}

	// bad namespace but valid workload ID
	results = summarizeMachinesForPing(workloads, "blurgle", "test")
	if len(results) != 0 {
		t.Fatalf("Should've returned 0 results, got %d", len(results))
	}
}
