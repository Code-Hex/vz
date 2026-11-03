package vz_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

var ci312Started = time.Now()

func ci312Phase(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Log(ci312JSON(t.Name(), format, args...))
}

func ci312Log(format string, args ...any) {
	fmt.Println(ci312JSON(os.Getenv("CI312_TARGET"), format, args...))
}

func ci312JSON(target, format string, args ...any) string {
	count, _ := strconv.Atoi(os.Getenv("CI312_COUNT"))
	process, _ := strconv.Atoi(os.Getenv("CI312_PROCESS_NUMBER"))
	record := struct {
		Case              string  `json:"case"`
		Go                string  `json:"go"`
		Target            string  `json:"target"`
		Count             int     `json:"count"`
		ProcessNumber     int     `json:"processNumber"`
		Phase             string  `json:"phase"`
		MonotonicDuration float64 `json:"monotonicDuration"`
		ExitCode          *int    `json:"exitCode"`
	}{
		Case:              os.Getenv("CI312_CASE"),
		Go:                os.Getenv("CI312_GO"),
		Target:            target,
		Count:             count,
		ProcessNumber:     process,
		Phase:             fmt.Sprintf(format, args...),
		MonotonicDuration: time.Since(ci312Started).Seconds(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return string(data)
}
