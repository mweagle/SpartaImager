package transforms

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"

	"github.com/mweagle/SpartaImager/assets"
	"github.com/rs/zerolog"

	// Ensure the JPEG decoder is registered
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
)

func watermarkName(suffix int) string {
	return fmt.Sprintf("/resources/SpartaHelmet%d.png", suffix)
}

// StampImage handles stamping the user uploaded image with the appropriately
// sized watermark
func StampImage(reader io.Reader, logger *zerolog.Logger) (io.ReadSeeker, error) {

	target, imageType, err := image.Decode(reader)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Failed to decode image")
		return nil, err
	}

	// Pick the longer edge and a reasonably sized stamp
	maxEdge := math.Max(float64(target.Bounds().Max.X), float64(target.Bounds().Max.Y))
	edgeLog := int(math.Floor(math.Log2(maxEdge))) - 1

	logger.Error().
		Str("ImageType", imageType).
		Float64("MaxEdge", maxEdge).
		Int("EdgeLog", edgeLog).
		Interface("TargetBounds", target.Bounds()).
		Msg("Target Dimensions")

	watermarkSuffix := int(math.Max(32, math.Pow(2, math.Min(float64(8), float64(edgeLog)))))
	resourceName := watermarkName(watermarkSuffix)
	logger.Info().
		Str("Resource", resourceName).
		Msg("Watermark resource")

	byteSource, err := assets.FSByte(false, resourceName)
	if err != nil {
		logger.Warn().
			Err(err).
			Str("Name", resourceName).
			Interface("TargetBounds", target.Bounds()).
			Msg("Failed to load computed watermark. Falling to default")
		byteSource = assets.FSMustByte(false, watermarkName(16))
	}
	stampReader := bytes.NewReader(byteSource)
	stamp, _, err := image.Decode(stampReader)
	if err != nil {
		logger.Info().
			Err(err).
			Msg("Failed to load stamp image")
		return nil, err
	}

	// Save it...
	compositedImage := image.NewRGBA(image.Rect(0, 0, target.Bounds().Max.X, target.Bounds().Max.Y))
	draw.Draw(compositedImage, compositedImage.Bounds(), target, image.Point{0, 0}, draw.Src)

	// Bottom right corner
	targetRect := target.Bounds()
	targetRect.Min.X = (targetRect.Max.X - stamp.Bounds().Max.X)
	targetRect.Min.Y = (targetRect.Max.Y - stamp.Bounds().Max.Y)

	logger.Info().
		Interface("TargetBounds", target.Bounds()).
		Interface("StampBounds", stamp.Bounds()).
		Interface("TargetRect", targetRect).
		Msg("Drawing")

	draw.Draw(compositedImage, targetRect, stamp, image.Point{0, 0}, draw.Over)
	buf := new(bytes.Buffer)
	err = png.Encode(buf, compositedImage)
	return bytes.NewReader(buf.Bytes()), nil
}
