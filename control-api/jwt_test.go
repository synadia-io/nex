package controlapi

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/nats-io/nkeys"
)

func TestJwtVerification(t *testing.T) {
	kp, _ := nkeys.CreateAccount()

	filebytes := []byte{1, 2, 3, 4, 5, 6}
	hasher := sha256.New()
	_, err := hasher.Write(filebytes)
	if err != nil {
		t.Fatalf("hash fail: %s", err)
	}
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	theJwt, _ := CreateWorkloadJwt(filebytes, "testworkload", kp)
	validation, _ := VerifyWorkloadJwt(theJwt, "testworkload", expectedHash)
	if len(validation.Issues) > 0 {
		t.Fatalf("Expected no issues with first validation test, got %d", len(validation.Issues))
	}

	// Should reject a bad hash
	badValidation, _ := VerifyWorkloadJwt(theJwt, "testworkload", "hashbrowns")
	if len(badValidation.Issues) == 0 {
		t.Fatalf("Expected a rejection issue but got 0")
	}
	if badValidation.Issues[0].Description != "Checksum in claims does not match expected hash" {
		t.Fatalf("Wrong description in checksum validation failure, got '%s' instead.", badValidation.Issues[0].Description)
	}

	// Should reject a mismatched name
	badValidation2, _ := VerifyWorkloadJwt(theJwt, "testNOTGONNAWORKload", expectedHash)
	if len(badValidation2.Issues) == 0 {
		t.Fatalf("Expected a rejection issue but got 0")
	}
	if badValidation2.Issues[0].Description != "Workload name in claims does not match expected name" {
		t.Fatalf("Wrong description in checksum validation failure, got '%s' instead.", badValidation.Issues[0].Description)
	}

	// Fail both
	badValidation3, _ := VerifyWorkloadJwt(theJwt, "testNOTGONNAWORKload", "hashbrowns")
	if len(badValidation3.Issues) != 2 {
		t.Fatalf("Expected 2 failure issues, got %d instead", len(badValidation3.Issues))
	}

}
