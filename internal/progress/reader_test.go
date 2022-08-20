package progress_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v2/internal/progress"
)

func TestReader(t *testing.T) {
	var buf bytes.Buffer

	pr := progress.NewReader(&buf, 100, 0)

	buf.WriteString(strings.Repeat("a", 50))

	_, err := pr.Read(make([]byte, 50))
	if err != nil {
		t.Fatal(err)
	}

	if got := pr.Current(); got != 50 {
		t.Fatalf("want 50 but got %d", got)
	}

	if got := pr.FractionCompleted(); got != 0.5 {
		t.Fatalf("want 0.5 but got %f", got)
	}
}

func TestReaderFinish(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "error is nil",
			err:  nil,
		},
		{
			name: "something error",
			err:  errors.New("error"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			pr := progress.NewReader(&buf, 100, 0)
			pr.Finish(tc.err)

			select {
			case <-pr.Finished():
			case <-time.After(time.Second):
				t.Fatal("failed incoming channel")
			}

			if got := pr.Err(); got != tc.err {
				t.Fatalf("want %v but got %v", tc.err, got)
			}
		})
	}
}
