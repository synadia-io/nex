package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type reference struct {
	RawRef     string
	Repo       *remote.Repository
	Descriptor v1.Descriptor
	Manifest   *v1.Manifest
}

type RootFS struct {
	OciRef    string            `arg:"" name:"oci-ref" help:"OCI image reference"`
	Files     map[string]string `placeholder:"src=dest;..." help:"Add a file to the rootfs"`
	Os        string            `help:"Set the OS to use when parsing the OCI reference" default:"linux"`
	Arch      string            `help:"Set the architecture to use when parsing the OCI reference" default:"amd64"`
	Status    bool              `help:"Print the status"`
	OutputDir string            `help:"Directory to output rootfs into" default:"."`
	NoTLS     bool              `help:"Disable TLS when connecting to registry" default:"false"`

	ociRef   *reference `kong:""`
	buildDir string     `kong:""`
}

func (r *RootFS) Run() error {
	var err error

	if os.Getuid() != 0 {
		return errors.New("Please run as root")
	}

	r.buildDir, err = os.MkdirTemp(os.TempDir(), "rootfs-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(r.buildDir)

	r.ociRef = &reference{
		RawRef: r.OciRef,
	}

	err = r.Initialize()
	if err != nil {
		return err
	}

	err = r.downloadExtractLayers()
	if err != nil {
		return err
	}

	for src, dst := range r.Files {
		err = r.AddFile(src, dst)
		if err != nil {
			return err
		}
	}

	err = r.makeRootFS()
	if err != nil {
		return err
	}

	return nil
}

func (r *RootFS) Initialize() error {
	repo, err := remote.NewRepository(r.ociRef.RawRef)
	if err != nil {
		return err
	}
	if r.NoTLS {
		repo.PlainHTTP = true
	}
	r.ociRef.Repo = repo

	repo.Client = &auth.Client{
		Cache: auth.DefaultCache,
		Credential: func(ctx context.Context, registry string) (auth.Credential, error) {
			switch r.ociRef.Repo.Reference.Registry {
			case "docker.io":
				authGet, err := http.Get(fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo.Reference.Repository))
				if err != nil {
					return auth.Credential{}, fmt.Errorf("failed to fetch auth token: %w", err)
				}
				defer authGet.Body.Close()
				authBody := make(map[string]any)
				if err := json.NewDecoder(authGet.Body).Decode(&authBody); err != nil {
					return auth.Credential{}, fmt.Errorf("failed to decode auth response: %w", err)
				}
				token, ok := authBody["token"].(string)
				if !ok {
					return auth.Credential{}, errors.New("failed to parse token")
				}
				cred := auth.Credential{
					AccessToken: token,
				}
				return cred, nil
			default:
				return auth.EmptyCredential, nil
			}
		},
	}

	r.ociRef.Descriptor, err = repo.Resolve(context.TODO(), repo.Reference.Reference)
	if err != nil {
		return err
	}

	manifestBytes, err := repo.Fetch(context.TODO(), r.ociRef.Descriptor)
	if err != nil {
		return err
	}

	// •	OCI Index (v1.Index): application/vnd.oci.image.index.v1+json
	//   •	Represents a multi-platform image.
	//   •	Contains a list of manifests, each with a Platform field indicating the OS and architecture.
	// •	OCI Manifest (v1.Manifest): application/vnd.oci.image.manifest.v1+json
	//   •	Describes an image for a specific platform.
	//   •	Contains a list of layers and a config object.
	switch r.ociRef.Descriptor.MediaType {
	case v1.MediaTypeImageManifest:
		var manifest v1.Manifest
		indexDecoder := json.NewDecoder(manifestBytes)
		if err := indexDecoder.Decode(&manifest); err != nil {
			return err
		}
		r.ociRef.Manifest = &manifest
	case v1.MediaTypeImageIndex:
		var index v1.Index
		manifestDecoder := json.NewDecoder(manifestBytes)
		if err := manifestDecoder.Decode(&index); err != nil {
			return err
		}
		for _, manifest := range index.Manifests {
			if manifest.MediaType == v1.MediaTypeImageManifest && manifest.Platform.OS == r.Os && manifest.Platform.Architecture == r.Arch {
				manifestBytes, err := repo.Fetch(context.TODO(), manifest)
				if err != nil {
					return err
				}
				var manifest v1.Manifest
				indexDecoder := json.NewDecoder(manifestBytes)
				if err := indexDecoder.Decode(&manifest); err != nil {
					return err
				}
				r.ociRef.Manifest = &manifest
				break
			}
		}
	}

	if r.ociRef.Manifest == nil {
		return errors.New("no matching manifest found")
	}

	return nil
}

