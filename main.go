package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	bolt "go.etcd.io/bbolt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type Location struct {
	Name string  `json:"suburb"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type Comparison struct {
	Key      string
	Distance float64
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

		lon, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			continue
		}

		locations = append(locations, Location{row[0], lat, lon})
	}

	return locations, nil
}

func insertToDb(loc Location, bucketName string, dbName string) error {
	db, err := bolt.Open(dbName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("failed to create bucket: %v", err)
		}

		enc, err := json.Marshal(loc)
		if err != nil {
			return fmt.Errorf("failed to encode location '%s': %v", loc.Name, err)
		}

		if err := bucket.Put([]byte(loc.Name), enc); err != nil {
			return fmt.Errorf("failed to insert '%s': %v", loc.Name, err)
		}
		return nil
	})
}

func loadGeoData(path string, dbName string) {
	locations, err := parseGeoData(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, l := range locations {
		if err := insertToDb(l, "lat_lon", dbName); err != nil {
			log.Println(err)
		}
	}
}

func upperCaseFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToUpper(v)) + str[i+1:]
	}
	return ""
}

func sanitizeKey(key string) string {
	var out string
	out = strings.Replace(key, " NSW", "", 1)
	out = strings.Replace(out, " VIC", "", 1)
	out = strings.Replace(out, " QLD", "", 1)
	out = strings.Replace(out, " TAS", "", 1)
	return out
}

func saveToFeedDb(key string, data []byte, bucketName string, db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		if err != nil {
			return fmt.Errorf("failed to create bucket: %v", err)
		}

		if err := bucket.Put([]byte(key), data); err != nil {
			return fmt.Errorf("failed to save to feed db '%s': %v", key, err)
		}
		return nil
	})
}

func loadFeedData(dir string, feedDB string) {
	db, err := bolt.Open(feedDB, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		var err error

		path := filepath.Join(dir, f.Name())
		b, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}

		fname := upperCaseFirst(f.Name())
		extn := filepath.Ext(f.Name())
		key := fname[0 : len(fname)-len(extn)]

		err = saveToFeedDb(key, b, "feed_data", db)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func lookupFeedData(key string, bucketName string, feedDB string) (SuburbRecord, error) {
	db, err := bolt.Open(feedDB, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var record SuburbRecord

	dbErr := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("failed to get '%s' bucket", bucketName)
		}

		b := bucket.Get([]byte(key))
		if b == nil {
			return fmt.Errorf("failed to find data for '%s'", key)
		}

		if err := json.Unmarshal(b, &record); err != nil {
			return fmt.Errorf("failed to unmarshal data for '%s'", key)
		}

		return nil
	})

	return record, dbErr
}

func lookupGeoData(name string, bucketName string, db *bolt.DB) (Location, error) {
	var loc Location

	dbErr := db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		if bucket == nil {
			return fmt.Errorf("failed to get '%s' bucket", bucketName)
		}

		b := bucket.Get([]byte(name))
		if b == nil {
			return fmt.Errorf("failed to find data for '%s'", name)
		}

		if err := json.Unmarshal(b, &loc); err != nil {
			return fmt.Errorf("failed to unmarshal data for '%s'", name)
		}

		return nil
	})

	return loc, dbErr
}

func haversine(theta float64) float64 {
	return math.Pow(math.Sin(theta/2), 2)
}

func distance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert to radians
	// Must cast radius as float to multiply later
	var la1, lo1, la2, lo2, r float64
	la1 = lat1 * math.Pi / 180
	lo1 = lon1 * math.Pi / 180
	la2 = lat2 * math.Pi / 180
	lo2 = lon2 * math.Pi / 180

	r = 6378100 // Earth radius in meters

	h := haversine(la2-la1) + math.Cos(la1)*math.Cos(la2)*haversine(lo2-lo1)

	return 2 * r * math.Asin(math.Sqrt(h))
}

func findNearest(lat float64, lon float64, bucketName string, dbName string) (Location, error) {
	db, err := bolt.Open(dbName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var comparisons []Comparison

	compareEach := func(key, value []byte) error {
		var loc Location
		if err := json.Unmarshal(value, &loc); err == nil {
			comparisons = append(comparisons, Comparison{
				string(key),
				distance(lat, lon, loc.Lat, loc.Lon),
			})
		}

		return nil
	}

	db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		b.ForEach(compareEach)
		return nil
	})

	min := Comparison{"", math.MaxFloat64}
	for _, c := range comparisons {
		if c.Distance < min.Distance {
			min = c
		}
	}

	return lookupGeoData(min.Key, "lat_lon", db)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	paths := strings.Split(r.URL.Path, "/")

	if len(paths) < 3 {
		http.NotFound(w, r)
		return
	}

	if paths[1] != "newsfeed" || paths[2] != "location" {
		http.NotFound(w, r)
		return
	}

	param1 := r.URL.Query()["lat"][0]
	param2 := r.URL.Query()["lon"][0]

	lat, err := strconv.ParseFloat(param1, 64)
	if err != nil {
		log.Fatal(err)
	}

	lon, err := strconv.ParseFloat(param2, 64)
	if err != nil {
		log.Fatal(err)
	}

	nearest, err := findNearest(lat, lon, "lat_lon", "news_nearby.db")
	if err != nil {
		log.Fatal(err)
	}

	sanitized := sanitizeKey(nearest.Name)

	record, err := lookupFeedData(sanitized, "feed_data", "news_nearby.db")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	b, err := json.Marshal(record)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func main() {
	setupDb("news_nearby.db")
	loadGeoData("lat_lon.csv", "news_nearby.db")
	loadFeedData("./rawdata", "news_nearby.db")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	http.HandleFunc("/", indexHandler)

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
