package main

import (
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
)

var data [][]string
var cols map[string]int
var requests atomic.Int64

func load() {
	f, err := os.Open("data/diabetes_dataset.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	data = rows
	cols = map[string]int{}
	for i, col := range rows[0] {
		cols[col] = i
	}
}

func diabetesRate(rows [][]string) float64 {
	if len(rows) == 0 {
		return 0
	}

	sick := 0
	for _, row := range rows {
		if row[cols["diagnosed_diabetes"]] == "1" {
			sick++
		}
	}
	return float64(sick) * 100 / float64(len(rows))
}

func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func main() {
	load()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]string{
			"status":  "ok",
			"message": "Glucolens API is running",
		})
	})

	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{
			"people":  len(data) - 1,
			"rate":    diabetesRate(data[1:]),
			"columns": data[0],
		})
	})
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		name := r.URL.Query().Get("column")
		index, ok := cols[name]
		if !ok {
			http.Error(w, "column not found", http.StatusNotFound)
			return
		}
		groups := map[string][]string{}
		for _, row := range data[1:] {
			groups[row[index]] = append(groups[row[index]], row[cols["diagnosed_diabetes"]])
		}
		result := map[string]any{}
		for label, values := range groups {
			sick := 0
			for _, v := range values {
				if v == "1" {
					sick++
				}
			}
			result[label] = map[string]any{
				"count": len(values),
				"rate":  float64(sick) * 100 / float64(len(values)),
			}
		}
		reply(w, result)
	})
	mux.HandleFunc("/api/people", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		age, err := strconv.Atoi(r.URL.Query().Get("age"))
		if err != nil {
			http.Error(w, "invalid age", http.StatusBadRequest)
			return
		}
		bmi, err := strconv.ParseFloat(r.URL.Query().Get("bmi"), 64)
		if err != nil {
			http.Error(w, "invalid bmi", http.StatusBadRequest)
			return
		}

		matches := [][]string{}
		for _, row := range data[1:] {
			rowAge, err1 := strconv.Atoi(row[cols["age"]])
			rowBmi, err2 := strconv.ParseFloat(row[cols["bmi"]], 64)

			if err1 != nil || err2 != nil {
				continue
			}

			if rowAge >= age-5 && rowAge <= age+5 && rowBmi >= bmi-2 && rowBmi <= bmi+2 {
				matches = append(matches, row)
			}
		}

		reply(w, map[string]any{
			"count": len(matches),
			"rate":  diabetesRate(matches),
		})
	})

	mux.HandleFunc("/api/ops", func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{
			"requests": requests.Load(),
			"rows":     len(data) - 1,
		})
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mux.ServeHTTP(w, r)
	})
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}