func (r *RootFS) Validate() error {
	return nil
}

func (r *RootFS) downloadExtractLayers() error {
	for _, layer := range r.ociRef.Manifest.Layers {
		reader, err := r.ociRef.Repo.Blobs().Fetch(context.TODO(), layer)
		if err != nil {
			return err
		}
		defer reader.Close()

		bs := new(bytes.Buffer)
		pr := ProgressReader{
			Action: "Downloading",
			Reader: reader,
			Total:  layer.Size,
			Title:  layer.Digest.Encoded()[len(layer.Digest.Encoded())-8:],
			Status: r.Status,
		}

		_, err = io.Copy(bs, &pr)
		if err != nil {
			return err
		}
		if r.Status {
			fmt.Println()
		}

		err = extractLayer(bs, r.buildDir)
		if err != nil {
			return err
		}
	}

	return nil
}

// └─❯ mke2fs -t ext4 -d rootfs rootfs.ext4 150M
// └─❯ resize2fs -M ./rootfs.ext4
func (r *RootFS) makeRootFS() error {
	err := os.MkdirAll(r.OutputDir, 0755)
	if err != nil {
		return err
	}

	err = exec.Command("mke2fs", "-t", "ext4", "-d", r.buildDir, filepath.Join(r.OutputDir, "rootfs.ext4"), "150M").Run()
	if err != nil {
		return err
	}

	err = exec.Command("resize2fs", "-M", filepath.Join(r.OutputDir, "rootfs.ext4")).Run()
	if err != nil {
		return err
	}
	return nil
}

func (r *RootFS) AddFile(src, dest string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("file %s does not exist", src)
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", src, err)
	}
	defer srcFile.Close()

	mvDest := filepath.Join(r.buildDir, filepath.Dir(dest))
	err = os.MkdirAll(mvDest, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory for file %s: %w", mvDest, err)
	}

	rfsFileName := filepath.Base(dest)
	if rfsFileName == "." {
		rfsFileName = filepath.Base(src)
	}

	dstFile, err := os.Create(filepath.Join(mvDest, rfsFileName))
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	err = dstFile.Chmod(0755)
	if err != nil {
		return err
	}

	err = dstFile.Chown(1000, 1000)
	if err != nil {
		return err
	}

	return nil
}

func extractLayer(tarGzReader io.Reader, rootfsDir string) error {
	gzReader, err := gzip.NewReader(tarGzReader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar header: %v", err)
		}

		target := filepath.Join(rootfsDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("mkdir failed: %v", err)
			}

		case tar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("file creation failed: %v", err)
			}

			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("file copy failed: %v", err)
			}
			file.Close()

		case tar.TypeSymlink:
			if _, err := os.Stat(target); err == nil {
				err := os.Remove(target)
				if err != nil {
					return fmt.Errorf("failed to remove existing file while creating symlink: %v", err)
				}
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("symlink failed: %v", err)
			}

		case tar.TypeLink:
			if err := os.Link(filepath.Join(rootfsDir, header.Linkname), target); err != nil {
				return fmt.Errorf("hard link creation failed: %v", err)
			}

		default:
			continue
		}

		if header.Typeflag != tar.TypeSymlink && header.Typeflag != tar.TypeLink {
			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("chmod failed: %v", err)
			}

			if err := os.Chown(target, header.Uid, header.Gid); err != nil {
				fmt.Printf("target: %v, header.Uid: %v, header.Gid: %v\n", target, header.Uid, header.Gid)
				return fmt.Errorf("chown failed: %v", err)
			}
		}
	}

	return nil
}

type ProgressReader struct {
	Action     string
	Reader     io.Reader
	Total      int64
	Downloaded int64
	LastPrint  time.Time
	Title      string
	Status     bool
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Downloaded += int64(n)
	now := time.Now()
	if now.Sub(pr.LastPrint) >= time.Millisecond*100 || err == io.EOF {
		pr.printProgress()
		pr.LastPrint = now
	}
	return n, err
}

func (pr *ProgressReader) printProgress() {
	percent := float64(pr.Downloaded) / float64(pr.Total) * 100
	barWidth := 50
	filled := int(percent / 100 * float64(barWidth))
	bar := "[" + string(repeat('=', filled)) + string(repeat(' ', barWidth-filled)) + "]"
	if pr.Status {
		fmt.Printf("\r%s [%s]: %s %.2f%%", pr.Action, pr.Title, bar, percent)
	}
}

func repeat(char rune, count int) []rune {
	result := make([]rune, count)
	for i := range result {
		result[i] = char
	}
	return result
}
