package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/jessevdk/go-flags"
)

const (
	outputDir   = "output"
	tileSize    = 512
	zoomLevel   = 1
	concurrency = 10
)

var args struct {
	BaseUrl string `short:"u" long:"url" description:"The base url of the ImageScope files." required:"true"`
}

func main() {
	log.SetFlags(0)

	svsFiles, err := flags.Parse(&args)
	if err != nil {
		os.Exit(1)
	}

	if len(svsFiles) == 0 {
		log.Print("No image files specified")
		os.Exit(1)
	}

	// Create output root directory.
	err = os.Mkdir(outputDir, os.ModePerm)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			log.Printf("Making output dir failed: %v", err)
			os.Exit(1)
		}
	}

	// Exit early if any of the requested images already exist.
	for _, svsFile := range svsFiles {
		_, err := os.Stat(filepath.Join(outputDir, svsFile))
		if !os.IsNotExist(err) {
			log.Printf("Output dir for %s already exists", svsFile)
			os.Exit(1)
		}
	}

	ripper, err := newRipper(args.BaseUrl, outputDir, zoomLevel, tileSize, concurrency)
	if err != nil {
		log.Printf("Initialization failed: %v", err)
		os.Exit(1)
	}

	for _, svsFile := range svsFiles {
		err = ripper.rip(svsFile)
		if err != nil {
			log.Printf("Ripping svs file %s failed: %v", svsFile, err)
		}
	}
}
