package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/schollz/progressbar/v3"
	"github.com/zenthangplus/goccm"
)

type ripper struct {
	baseUrl     string
	outputDir   string
	zoomLevel   int
	tileSize    int
	concurrency int

	heightRegex *regexp.Regexp
	widthRegex  *regexp.Regexp
}

func newRipper(baseUrl string, outputDir string, zoomLevel, tileSize, concurrency int) (*ripper, error) {
	// Compile regular expressions to be used later.
	heightRegex, err := regexp.Compile("height: \"(\\d+)\"")
	if err != nil {
		return nil, errors.Wrapf(err, "compiling height regex failed")
	}
	widthRegex, err := regexp.Compile("width: \"(\\d+)\"")
	if err != nil {
		return nil, errors.Wrapf(err, "compiling width regex failed")
	}

	return &ripper{
		baseUrl:     baseUrl,
		outputDir:   outputDir,
		zoomLevel:   zoomLevel,
		tileSize:    tileSize,
		concurrency: concurrency,

		heightRegex: heightRegex,
		widthRegex:  widthRegex,
	}, nil
}

func (r *ripper) rip(image string) error {

	// Download the HTML of the WebScope page to extract the dimensions of the
	// full image.
	webscopeURL := fmt.Sprintf("%s%s.svs/view.apml", r.baseUrl, image)
	resp, err := http.Get(webscopeURL)
	if err != nil {
		return errors.Wrapf(err, "http get failed")
	}
	defer resp.Body.Close()

	html, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "http read failed")
	}

	// Extract height and width of the image.
	heightMatch := r.heightRegex.FindSubmatch(html)
	if len(heightMatch) != 2 {
		return fmt.Errorf("can't get height")
	}
	height, err := strconv.Atoi(string(heightMatch[1]))
	if err != nil {
		return errors.Wrapf(err, "failed to parse height")
	}

	widthMatch := r.widthRegex.FindSubmatch(html)
	if len(widthMatch) != 2 {
		return fmt.Errorf("can't get width")
	}
	width, err := strconv.Atoi(string(widthMatch[1]))
	if err != nil {
		return errors.Wrapf(err, "failed to parse width")
	}

	// Output some useful info before downloading the image.
	xTiles := width / 512
	yTiles := height / 512
	numTiles := xTiles * yTiles

	log.Printf("")
	log.Printf("Ripping %s.svs", image)
	log.Printf("")
	log.Printf("Image size: %v x %v", width, height)
	log.Printf("Number of tiles: %d x %d = %d", xTiles, yTiles, numTiles)
	log.Printf("")

	// Make output dir for this image.
	imgDir := filepath.Join(r.outputDir, image)
	err = os.Mkdir(imgDir, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failed to make output dir for %s", image)
	}

	// Prepare a progress bar.
	bar := progressbar.NewOptions(numTiles,
		progressbar.OptionSetDescription("Downloading..."),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(45),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			log.Println()
		}),
	)

	// Limit number of concurrent tile downloads.
	c := goccm.New(r.concurrency)

	// Download tiles.
	for x := 0; x < xTiles; x++ {
		for y := 0; y < yTiles; y++ {

			c.Wait()

			go func(x, y int) {
				err := r.downloadTile(x, y, image, imgDir)
				if err != nil {
					// Try to delete the partially downloaded image, ignore errors.
					_ = os.RemoveAll(imgDir)
					panic(err)
				}
				bar.Add(1)
				c.Done()
			}(x, y)
		}
	}

	// Wait for all tiles to be downloaded.
	c.WaitAllDone()

	return nil
}

func (r *ripper) downloadTile(x, y int, image, imgDir string) error {

	fileName := fmt.Sprintf("%dx%d.jpeg", x, y)
	path := filepath.Join(imgDir, fileName)

	// Construct the URL for this tile.
	tileUrl := fmt.Sprintf("%s%s.svs?%d+%d+%d+%d+%d", r.baseUrl,
		image, x*r.tileSize, y*r.tileSize, r.tileSize, r.tileSize, r.zoomLevel)

	// Download the tile.
	resp, err := http.Get(tileUrl)
	if err != nil {
		return errors.Wrapf(err, "http get failed")
	}
	defer resp.Body.Close()

	// Save the tile.
	out, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "creating image file failed")
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return errors.Wrapf(err, "writing to image file failed")
	}

	return nil
}
