package jsdelivr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/donseba/go-importmap/library"
)

var (
	defaultApiBaseURL = "https://data.jsdelivr.com/v1/package/npm/"
	defaultCdnBaseURL = "https://cdn.jsdelivr.net/npm/"
)

type (
	Client struct {
		apiBaseURL string
		cdnBaseURL string
	}

	SearchResponse struct {
		Tags     Tags     `json:"tags"`
		Versions []string `json:"versions"`
	}

	Tags struct {
		Latest string `json:"latest"`
		Next   string `json:"next"`
	}

	PackageResponse struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Default string `json:"default"`
		Files   Files  `json:"files"`
		Links   struct {
			Stats       string `json:"stats"`
			Entrypoints string `json:"entrypoints"`
		} `json:"links"`
	}

	File struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Hash  string `json:"hash"`
		Size  int    `json:"size"`
		Files Files  `json:"files,omitempty"`
	}

	Files []File
)

func New() *Client {
	return &Client{
		apiBaseURL: defaultApiBaseURL,
		cdnBaseURL: defaultCdnBaseURL,
	}
}

func (c *Client) FetchPackageFiles(ctx context.Context, name, version string) (library.Files, string, error) {
	url := defaultApiBaseURL + name

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("client api responded with code %d", resp.StatusCode)
	}

	var sr SearchResponse
	err = json.NewDecoder(resp.Body).Decode(&sr)
	if err != nil {
		return nil, "", err
	}

	var (
		useVersion = sr.Tags.Latest
	)

	if version != "" && version != useVersion {
		for _, v := range sr.Versions {
			if version == v {
				useVersion = v
				break
			}
		}
	}

	// get all the files
	vUrl := fmt.Sprintf("%s%s@%s", defaultApiBaseURL, name, useVersion)

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, vUrl, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("client api responded with code %d", resp.StatusCode)
	}

	var pr PackageResponse
	err = json.NewDecoder(resp.Body).Decode(&pr)
	if err != nil {
		return nil, "", err
	}

	basePath := c.cdnBaseURL + name + "@" + useVersion + "/"

	var hasDist bool
	if strings.Contains(pr.Default, "dist") {
		hasDist = true
	}

	var files = walkFiles(pr.Files, basePath, "", hasDist)

	return files, useVersion, nil
}

func walkFiles(files Files, basePath string, filePath string, dist bool) library.Files {
	var f library.Files
	cssJsRe := regexp.MustCompile(`(\.js$|\.css$)`)
	minRe := regexp.MustCompile(`(\.min\.js$|\.min\.css$)`)
	for _, file := range files {
		if file.Type == "directory" {
			if dist && file.Name == "dist" {
				f = append(f, walkFiles(file.Files, basePath, filePath+file.Name+"/", false)...)
				break
			}

			f = append(f, walkFiles(file.Files, basePath, filePath+file.Name+"/", dist)...)
			continue
		}

		f = append(f, library.File{
			Type:      library.ExtractFileType(file.Name),
			Path:      basePath + filePath + file.Name,
			LocalPath: filePath + file.Name,
		})
		if cssJsRe.Match([]byte(file.Name)) {
			if !minRe.Match([]byte(file.Name)) {
				f = append(f, library.File{
					Type:      library.ExtractFileType(file.Name),
					Path:      basePath + filePath + library.FileNameMin(file.Name),
					LocalPath: filePath + library.FileNameMin(file.Name),
				})
			}
		}
	}

	return f
}
