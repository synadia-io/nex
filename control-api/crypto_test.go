package controlapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/nats-io/nkeys"
)

func TestEncryption(t *testing.T) {

	myKey, _ := nkeys.CreateCurveKeys()
	myPk, _ := myKey.PublicKey()

	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()

	rawEnvironment := map[string]string{
		"NATS_URL":           "nats://127.0.0.1:4222",
		"TOP_SECRET_LUGGAGE": "12345",
	}

	jsonEnv, _ := json.Marshal(rawEnvironment)

	encEnv, _ := myKey.Seal(jsonEnv, recipientPk)
	hexenv := base64.StdEncoding.EncodeToString(encEnv)

	request := RunRequest{
		WorkloadName:    "test",
		Description:     "testy mctesto",
		WorkloadType:    "elf",
		Location:        url.URL{},
		WorkloadHash:    "abc",
		Environment:     hexenv,
		SenderPublicKey: myPk,
		JsDomain:        "",
	}
	fmt.Printf("Request: %v\n", request)

	request.DecryptRequestEnvironment(recipientKey)
	if request.workloadEnvironment["TOP_SECRET_LUGGAGE"] != "12345" {
		t.Fatalf("Expected a good luggage password, found %s", request.workloadEnvironment["TOP_SECRET_LUGGAGE"])
	}
}
