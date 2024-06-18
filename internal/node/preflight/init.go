package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/fatih/color"
)

var (
	cyan    = color.New(color.FgCyan).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	green   = color.New(color.FgHiGreen).SprintFunc()

	nexLatestVersion = func(ctx context.Context) string {
		version, ok := ctx.Value("version").(string)
		if !ok {
			version = "development"
		}
		if version == "development" {
			res, err := http.Get("https://api.github.com/repos/synadia-io/nex/releases/latest")
			if err != nil {
				fmt.Printf("error making http request: %s\n", err)
				return ""
			}
			defer res.Body.Close()

			b, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Printf("error reading body: %s\n", err)
				return ""
			}

			payload := make(map[string]interface{})
			err = json.Unmarshal(b, &payload)
			if err != nil {
				fmt.Printf("error parsing json: %s\n", err)
				return ""
			}

			latestTag, ok := payload["tag_name"].(string)
			if !ok {
				fmt.Println("error parsing tag_name")
				return ""
			}
			return latestTag
		} else {
			return version
		}
	}
)
