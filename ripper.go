package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
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
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
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

func (r *ripper) rip(svsFile string) error {

	// Download the HTML of the WebScope page to extract the dimensions of the
	// full image.
	webscopeURL := fmt.Sprintf("%s%s.svs/view.apml", r.baseUrl, svsFile)
	resp, err := http.Get(webscopeURL)
	if err != nil {
		return errors.Wrapf(err, "getting webscope page failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Wrapf(err, "http status %s", resp.Status)
	}

	html, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "http read failed")
	}

	// Extract height and width of the image.
	heightMatch := r.heightRegex.FindSubmatch(html)
	if len(heightMatch) != 2 {
		return fmt.Errorf("can't get height, %d regex matches", len(heightMatch))
	}
	height, err := strconv.Atoi(string(heightMatch[1]))
	if err != nil {
		return errors.Wrapf(err, "failed to parse height")
	}

	widthMatch := r.widthRegex.FindSubmatch(html)
	if len(widthMatch) != 2 {
		return fmt.Errorf("can't get width, %d regex matches", len(widthMatch))
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
	log.Printf("Ripping %s.svs", svsFile)
	log.Printf("")
	log.Printf("Image size: %v x %v", width, height)
	log.Printf("Number of tiles: %d x %d = %d", xTiles, yTiles, numTiles)
	log.Printf("")

	// Make output dirs for this image.
	imgDir := filepath.Join(r.outputDir, svsFile)
	err = os.MkdirAll(imgDir, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failed to make output dir for %s", svsFile)
	}
	tilesDir := filepath.Join(r.outputDir, svsFile, "tiles")
	err = os.MkdirAll(tilesDir, os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "failed to make output dir for %s", svsFile)
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

	previewScale := 4
	completeImg := image.NewRGBA(image.Rect(0, 0, width/previewScale, height/previewScale))

	// Download tiles.
	for x := 0; x < xTiles; x++ {
		for y := 0; y < yTiles; y++ {

			c.Wait()

			go func(x, y int) {
				fileName := fmt.Sprintf("%dx%d.jpeg", x, y)
				path := filepath.Join(tilesDir, fileName)

				err := r.downloadTile(x, y, path, svsFile)
				if err != nil {
					// Try to delete the partially downloaded svs, ignore errors.
					_ = os.RemoveAll(imgDir)
					panic(err)
				}

				// Add the downloaded image to the complete image.
				imgFile, err := os.Open(path)
				if err != nil {
					// Try to delete the partially downloaded svs, ignore errors.
					_ = os.RemoveAll(imgDir)
					panic(err)
				}
				img, _, err := image.Decode(imgFile)
				if err != nil {
					// Try to delete the partially downloaded svs, ignore errors.
					_ = os.RemoveAll(imgDir)
					panic(err)
				}

				// Where to draw this image inside the complete image.
				drawX := x * r.tileSize / previewScale
				drawY := y * r.tileSize / previewScale
				drawPos := image.Point{drawX, drawY}

				// Where to draw the image inside the output image.
				drawRect := image.Rectangle{drawPos, drawPos.Add(img.Bounds().Size().Div(previewScale))}

				draw.CatmullRom.Scale(completeImg, drawRect, img, img.Bounds(), draw.Over, nil)

				d := &font.Drawer{
					Dst:  completeImg,
					Src:  image.NewUniform(color.Black),
					Face: basicfont.Face7x13,
					Dot:  fixed.Point26_6{fixed.I(drawX + 3), fixed.I(drawY + 13)},
				}
				d.DrawString(fileName)

				bar.Add(1)
				c.Done()
			}(x, y)
		}
	}

	// Wait for all tiles to be downloaded.
	c.WaitAllDone()

	// Add grid lines to preview image.
	for x := 0; x < xTiles; x++ {
		for y := 0; y < completeImg.Rect.Dy(); y++ {
			completeImg.Set(x*r.tileSize/previewScale, y, color.Black)
		}
	}

	for y := 0; y < yTiles; y++ {
		for x := 0; x < completeImg.Rect.Dx(); x++ {
			completeImg.Set(x, y*r.tileSize/previewScale, color.Black)
		}
	}

	// Write preview image.
	previewFile := filepath.Join(imgDir, "preview.jpeg")
	out, err := os.Create(previewFile)
	if err != nil {
		// Try to delete the partially downloaded svs, ignore errors.
		_ = os.RemoveAll(imgDir)
		panic(err)
	}

	var opt jpeg.Options
	opt.Quality = 100
	log.Printf("Writing preview image...")
	jpeg.Encode(out, completeImg, &opt)

	return nil
}

func (r *ripper) downloadTile(x, y int, path, svsFile string) error {

	// Construct the URL for this tile.
	tileUrl := fmt.Sprintf("%s%s.svs?%d+%d+%d+%d+%d", r.baseUrl,
		svsFile, x*r.tileSize, y*r.tileSize, r.tileSize, r.tileSize, r.zoomLevel)

	// Download the tile.
	resp, err := http.Get(tileUrl)
	if err != nil {
		return errors.Wrapf(err, "getting tile failed")
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
