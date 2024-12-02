package agentapi

import "fmt"

// Subjects as indicated in ADR-1
// https://github.com/synadia-io/nex/blob/main/adr/adr-1.md
// NOTE: there are some subjects that are different here than the original ADR.
// As always, the code is the source of truth

func AgentRegisterSubject() string {
	return "host.register"
}

func StartWorkloadSubscribeSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.start", workloadType)
}

func StartWorkloadSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.start", workloadType)
}

// These _could_ be shared in a single function, but if we decide to change
// the wildcards in the future, we're ok and don't need to refactor
func StopWorkloadSubscribeSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.stop", workloadType)
}

func StopWorkloadSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.stop", workloadType)
}

func ListWorkloadsSubscribeSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.list", workloadType)
}

func WorkloadTriggerSubscribeSubject(workloadType string) string {
	return fmt.Sprintf("agent.%s.workloads.*.trigger", workloadType)
}

func WorkloadTriggerSubject(workloadType string, workloadId string) string {
	return fmt.Sprintf("agent.%s.workloads.%s.trigger", workloadType, workloadId)
}

func PerformRPCSubject(
	workloadType string,
	workloadId string,
	namespace string,
	service string,
	method string) string {
	return fmt.Sprintf("host.%s.rpc.%s.%s.%s.%s", workloadType, namespace, workloadId, service, method)
}

func PerformRPCSubscribeSubject() string {
	return "host.*.rpc.>"
}
