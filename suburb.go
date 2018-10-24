package main

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

