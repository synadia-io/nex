package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func versionCheck() error {
	if VERSION == "development" {
		return nil
	}

	res, err := http.Get("https://api.github.com/repos/synadia-io/nex/releases/latest")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	payload := make(map[string]interface{})
	err = json.Unmarshal(b, &payload)
	if err != nil {
		return err
	}

	latestTag, ok := payload["tag_name"].(string)
	if !ok {
		return errors.New("error parsing tag_name")
	}

	if latestTag != VERSION {
		fmt.Printf(`================================================================
🎉 There is a newer version [v%s] of the NEX CLI available 🎉
================================================================

`,
			latestTag)
	}

	return nil
}
