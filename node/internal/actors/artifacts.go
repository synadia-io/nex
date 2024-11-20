package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	SchemeFile = "file"
	SchemeNATS = "nats"
	SchemeOCI  = "oci"
)

type ArtifactReference struct {
	// The name of the artifact, as indicated by a deployment request. This is NOT a filename
	Name string
	// An optional tag for the artifact. Empty string if one isn't used
	Tag string
	// The location URL as contained on the original deployment request
	OriginalLocation string
	// Once downloaded, this is the local file system path to the artifact. Used for loading
	// or execution
	LocalCachePath string
	// The calculated digest of the artifact file as it sits on disk
	Digest string
	// The size of the artifact, in bytes
	Size int
}

// Obtains an artifact from the specified location. If the indicated location allows for
// differentiation using tags (e.g. OCI, Object Store), then the supplied tag will be used,
// otherwise it will be ignored
func GetArtifact(name string, inUri string, nc *nats.Conn) (*ArtifactReference, error) {
	uri, err := parseUri(inUri)
	if err != nil {
		return nil, err
	}

	switch uri.schema {
	case SchemeFile:
		return cacheFile(name, uri)
	case SchemeNATS:
		return cacheObjectStoreArtifact(name, uri, nc)
	case SchemeOCI:
		return cacheOciArtifact(name, uri)
	default:
		return nil, errors.New("unsupported artifact scheme")
	}
}

func cacheFile(name string, uri *uri) (*ArtifactReference, error) {
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

	fCache, err := os.CreateTemp(os.TempDir(), getFileName())
	if err != nil {
		return nil, err
	}
	defer fCache.Close()

	if _, err = io.Copy(fCache, fOrig); err != nil {
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

	return &ArtifactReference{
		Name:             name,
		Tag:              uri.tag,
		OriginalLocation: uri.schema + "://" + uri.path,
		LocalCachePath:   fCache.Name(),
		Digest:           hex.EncodeToString(hasher.Sum(nil)),
		Size:             int(fCacheInfo.Size()),
	}, nil
}

func cacheObjectStoreArtifact(name string, uri *uri, nc *nats.Conn) (*ArtifactReference, error) {

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
		Name:             name,
		Tag:              uri.tag,
		OriginalLocation: ol,
		LocalCachePath:   fCache.Name(),
		Digest:           hex.EncodeToString(hasher.Sum(nil)),
		Size:             int(fCacheInfo.Size()),
	}, nil
}

func cacheOciArtifact(name string, uri *uri) (*ArtifactReference, error) {
	// TODO/NOTE: for now let's assume that if the target registry requires auth, it will be supplied in the location.User field

	repo, err := remote.NewRepository(uri.location + "/" + uri.path)
	if err != nil {
		return nil, err
	}
	repo.PlainHTTP = true
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}

	store := memory.New()
	descriptor, err := oras.Copy(context.TODO(), repo, uri.tag, store, "", oras.DefaultCopyOptions)
	if err != nil {
		return nil, err
	}

	if found, err := store.Exists(context.TODO(), descriptor); err != nil || !found {
		return nil, errors.New("artifact not found")
	}

	rc, err := store.Fetch(context.TODO(), descriptor)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	fetchData, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	var manifest v1.Manifest
	err = json.Unmarshal(fetchData, &manifest)
	if err != nil {
		return nil, err
	}

	for _, layer := range manifest.Layers {
		if !strings.HasPrefix(layer.ArtifactType, "application/nex.") {
			continue
		}

		if layer.Annotations["os"] != runtime.GOOS && layer.Annotations["arch"] != runtime.GOARCH {
			continue
		}

		l, err := repo.Fetch(context.TODO(), layer)
		if err != nil {
			return nil, err
		}
		defer l.Close()

		fCache, err := os.CreateTemp(os.TempDir(), getFileName())
		if err != nil {
			return nil, err
		}

		if _, err = io.Copy(fCache, l); err != nil {
			return nil, err
		}
		defer fCache.Close()

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
			Name:             name,
			Tag:              uri.tag,
			OriginalLocation: ol,
			LocalCachePath:   fCache.Name(),
			Digest:           hex.EncodeToString(hasher.Sum(nil)),
			Size:             int(fCacheInfo.Size()),
		}, nil
	}
	return nil, errors.New("nex artifact not found in provided oci manifest")
}
