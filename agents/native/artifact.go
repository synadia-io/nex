package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	SchemeFile = "file"
	SchemeNATS = "nats"
)

type ArtifactReference struct {
	// The name of the artifact, as indicated by a deployment request. This is NOT a filename
	Name string
	// An optional tag for the artifact. Empty string if one isn't used
	Tag string
	// The location URL as contained on the original deployment request
	OriginalURI string
	// Once downloaded, this is the local file system path to the artifact. Used for loading
	// or execution -> for file:// it will match the original URI, for nats:// it will be a temp file
	LocalCachePath string
	// The calculated digest of the artifact file as it sits on disk
	Digest string
	// The size of the artifact, in bytes
	Size int
}

// Obtains an artifact from the specified location. If the indicated location allows for
// differentiation using tags (e.g. OCI, Object Store), then the supplied tag will be used,
// otherwise it will be ignored
func getArtifact(inUri string, nc *nats.Conn) (*ArtifactReference, error) {
	uri, err := parseUri(inUri)
	if err != nil {
		return nil, err
	}

	switch uri.schema {
	case SchemeFile:
		return cacheFile(uri)
	case SchemeNATS:
		return cacheObjectStoreArtifact(uri, nc)
	default:
		return nil, errors.New("unsupported artifact scheme")
	}
}

func cacheFile(uri *uri) (*ArtifactReference, error) {
	info, err := os.Stat(uri.path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, errors.New("artifact path is a directory")
	}

	fOrig, err := os.Open(uri.path)
	if err != nil {
		return nil, err
	}
	defer fOrig.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, fOrig); err != nil {
		return nil, err
	}

	return &ArtifactReference{
		Name:           filepath.Base(uri.path),
		Tag:            uri.tag,
		OriginalURI:    uri.schema + "://" + uri.path,
		LocalCachePath: uri.path,
		Digest:         hex.EncodeToString(hasher.Sum(nil)),
		Size:           int(info.Size()),
	}, nil
}

func cacheObjectStoreArtifact(uri *uri, nc *nats.Conn) (*ArtifactReference, error) {
	var err error
	binary := uri.path + "_" + uri.tag

	if nc == nil {
		return nil, errors.New("nats connection not provided")
	}

	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	obs, err := jsCtx.ObjectStore(context.TODO(), uri.location)
	if err != nil {
		return nil, err
	}

	bin_b, err := obs.GetBytes(context.TODO(), binary)
	if err != nil {
		return nil, err
	}

	fCache, err := os.CreateTemp(os.TempDir(), getFileName())
	if err != nil {
		return nil, err
	}
	defer fCache.Close()

	_, err = fCache.Write(bin_b)
	if err != nil {
		return nil, err
	}

	fCacheInfo, err := fCache.Stat()
	if err != nil {
		return nil, err
	}

	if _, err := fCache.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, fCache); err != nil {
		return nil, err
	}

	err = os.Chmod(fCache.Name(), 0755)
	if err != nil {
		return nil, err
	}

	ol := func() string {
		if uri.tag == "" {
			return uri.schema + "://" + uri.location + "/" + uri.path
		}
		return uri.schema + "://" + uri.location + "/" + uri.path + ":" + uri.tag
	}()

	return &ArtifactReference{
		Name:           filepath.Base(uri.path),
		Tag:            uri.tag,
		OriginalURI:    ol,
		LocalCachePath: fCache.Name(),
		Digest:         hex.EncodeToString(hasher.Sum(nil)),
		Size:           int(fCacheInfo.Size()),
	}, nil
}

type uri struct {
	schema   string
	location string
	path     string
	tag      string
}

func (u uri) String() string {
	return fmt.Sprintf("%s://%s/%s:%s", u.schema, u.location, u.path, u.tag)
}

func parseUri(inUri string) (*uri, error) {
	u := new(uri)

	// example oci://domain:port/repo:tag
	sUri := strings.Split(inUri, "://")
	if len(sUri) != 2 {
		return nil, fmt.Errorf("invalid uri: %s", inUri)
	}

	// oci
	u.schema = sUri[0]

	if u.schema == SchemeFile {
		// /tmp/foo
		u.path = sUri[1]
		return u, nil
	}

	// split should be: [0]=domain:port [1]=repo:tag
	sUriPath := strings.SplitN(sUri[1], "/", 2)
	if len(sUriPath) != 2 {
		return nil, fmt.Errorf("invalid uri: %s", inUri)
	}
	// domain:port
	location := sUriPath[0]
	u.location = location

	sPath := strings.SplitN(sUriPath[1], ":", 2)
	if len(sPath) != 2 {
		u.path = sUriPath[1]
		u.tag = "latest"
		return u, nil
	}

	u.path = sPath[0]
	u.tag = sPath[1]
	return u, nil
}
