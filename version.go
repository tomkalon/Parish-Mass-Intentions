package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed build/windows/winres.json
var winres []byte

func appVersion(data []byte) string {
	var resources struct {
		Version map[string]map[string]struct {
			Info map[string]struct {
				ProductVersion string
			} `json:"info"`
		} `json:"RT_VERSION"`
	}

	err := json.Unmarshal(data, &resources)
	if err != nil {
		return ""
	}

	for _, languages := range resources.Version {
		for _, version := range languages {
			for _, info := range version.Info {
				if info.ProductVersion != "" {
					return info.ProductVersion
				}
			}
		}
	}

	return ""
}
