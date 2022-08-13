package main

import (
	"context"
	"log"
	"time"

	"github.com/Code-Hex/vz/v2"
)

func main() {
	progressReader, err := vz.FetchLatestSupportedMacOSRestoreImage(context.Background(), "RestoreImage.ipsw")
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Printf("progress: %f", progressReader.FractionCompleted()*100)
		case <-progressReader.Finished():
			log.Println("finished", progressReader.Err())
			return
		}
	}

	// log.Println(restoreImage.BuildVersion())
	// log.Println(restoreImage.URL())
	// log.Println(restoreImage.OperatingSystemVersion())
	// log.Println(restoreImage.MostFeaturefulSupportedConfiguration())
	// log.Println(err, "err == nil", err == nil)

}
