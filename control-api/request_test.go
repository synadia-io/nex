package controlapi

import (
	"testing"

	"github.com/nats-io/nkeys"
)

func TestEncryption(t *testing.T) {

	myKey, _ := nkeys.CreateCurveKeys()
	//myPk, _ := myKey.PublicKey()

	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()

	issuerAccount, _ := nkeys.CreateAccount()

	request, _ := NewRunRequest(
		WorkloadName("testworkload"),
		WorkloadDescription("testy mctesto"),
		FileBytes([]byte{1, 2, 3, 4, 5}),
		EnvironmentValue("NATS_URL", "nats://127.0.0.1:4222"),
		EnvironmentValue("TOP_SECRET_LUGGAGE", "12345"),
		SenderXKey(myKey),
		Issuer(issuerAccount),
		TargetPublicKey(recipientPk),
	)

	request.DecryptRequestEnvironment(recipientKey)
	if request.workloadEnvironment["TOP_SECRET_LUGGAGE"] != "12345" {
		t.Fatalf("Expected a good luggage password, found %s", request.workloadEnvironment["TOP_SECRET_LUGGAGE"])
	}

	err := request.Validate(recipientKey)
	if err != nil {
		t.Fatalf("Expected no error, but got one: %s", err)
	}

}
