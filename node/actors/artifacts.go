package actors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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
	OriginalLocation *url.URL
	// Once downloaded, this is the local file system path to the artifact. Used for loading
	// or execution
	LocalCachePath string
	// The calculated digest of the artifact file as it sits on disk
	Digest string
	// The size of the artifact, in bytes
	Size int
}

func parsePathTag(location *url.URL) (string, string) {
	filePath, tag := "", ""
	sPath := strings.SplitN(location.Path, ":", 2) // splits on first : only
	if len(sPath) == 2 {
		filePath = sPath[0]
		tag = sPath[1]
	} else {
		filePath = location.Path
		tag = "latest"
	}

	return filePath, tag
}

// Obtains an artifact from the specified location. If the indicated location allows for
// differentiation using tags (e.g. OCI, Object Store), then the supplied tag will be used,
// otherwise it will be ignored
func getArtifact(name string, uri string, nc *nats.Conn) (*ArtifactReference, error) {
	location, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	switch location.Scheme {
	case SchemeFile:
		return cacheFile(name, location)
	case SchemeNATS:
		return cacheObjectStoreArtifact(name, location, nc)
	case SchemeOCI:
		return cacheOciArtifact(name, location)
	default:
		return nil, errors.New("unsupported artifact scheme")
	}
}

func cacheFile(name string, location *url.URL) (*ArtifactReference, error) {
	filePath, tag := parsePathTag(location)

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, errors.New("artifact path is a directory")
	}

	fOrig, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer fOrig.Close()

	fCache, err := os.CreateTemp(os.TempDir(), "workload-*")
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
		Tag:              tag,
		OriginalLocation: location,
		LocalCachePath:   fCache.Name(),
		Digest:           hex.EncodeToString(hasher.Sum(nil)),
		Size:             int(fCacheInfo.Size()),
	}, nil
}

func cacheObjectStoreArtifact(name string, location *url.URL, nc *nats.Conn) (*ArtifactReference, error) {
	artifact, tag := parsePathTag(location)
	if tag == "" {
		tag = "latest"
	}
	binary := strings.TrimPrefix(artifact, "/") + "_" + tag

	if nc == nil {
		return nil, errors.New("nats connection not provided")
	}

	jsCtx, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	obs, err := jsCtx.ObjectStore(context.TODO(), strings.TrimPrefix(location.Host, "/"))
	if err != nil {
		return nil, err
	}

	bin_b, err := obs.GetBytes(context.TODO(), binary)
	if err != nil {
		return nil, err
	}
	fCache, err := os.CreateTemp(os.TempDir(), "workload-*")
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

	return &ArtifactReference{
		Name:             name,
		Tag:              tag,
		OriginalLocation: location,
		LocalCachePath:   fCache.Name(),
		Digest:           hex.EncodeToString(hasher.Sum(nil)),
		Size:             int(fCacheInfo.Size()),
	}, nil
}

func cacheOciArtifact(name string, location *url.URL) (*ArtifactReference, error) {
	// TODO/NOTE: for now let's assume that if the target registry requires auth, it will be supplied in the location.User field

	artifact, tag := parsePathTag(location)
	repo, err := remote.NewRepository(fmt.Sprintf("%s%s", location.Host, artifact))
	if err != nil {
		return nil, err
	}
	repo.PlainHTTP = true
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.NewCache(),
	}

	store := memory.New()
	descriptor, err := oras.Copy(context.TODO(), repo, tag, store, "", oras.DefaultCopyOptions)
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
		if layer.MediaType != "application/nex.artifact.binary" {
			continue
		}

		l, err := repo.Fetch(context.TODO(), layer)
		if err != nil {
			return nil, err
		}
		defer l.Close()

		fCache, err := os.CreateTemp(os.TempDir(), "workload-*")
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

		return &ArtifactReference{
			Name:             name,
			Tag:              tag,
			OriginalLocation: location,
			LocalCachePath:   fCache.Name(),
			Digest:           hex.EncodeToString(hasher.Sum(nil)),
			Size:             int(fCacheInfo.Size()),
		}, nil
	}
	return nil, errors.New("nex artifact not found in provided oci manifest")
}
