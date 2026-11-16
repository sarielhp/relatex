package alttext

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/jpeg" // a JPEG figure is decoded to be turned
	"image/png"
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/figdesc"
	"github.com/sarielhp/relatex/internal/rec"
)

// writeImage puts the figure in directory as a file a describer can read (placeImage), turned as
// the paper prints it (\includegraphics[angle=…], decision 123 amendment 3): a PNG or JPEG picture
// of a turned figure becomes a turned PNG, as its geometry is turned. A picture that cannot be
// decoded (a PDF left as it is, an SVG) stays as drawn.
func writeImage(document rec.Map, figure *figdesc.Figure, directory string) (string, string, error) {
	path, mime, err := placeImage(document, figure, directory)
	if err != nil || figure.Turns == 0 || (mime != "image/png" && mime != "image/jpeg") {
		return path, mime, err
	}
	turned, err := turnImageFile(path, figure.Turns, filepath.Join(directory, "figure-turned.png"))
	if err != nil {
		return path, mime, nil
	}
	return turned, "image/png", nil
}

// turnImageFile writes the picture at path turned by quarter turns counterclockwise as a PNG at
// target.
func turnImageFile(path string, quarters int, target string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, turnImage(picture, quarters)); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, out.Bytes(), 0o600)
}

// turnImage is a picture turned by quarter turns counterclockwise (0 to 3).
func turnImage(picture image.Image, quarters int) image.Image {
	quarters = ((quarters % 4) + 4) % 4
	bounds := picture.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(source, source.Bounds(), picture, bounds.Min, draw.Src)
	turnedWidth, turnedHeight := width, height
	if quarters%2 == 1 {
		turnedWidth, turnedHeight = height, width
	}
	turned := image.NewRGBA(image.Rect(0, 0, turnedWidth, turnedHeight))
	for y := range height {
		for x := range width {
			tx, ty := turnPixel(x, y, width, height, quarters)
			turned.Set(tx, ty, source.At(x, y))
		}
	}
	return turned
}

// turnPixel is where pixel (x, y) of a width × height picture (y down) lands when the picture is
// turned by quarter turns counterclockwise.
func turnPixel(x, y, width, height, quarters int) (int, int) {
	switch quarters {
	case 1:
		return y, width - 1 - x
	case 2:
		return width - 1 - x, height - 1 - y
	case 3:
		return height - 1 - y, x
	}
	return x, y
}
