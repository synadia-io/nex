package preflight

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func decompressAndValidateTarFromURL(url string, knownGoodShaURL string) (*tar.Reader, bool, PreflightError) {
	respTar, err := http.Get(url)
	if err != nil || respTar.StatusCode != http.StatusOK {
		return nil, false, ErrFailedToDownload
	}
	defer respTar.Body.Close()

	rawTar, err := io.ReadAll(respTar.Body)
	if err != nil {
		return nil, false, ErrFailedToDownload
	}

	var hashBuffer bytes.Buffer
	gzipReader := io.TeeReader(bytes.NewReader(rawTar), &hashBuffer)

	uncompressedTar, err := gzip.NewReader(gzipReader)
	if err != nil {
		return nil, false, ErrFailedToUncompress
	}
	rawData := tar.NewReader(uncompressedTar)

	if knownGoodShaURL != "" {
		kgSha, err := getKnownGoodSha256FromURL(knownGoodShaURL)
		if err != nil {
			return nil, false, ErrFailedToDownloadKnownGoodSha
		}

		_, err = validateSha256(&hashBuffer, kgSha)
		if err != nil {
			return nil, false, err
		}
		return rawData, true, nil
	}

	return rawData, false, nil
}

func addQuotes(urls []string) string {
	ret := strings.Builder{}
	ret.WriteRune('[')
	for i, url := range urls {
		ret.WriteString(fmt.Sprintf("\"%s\"", url))
		if i < len(urls)-1 {
			ret.WriteRune(',')
		}
	}
	ret.WriteRune(']')
	return ret.String()
}

func validateSha256(data io.Reader, knownGood string) (string, PreflightError) {
	h := sha256.New()
	if _, err := io.Copy(h, data); err != nil {
		return "", ErrFailedToCalculateSha256
	}
	shasum := hex.EncodeToString(h.Sum(nil))
	if shasum != knownGood {
		return shasum, ErrSha256Mismatch
	}
	return shasum, nil
}

func getKnownGoodSha256FromURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", err
	}
	defer resp.Body.Close()

	sha, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(sha), nil
}

func checkContinue(msg string) PreflightError {
	fmt.Printf("⛔ %s [y/N] ", msg)
	inputReader := bufio.NewReader(os.Stdin)
	input, err := inputReader.ReadSlice('\n')
	if err != nil {
		return err
	}
	if strings.ToUpper(string(input)) != "Y\n" {
		return ErrUserCanceledPreflight
	}
	return nil
}
