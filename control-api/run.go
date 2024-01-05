package controlapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type RunRequest struct {
	Description  *string  `json:"description,omitempty"`
	WorkloadType *string  `json:"type"`
	Location     *url.URL `json:"location"`

	// Contains claims for the workload: name, hash
	WorkloadJwt *string `json:"workload_jwt"`

	// A base64-encoded byte array that contains an encrypted json-serialized map[string]string.
	Environment *string `json:"environment"`

	// If the payload indicates an object store bucket & key, JS domain can be supplied
	JsDomain *string `json:"jsdomain,omitempty"`

	SenderPublicKey *string  `json:"sender_public_key"`
	TargetNode      *string  `json:"target_node"`
	TriggerSubjects []string `json:"trigger_subjects,omitempty"`

	WorkloadEnvironment map[string]string `json:"-"`
	DecodedClaims       jwt.GenericClaims `json:"-"`
}

var (
	validWorkloadName = regexp.MustCompile(`^[a-z]+$`)
)

// Creates a new run request based on the supplied options. Note that there is a fluent API function
// for each available option
func NewRunRequest(opts ...RequestOption) (*RunRequest, error) {
	reqOpts := requestOptions{}
	for _, o := range opts {
		reqOpts = o(reqOpts)
	}

	// TODO: ensure that all the required fields are here

	workloadJwt, err := CreateWorkloadJwt(reqOpts.hash, reqOpts.workloadName, reqOpts.claimsIssuer)
	if err != nil {
		return nil, err
	}

	encryptedEnv, err := EncryptRequestEnvironment(reqOpts.senderXkey, reqOpts.targetPublicXKey, reqOpts.env)
	if err != nil {
		return nil, err
	}

	senderPublic, _ := reqOpts.senderXkey.PublicKey()

	req := &RunRequest{
		Description:     &reqOpts.workloadDescription,
		WorkloadType:    &reqOpts.workloadType,
		Location:        &reqOpts.location,
		WorkloadJwt:     &workloadJwt,
		Environment:     &encryptedEnv,
		SenderPublicKey: &senderPublic,
		TargetNode:      &reqOpts.targetNode,
		TriggerSubjects: reqOpts.triggerSubjects,
		JsDomain:        &reqOpts.jsDomain,
	}

	return req, nil
}

// This will validate a request's workload JWT, decrypt the request environment. It will not
// perform a comparison of the hash found in the claims with a recipient's expected hash
func (request *RunRequest) Validate(myKey nkeys.KeyPair) (*jwt.GenericClaims, error) {
	claims, err := jwt.DecodeGeneric(*request.WorkloadJwt)
	if err != nil {
		return nil, fmt.Errorf("could not decode workload JWT: %s", err)
	}

	request.DecodedClaims = *claims
	if !validWorkloadName.MatchString(claims.Subject) {
		return nil, fmt.Errorf("workload name claim ('%s') does not match requirements of all lowercase letters", claims.Subject)
	}

	err = request.DecryptRequestEnvironment(myKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt request environment: %s", err)
	}

	var vr jwt.ValidationResults
	claims.Validate(&vr)
	if len(vr.Issues) > 0 || len(vr.Errors()) > 0 {
		return nil, errors.New("standard claims within JWT are not valid")
	}

	return claims, nil
}

func CreateWorkloadJwt(hash string, name string, issuer nkeys.KeyPair) (string, error) {
	genericClaims := jwt.NewGenericClaims(name)
	genericClaims.Data["hash"] = hash

	return genericClaims.Encode(issuer)
}

func EncryptRequestEnvironment(senderXKey nkeys.KeyPair, recipientPublicKey string, env map[string]string) (string, error) {
	jsonEnv, _ := json.Marshal(env)

	encEnv, _ := senderXKey.Seal(jsonEnv, recipientPublicKey)
	hexenv := base64.StdEncoding.EncodeToString(encEnv)
	return hexenv, nil
}

func (request *RunRequest) DecryptRequestEnvironment(recipientXKey nkeys.KeyPair) error {
	data, err := base64.StdEncoding.DecodeString(*request.Environment)
	if err != nil {
		return err
	}

	unencrypted, err := recipientXKey.Open(data, *request.SenderPublicKey)
	if err != nil {
		return err
	}

	var cleanEnv map[string]string
	err = json.Unmarshal(unencrypted, &cleanEnv)
	if err != nil {
		return err
	}

	// "I can't believe I can do this" - Every Rust developer ever.
	request.WorkloadEnvironment = cleanEnv
	return nil
}

type requestOptions struct {
	workloadName        string
	workloadType        string
	workloadDescription string
	location            url.URL
	env                 map[string]string
	senderXkey          nkeys.KeyPair
	claimsIssuer        nkeys.KeyPair
	targetPublicXKey    string
	jsDomain            string
	hash                string
	targetNode          string
	triggerSubjects     []string
}

type RequestOption func(o requestOptions) requestOptions

// Name of the workload. Conforms to the same name rules as the services API
func WorkloadName(name string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadName = name
		return o
	}
}

// Type of the workload, e.g., one of "elf", "v8", "oci", "wasm" for this request
func WorkloadType(workloadType string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadType = workloadType
		return o
	}
}

// Sets the target execution engine node (a public key of type "server") for this request
func TargetNode(publicKey string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.targetNode = publicKey
		return o
	}
}

// Sets the trigger subjects to register for this request
func TriggerSubjects(triggerSubjects []string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.triggerSubjects = triggerSubjects
		return o
	}
}

// Location of the workload. For files in NATS object stores, use nats://BUCKET/key
func Location(fileUrl string) RequestOption {
	return func(o requestOptions) requestOptions {
		nurl, err := url.Parse(fileUrl)
		if err != nil {
			o.location = url.URL{}
		} else {
			o.location = *nurl
		}
		return o
	}
}

// Description of the workload to run
func WorkloadDescription(name string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadDescription = name
		return o
	}
}

// Set the map of environment variables to be used by the workload
func Environment(env map[string]string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.env = env
		return o
	}
}

// This is the sender's xkey. The public key will be placed on the request while the private key will be used
// to encrypt the environment variables
func SenderXKey(xkey nkeys.KeyPair) RequestOption {
	return func(o requestOptions) requestOptions {
		o.senderXkey = xkey
		return o
	}
}

// Sets the public key of the recipient (A public curve key). This must be set properly or the recipient of the request
// will be unable to decrypt the environment variables
func TargetPublicXKey(key string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.targetPublicXKey = key
		return o
	}
}

// An account key used to sign the JWT that accompanies the request and asserts the hash of the file
func Issuer(issuerAccountKey nkeys.KeyPair) RequestOption {
	return func(o requestOptions) requestOptions {
		o.claimsIssuer = issuerAccountKey
		return o
	}
}

// Optionally set a JetStream domain that will be used to locate an object store when necessary
func JsDomain(domain string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.jsDomain = domain
		return o
	}
}

// Sets a single environment value
func EnvironmentValue(key string, value string) RequestOption {
	return func(o requestOptions) requestOptions {
		if o.env == nil {
			o.env = make(map[string]string)
		}
		o.env[key] = value
		return o
	}
}

// Sets the hash of the workload payload for verification purposes
func Checksum(hash string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.hash = hash
		return o
	}
}
