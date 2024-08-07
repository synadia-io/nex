package test

import (
	"testing"

	"github.com/nats-io/nkeys"
	. "github.com/synadia-io/nex/control-api"
)

func TestEncryption(t *testing.T) {

	myKey, _ := nkeys.CreateCurveKeys()

	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()
	issuerAccount, _ := nkeys.CreateAccount()

	request, _ := NewDeployRequest(
		HostServicesConfig(
			NatsJwtConnectionInfo{
				NatsUrl:      "nats://1.2.3.4",
				NatsUserJwt:  "eyJ0eXAiOi...",
				NatsUserSeed: "SUAP6AZZJC35W42XYP5RRABRDWFMD6ZF3WTHQ7NTXEYPHTBRFU7XOQ2D2E",
			}),
		WorkloadType(NexWorkloadNative),
		WorkloadName("testworkload"),
		WorkloadDescription("testy mctesto"),
		Hash("hashbrowns"),
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

	if request.WorkloadEnvironment["NEX_HOSTSERVICES_NATS_SERVER"] != "nats://1.2.3.4" {
		t.Fatalf("Hostservices config was not properly configured. Expected %s, Got: '%s'", "nats://1.2.3.4", request.WorkloadEnvironment["NEX_HOSTSERVICES_NATS_SERVER"])
	}
}
