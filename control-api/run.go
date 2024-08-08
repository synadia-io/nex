package controlapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

const (
	workloadRegex = `^[a-zA-Z0-9_-]+$`
)

type DeployRequest struct {
	Argv        []string `json:"argv,omitempty"`
	Description *string  `json:"description,omitempty"`
	Hash        *string  `json:"hash"`
	Location    *url.URL `json:"location"`
	Essential   *bool    `json:"essential,omitempty"`

	// A base64-encoded byte array that contains an encrypted json-serialized map[string]string.
	Environment *string `json:"environment"`

	ID *string `json:"id"`

	// If the payload indicates an object store bucket & key, JS domain can be supplied
	JsDomain *string `json:"jsdomain,omitempty"`

	RetryCount *uint      `json:"retry_count,omitempty"`
	RetriedAt  *time.Time `json:"retried_at,omitempty"`

	SenderPublicKey    *string                `json:"sender_public_key"`
	TargetNode         *string                `json:"target_node"`
	TriggerSubjects    []string               `json:"trigger_subjects,omitempty"`
	HostServicesConfig *NatsJwtConnectionInfo `json:"host_services_config,omitempty"`

	WorkloadName *string     `json:"workload_name"`
	WorkloadJWT  *string     `json:"workload_jwt"` // Contains claims for the workload: id, namespace
	WorkloadType NexWorkload `json:"type"`

	DecodedClaims       jwt.GenericClaims `json:"-"`
	WorkloadEnvironment map[string]string `json:"-"`
}

type NatsJwtConnectionInfo struct {
	NatsUrl      string `json:"nats_url"`
	NatsUserJwt  string `json:"nats_user_jwt"`
	NatsUserSeed string `json:"nats_user_seed"`
}

var (
	validWorkloadName = regexp.MustCompile(workloadRegex)
)

// Creates a new deploy request based on the supplied options. Note that there is a fluent API function
// for each available option
func NewDeployRequest(opts ...RequestOption) (*DeployRequest, error) {
	reqOpts := requestOptions{}
	for _, o := range opts {
		reqOpts = o(reqOpts)
	}

	// TODO: ensure that all the required fields are here

	reqUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	id := reqUUID.String()

	workloadJWT, err := CreateWorkloadJWT(id, reqOpts.workloadName, reqOpts.hash, reqOpts.claimsIssuer)
	if err != nil {
		return nil, err
	}

	encryptedEnv, err := EncryptRequestEnvironment(reqOpts.senderXkey, reqOpts.targetPublicXKey, reqOpts.env)
	if err != nil {
		return nil, err
	}

	senderPublic, _ := reqOpts.senderXkey.PublicKey()

	req := &DeployRequest{
		Argv:               reqOpts.argv,
		Description:        &reqOpts.workloadDescription,
		Environment:        &encryptedEnv,
		Essential:          &reqOpts.essential,
		HostServicesConfig: reqOpts.hostServicesConfig,
		ID:                 &id,
		Location:           &reqOpts.location,
		SenderPublicKey:    &senderPublic,
		TargetNode:         &reqOpts.targetNode,
		TriggerSubjects:    reqOpts.triggerSubjects,
		WorkloadJWT:        &workloadJWT,
		WorkloadName:       &reqOpts.workloadName,
		WorkloadType:       reqOpts.workloadType,
	}

	if reqOpts.jsDomain != "" {
		req.JsDomain = &reqOpts.jsDomain
	}

	return req, nil
}

// Returns true if the run request supports trigger subjects
func (request *DeployRequest) SupportsTriggerSubjects() bool {
	return request.WorkloadType == NexWorkloadV8 ||
		request.WorkloadType == NexWorkloadWasm
}

// This will validate a request's workload JWT and return the parsed claims
func (request *DeployRequest) Validate() (*jwt.GenericClaims, error) {
	claims, err := jwt.DecodeGeneric(*request.WorkloadJWT)
	if err != nil {
		return nil, fmt.Errorf("could not decode workload JWT: %s", err)
	}

	request.DecodedClaims = *claims

	if !validWorkloadName.MatchString(claims.Data["name"].(string)) {
		return nil, errors.New("invalid workload name")
	}

	if request.Hash != nil && *request.Hash != request.DecodedClaims.Data["hash"].(string) {
		return nil, errors.New("artifact hash claim does not match request")
	}

	var vr jwt.ValidationResults
	claims.Validate(&vr)
	if len(vr.Issues) > 0 || len(vr.Errors()) > 0 {
		return nil, errors.New("standard claims within JWT are not valid")
	}

	return claims, nil
}

func CreateWorkloadJWT(id, name, hash string, issuer nkeys.KeyPair) (string, error) {
	genericClaims := jwt.NewGenericClaims(id)
	genericClaims.Data["hash"] = hash
	genericClaims.Data["name"] = name

	return genericClaims.Encode(issuer)
}

func EncryptRequestEnvironment(senderXKey nkeys.KeyPair, recipientPublicKey string, env map[string]string) (string, error) {
	jsonEnv, _ := json.Marshal(env)

	encEnv, _ := senderXKey.Seal(jsonEnv, recipientPublicKey)
	hexenv := base64.StdEncoding.EncodeToString(encEnv)
	return hexenv, nil
}

func (request *DeployRequest) DecryptRequestEnvironment(recipientXKey nkeys.KeyPair) error {
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
	argv                []string
	claimsIssuer        nkeys.KeyPair
	env                 map[string]string
	essential           bool
	hash                string
	hostServicesConfig  *NatsJwtConnectionInfo
	jsDomain            string
	location            url.URL
	senderXkey          nkeys.KeyPair
	targetNode          string
	targetPublicXKey    string
	triggerSubjects     []string
	workloadDescription string
	workloadName        string
	workloadType        NexWorkload
}

type RequestOption func(o requestOptions) requestOptions

// Arguments to be passed to the workload, if applicable
func Argv(argv []string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.argv = argv
		return o
	}
}

// When set, overrides the node's host services configuration
func HostServicesConfig(config *NatsJwtConnectionInfo) RequestOption {
	return func(o requestOptions) requestOptions {
		o.hostServicesConfig = config
		return o
	}
}

// Name of the workload. Conforms to the same name rules as the services API
func WorkloadName(name string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadName = name
		return o
	}
}

// Type of the workload, e.g., one of "native", "v8", "oci", "wasm" for this request
func WorkloadType(workloadType NexWorkload) RequestOption {
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
func Location(workloadUrl string) RequestOption {
	return func(o requestOptions) requestOptions {
		nurl, err := url.Parse(workloadUrl)
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
		if o.env == nil {
			o.env = make(map[string]string)
		}
		return o
	}
}

// Set the essential flag to be used by the workload
func Essential(essential bool) RequestOption {
	return func(o requestOptions) requestOptions {
		o.essential = essential
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
func Hash(hash string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.hash = hash
		return o
	}
}
