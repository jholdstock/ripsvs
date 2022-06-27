package main

import (
	"errors"
	"fmt"
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

func print(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

func main() {

	images := os.Args[1:]

	// Create output root directory.
	err := os.Mkdir(outputDir, os.ModePerm)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			print("Making output dir failed: %v", err)
			os.Exit(1)
		}
	}

	// Exit early if any of the requested images already exist.
	for _, img := range images {
		imgDir := filepath.Join(outputDir, img)
		_, err := os.Stat(imgDir)
		if !os.IsNotExist(err) {
			print("Output dir for %s already exists", img)
			os.Exit(1)
		}
	}

	ripper, err := newRipper(baseUrl, outputDir, zoomLevel, tileSize, concurrency)
	if err != nil {
		print("Initialization failed: %v", err)
		os.Exit(1)
	}

	for _, img := range images {
		err = ripper.rip(img)
		if err != nil {
			print("Ripping image %s failed: %v", img, err)
		}
	}
}
