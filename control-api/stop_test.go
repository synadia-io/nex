package controlapi

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

func TestStopValidation(t *testing.T) {
	myKey, _ := nkeys.CreateCurveKeys()

	recipientKey, _ := nkeys.CreateCurveKeys()
	recipientPk, _ := recipientKey.PublicKey()

	issuerAccount, _ := nkeys.CreateAccount()

	request, _ := NewRunRequest(
		WorkloadName("testworkload"),
		WorkloadType("elf"),
		WorkloadDescription("testy mctesto"),
		Checksum("hashbrowns"),
		EnvironmentValue("NATS_URL", "nats://127.0.0.1:4222"),
		EnvironmentValue("TOP_SECRET_LUGGAGE", "12345"),
		SenderXKey(myKey),
		Issuer(issuerAccount),
		Location("nats://MUHBUCKET/muhfile"),
		TargetPublicXKey(recipientPk),
	)

	_, err := request.Validate(recipientKey) // need this to produce the `DecodedClaims` field
	if err != nil {
		t.Fatalf("Failed to validate request that should've passed: %s", err)
	}

	originalClaims := request.DecodedClaims
	fmt.Printf("%+v", originalClaims)
	time.Sleep(1 * time.Second) // ensure that second token has a newer timestamp

	stopRequest, _ := NewStopRequest("1234", "testworkload", "Nx", issuerAccount)

	err = stopRequest.Validate(&originalClaims)
	if err != nil {
		t.Fatalf("Expected no errors during positive path validation, got %s", err)
	}

	badIssuerAccount, _ := nkeys.CreateAccount()
	badStopRequest, _ := NewStopRequest("1234", "testworkload", "Nx", badIssuerAccount)
	err = badStopRequest.Validate(&originalClaims)
	if err == nil {
		t.Fatalf("Expected to get an error validating a bad issuer, but got none")
	}

	badStopRequest2, _ := NewStopRequest("1234", "nonexistentworkload", "Nx", issuerAccount)
	err = badStopRequest2.Validate(&originalClaims)
	if err == nil {
		t.Fatalf("Expected to get an error validating bad subject, but got none")
	}
}
