package test

import (
	"testing"

	"github.com/nats-io/nkeys"
	. "github.com/synadia-io/nex/internal/control-api"
)

func TestEncryption(t *testing.T) {

	myKey, _ := nkeys.CreateCurveKeys()

	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()

	issuerAccount, _ := nkeys.CreateAccount()

	request, _ := NewDeployRequest(
		WorkloadName("testworkload"),
		WorkloadDescription("testy mctesto"),
		Checksum("hashbrowns"),
		EnvironmentValue("NATS_URL", "nats://127.0.0.1:4222"),
		EnvironmentValue("TOP_SECRET_LUGGAGE", "12345"),
		SenderXKey(myKey),
		Issuer(issuerAccount),
		Location("nats://MUHBUCKET/muhfile"),
		TargetPublicXKey(recipientPk),
	)

	err := request.DecryptRequestEnvironment(recipientKey)
	if err != nil {
		t.Fatalf("Did not decrypt request environment: %s", err)
	}
	if request.WorkloadEnvironment["TOP_SECRET_LUGGAGE"] != "12345" {
		t.Fatalf("Expected a good luggage password, found %s", request.WorkloadEnvironment["TOP_SECRET_LUGGAGE"])
	}

	_, err = request.Validate()
	if err != nil {
		t.Fatalf("Expected no error, but got one: %s", err)
	}

}
