package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	ContentAPIEndpoint = "https://api.ffx.io/api/content/v0/assets/"
)

type SuburbRecord struct {
	SuburbInfo
	Assets []Asset `json:"assets"`
}

type SuburbInfo struct {
	Name     string  `json:"suburb"`
	State    string  `json:"state"`
	Postcode string  `json:"postcode"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

func outputPath(path string, name string) string {
	return filepath.Join(path, strings.ToLower(name)+".json")
}

func writeJSON(path string, s SuburbInfo, assets []Asset) {
	record := SuburbRecord{s, assets}

	b, err := json.Marshal(record)
	if err != nil {
		log.Fatalf("Failed to JSON encode: %v\n", err)
	}

	f := outputPath(path, s.Name)
	if ioutil.WriteFile(f, b, 0644); err != nil {
		log.Fatalf("Failed to write file %s: %v\n", f, err)
	}
}

func fetchAssetData(endpoint string, assetIds []string) []Asset {
	var assets []Asset

	for _, id := range assetIds {
		url := fmt.Sprintf("%s/%s", endpoint, id)

		res, err := http.Get(url)
		if err != nil {
			continue
		}

		var a Asset
		json.NewDecoder(res.Body).Decode(&a)
		assets = append(assets, a)
	}

	return assets
}

func main() {
	var (
		path     string
		endpoint string
		suburb   string
		state    string
		postcode string
		lat      float64
		lon      float64
		assetIds string
	)

	flag.StringVar(&path, "path", "/tmp", "path to the output file")
	flag.StringVar(&endpoint, "endpoint", ContentAPIEndpoint, "path to the output file")
	flag.StringVar(&suburb, "suburb", "", "suburb, e.g. Pyrmont, Bondi, etc.")
	flag.StringVar(&state, "state", "", "state, e.g. NSW, VIC, etc.")
	flag.StringVar(&postcode, "postcode", "", "postcode, e.g. 2009, 2033, etc.")
	flag.Float64Var(&lat, "lat", 0, "latitude (in decimal format)")
	flag.Float64Var(&lon, "lon", 0, "longitude (in decimal format)")
	flag.StringVar(&assetIds, "assetIds", "", "comma separated list of asset IDs")
	flag.Parse()

	info := SuburbInfo{suburb, state, postcode, lat, lon}
	assets := fetchAssetData(endpoint, strings.Split(assetIds, ","))

	writeJSON(path, info, assets)

	fmt.Println("Record created for", suburb, "at", outputPath(path, suburb))
}
