package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/nfnt/resize"
	"golang.org/x/image/bmp"
)

var (
	flagTileSize    int
	flagJpegQuality int
	flagEncoding    string
	flagPattern     string
	flagInterpFunc  string
	flagOutDir      string
)

func init() {
	flag.IntVar(&flagTileSize, "size", 256, "tile size in pixels")
	flag.IntVar(&flagJpegQuality, "q", 5, "jpeg quality setting")
	flag.StringVar(&flagEncoding, "e", "png", "image encoding (png or jpeg)")
	flag.StringVar(&flagPattern, "p", "{zoom}_{x}_{y}.png", "naming pattern for output files")
	flag.StringVar(&flagInterpFunc, "interp", "Bicubic", "cropping interpolation function")
	flag.StringVar(&flagOutDir, "o", "tiles", "output directory for tile files")
}

var validEncodings = []string{"png", "jpeg"}

var interpFuncs = map[string]resize.InterpolationFunction{
	"NearestNeighbor":   resize.NearestNeighbor,
	"Bilinear":          resize.Bilinear,
	"Bicubic":           resize.Bicubic,
	"MitchellNetravali": resize.MitchellNetravali,
	"Lanczos2":          resize.Lanczos2,
	"Lanczos3":          resize.Lanczos3,
}

func main() {
	flag.Parse()

	if flagTileSize <= 0 {
		log.Fatalln("tile size must be a positive integer")
	}

	interpFunc, ok := interpFuncs[flagInterpFunc]
	if !ok {
		fmt.Fprint(os.Stderr, "Valid interpolation function parameters:")
		for fn := range interpFuncs {
			fmt.Fprint(os.Stderr, " "+fn)
		}
	}

	found := false
	for _, enc := range validEncodings {
		if enc == flagEncoding {
			found = true
			break
		}
	}
	if !found {
		log.Fatalln("unsupported encoding:", validEncodings)
	}

	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: tiler [1-n] [filename]")
		return
	}

	_, err := os.Stat(flagOutDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(flagOutDir, 0755); err != nil {
			fmt.Println(err)
			return
		}
	} else if err != nil {
		fmt.Println(err)
		return
	}

	f, err := os.Open(args[1])
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	var img image.Image

	ext := filepath.Ext(f.Name())

	switch ext {
	case ".png":
		img, err = png.Decode(f)
		break
	case ".bmp":
		img, err = bmp.Decode(f)
		break
	default:
		log.Fatal("unsupported file format")
	}
	if err != nil {
		log.Println(err)
		return
	}

	level, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	if level == 0 {
		log.Fatalln("level must be at least 1")
	}

	var wg sync.WaitGroup

	for i := level; i >= 0; i-- {
		wg.Add(1)
		go SplitTiles(img, flagTileSize, int(i), interpFunc, &wg)
	}

	wg.Wait()
}

func SplitTiles(img image.Image, tileSize, level int, interp resize.InterpolationFunction, wg *sync.WaitGroup) {
	defer wg.Done()

	side := 1 << uint(level)
	width := uint(side) * uint(tileSize)
	height := width

	resized := resize.Resize(width, height, img, interp)

	var lwg sync.WaitGroup

	for y := 0; y < side; y++ {
		lwg.Add(1)
		go func(row int) {
			defer lwg.Done()
			for x := 0; x < side; x++ {
				Crop(resized, tileSize, x, row, level)
			}
		}(y)
	}

	lwg.Wait()
}

func Crop(img image.Image, tileSize, x, y, level int) {
	area := image.Rect(x*tileSize, y*tileSize, tileSize+x*tileSize, tileSize+y*tileSize)

	tile := image.Rect(0, 0, tileSize, tileSize)

	dst := image.NewRGBA(tile)

	draw.Draw(dst, tile.Bounds(), img, area.Bounds().Min, draw.Src)

	path := filepath.Join(flagOutDir, fileName(flagPattern, level, x, y))
	f, err := os.Create(path)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	switch flagEncoding {
	case "png":
		err = png.Encode(f, dst)
	case "jpeg":
		err = jpeg.Encode(f, dst, &jpeg.Options{Quality: flagJpegQuality})
	default:
		err = errors.New("encoding not supported")
	}
	if err != nil {
		log.Println(err)
	}
}

func fileName(p string, zoom, x, y int) string {
	p = strings.Replace(p, "{zoom}", strconv.Itoa(zoom), -1)
	p = strings.Replace(p, "{x}", strconv.Itoa(x), -1)
	p = strings.Replace(p, "{y}", strconv.Itoa(y), -1)
	return p
}
