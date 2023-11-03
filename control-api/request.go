package controlapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type RunRequest struct {
	Description  string  `json:"description,omitempty"`
	WorkloadType string  `json:"type"`
	Location     url.URL `json:"location"`
	// Contains claims for the workload: name, hash
	WorkloadJwt string `json:"workload_jwt"`
	// A base64-encoded byte array that contains an encrypted json-serialized map[string]string.
	Environment     string `json:"environment"`
	SenderPublicKey string `json:"sender_public_key"`
	// If the payload indicates an object store bucket & key, JS domain can be supplied
	JsDomain string `json:"jsdomain,omitempty"`

	workloadEnvironment map[string]string `json:"-"`
	DecodedClaims       jwt.GenericClaims `json:"-"`
}

var (
	validWorkloadName = regexp.MustCompile(`^[a-z]+$`)
)

func NewRunRequest(opts ...RequestOption) (*RunRequest, error) {
	reqOpts := requestOptions{}
	for _, o := range opts {
		reqOpts = o(reqOpts)
	}
	workloadJwt, err := CreateWorkloadJwt(reqOpts.bytes, reqOpts.workloadName, reqOpts.claimsIssuer)
	if err != nil {
		return nil, err
	}
	encryptedEnv, err := EncryptRequestEnvironment(reqOpts.senderPublicXkey, reqOpts.targetPublicKey, reqOpts.env)
	if err != nil {
		return nil, err
	}
	senderPublic, _ := reqOpts.senderPublicXkey.PublicKey()

	req := &RunRequest{
		Description:     reqOpts.workloadDescription,
		WorkloadType:    "elf",
		Location:        url.URL{},
		WorkloadJwt:     workloadJwt,
		Environment:     encryptedEnv,
		SenderPublicKey: senderPublic,
		JsDomain:        reqOpts.jsDomain,
	}

	return req, nil
}

func (request *RunRequest) Validate(myKey nkeys.KeyPair) error {
	fmt.Printf("%v", request)
	claims, err := jwt.DecodeGeneric(request.WorkloadJwt)
	if err != nil {
		return err
	}
	request.DecodedClaims = *claims
	if !validWorkloadName.MatchString(claims.Subject) {
		return fmt.Errorf("workload name claim ('%s') does not match requirements of all lowercase letters", claims.Subject)
	}

	err = request.DecryptRequestEnvironment(myKey)
	if err != nil {
		return err
	}

	var vr jwt.ValidationResults
	claims.Validate(&vr)
	if len(vr.Issues) > 0 || len(vr.Errors()) > 0 {
		return errors.New("standard claims within JWT are not valid")
	}

	return nil

}

func CreateWorkloadJwt(filebytes []byte, name string, issuer nkeys.KeyPair) (string, error) {
	hasher := sha256.New()
	_, err := hasher.Write(filebytes)
	if err != nil {
		return "", err
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
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
	data, err := base64.StdEncoding.DecodeString(request.Environment)
	if err != nil {
		return err
	}
	unencrypted, err := recipientXKey.Open(data, request.SenderPublicKey)
	if err != nil {
		return err
	}
	var cleanEnv map[string]string
	err = json.Unmarshal(unencrypted, &cleanEnv)
	if err != nil {
		return err
	}
	// "I can't believe I can do this" - Every Rust developer ever.
	request.workloadEnvironment = cleanEnv
	return nil
}

type requestOptions struct {
	workloadName        string
	workloadDescription string
	bytes               []byte
	env                 map[string]string
	senderPublicXkey    nkeys.KeyPair
	claimsIssuer        nkeys.KeyPair
	targetPublicKey     string
	jsDomain            string
}

type RequestOption func(o requestOptions) requestOptions

func WorkloadName(name string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadName = name
		return o
	}
}

func WorkloadDescription(name string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.workloadDescription = name
		return o
	}
}

func FileBytes(bytes []byte) RequestOption {
	return func(o requestOptions) requestOptions {
		o.bytes = bytes
		return o
	}
}

func Environment(env map[string]string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.env = env
		return o
	}
}

func SenderXKey(xkey nkeys.KeyPair) RequestOption {
	return func(o requestOptions) requestOptions {
		o.senderPublicXkey = xkey
		return o
	}
}

func TargetPublicKey(key string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.targetPublicKey = key
		return o
	}
}

func Issuer(issuerAccountKey nkeys.KeyPair) RequestOption {
	return func(o requestOptions) requestOptions {
		o.claimsIssuer = issuerAccountKey
		return o
	}
}

func JsDomain(domain string) RequestOption {
	return func(o requestOptions) requestOptions {
		o.jsDomain = domain
		return o
	}
}

func EnvironmentValue(key string, value string) RequestOption {
	return func(o requestOptions) requestOptions {
		if o.env == nil {
			o.env = make(map[string]string)
		}
		o.env[key] = value
		return o
	}
}
