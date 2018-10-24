package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type Location struct {
	Name string  `json:"suburb"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

func setupDb(name string) {
	db, err := bolt.Open(name, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
}

func parseGeoData(filePath string) ([]Location, error) {
	csvFile, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(bufio.NewReader(csvFile))
	var locations []Location

loop:
	for {
		row, err := reader.Read()
		switch {
		case err == io.EOF:
			break loop
		case err != nil:
			log.Fatal(err)
		}

		lat, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}

		lon, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}

		locations = append(locations, Location{row[0], lat, lon})
	}

	return locations, nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	parsed, err := parseGeoData("lat_lon.csv")
	if err != nil {
		log.Fatal(err)
	}

	b, err := json.Marshal(parsed)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func main() {
	setupDb("news_nearby.db")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	http.HandleFunc("/", indexHandler)

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
