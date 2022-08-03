package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
)

const (
	baseUrl     = "http://image.upmc.edu:8080/NikiForov%20EFV%20Study/BoxA/"
	outputDir   = "output"
	tileSize    = 512
	zoomLevel   = 1
	concurrency = 10
)

func main() {
	log.SetFlags(0)
	svsFiles := os.Args[1:]

	// Create output root directory.
	err := os.Mkdir(outputDir, os.ModePerm)
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

	ripper, err := newRipper(baseUrl, outputDir, zoomLevel, tileSize, concurrency)
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
