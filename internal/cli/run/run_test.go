package run

import (
	"net/url"
	"testing"
)

func TestRunValidateHappy(t *testing.T) {
	url, err := url.Parse("nats://bucket/binary")
	if err != nil {
		t.Fatal(err)
	}

	ro := RunOptions{
		WorkloadUrl: url,
	}

	if err := ro.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidatePasses(t *testing.T) {
	url, err := url.Parse("nats://bucket/binary")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Description string
		RO          RunOptions
	}{
		{"Valid Target node", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{TargetNode: "NC2QZZGD2KIV3N2S4RD3HYXJ2JBGB7GJWDYLXEK5HKZIYNKIS23CX4DA"}}},
		{"Valid Name", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{Name: "derp"}}},
		{"Valid Trigger Subject", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{TriggerSubjects: []string{"derp", "foo", "bar"}}}},
	}

	for i, tt := range tests {
		err := tt.RO.Validate()
		if err != nil {
			t.Fatalf("Test %d failed: %s", i, tt.Description)
		}
	}

}

func TestRunValidateFailures(t *testing.T) {
	url, err := url.Parse("nats://bucket/binary")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Description string
		RO          RunOptions
		ErrorString string
	}{
		{"Invalid Target node", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{TargetNode: "derp"}}, "provided bad TargetNode.  Must be valid public server nkey. eg: NABCDEFG..."},
		{"Invalid Name", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{Name: "derp!"}}, "workload name must be alphanumeric"},
		{"Invalid Trigger Subject", RunOptions{WorkloadUrl: url, SharedRunOptions: SharedRunOptions{TriggerSubjects: []string{"derp", "foo", "bar/"}}}, "trigger subjects contains invalid nats subject"},
	}

	for i, tt := range tests {
		err := tt.RO.Validate()
		if err == nil {
			t.Fatalf("Expected error, got nil: %s", tt.Description)
		} else if err.Error() != tt.ErrorString {
			t.Log("Expected:", tt.ErrorString, "Got: ", err.Error())
			t.Fatalf("Test %d failed: %s", i, tt.Description)
		}
	}
}
