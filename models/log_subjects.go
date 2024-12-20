package models

import "fmt"

const (
	// $NEX.logs.namespace."WorkloadID"."stdout|stderr"
	LOGS_SUBJECT NexSubject = "$NEX.logs.%s.%s.%s"

	// $NEX.events.namespace."WorkloadID"."EventType"
	EVENTS_SUBJECT NexSubject = "$NEX.events.%s.%s.%s"
)

type NexSubject string

func (n NexSubject) String() string {
	switch n {
	case LOGS_SUBJECT:
		return fmt.Sprintf(string(LOGS_SUBJECT), "*", "*", "*")
	case EVENTS_SUBJECT:
		return fmt.Sprintf(string(EVENTS_SUBJECT), "*", "*", "*")
	default:
		return ""
	}
}

func (n NexSubject) Filter(namespace string, inputs ...string) (string, error) {
	switch n {
	case LOGS_SUBJECT:
		if len(inputs) != 2 {
			return "", fmt.Errorf("invalid inputs: %v", inputs)
		}
		// TODO: validate inputs[0] is a valid id
		if inputs[1] != "*" && inputs[1] != "stdout" && inputs[1] != "stderr" {
			return "", fmt.Errorf("invalid log type: %v", inputs[1])
		}
		return fmt.Sprintf(string(LOGS_SUBJECT), namespace, inputs[0], inputs[1]), nil
	case EVENTS_SUBJECT:
		if len(inputs) != 2 {
			return "", fmt.Errorf("invalid inputs: %v", inputs)
		}
		// TODO: validate inputs[0] is a valid id
		// TODO: only valid types in input[1]
		return fmt.Sprintf(string(EVENTS_SUBJECT), namespace, inputs[0], inputs[1]), nil
	default:
		return "", fmt.Errorf("invalid subject: %v", n)
	}
}
