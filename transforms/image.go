package transforms

import (
	assets "SpartaImager/assets"
	"bytes"
	"fmt"
	"github.com/Sirupsen/logrus"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
)

func watermarkName(suffix int) string {
	return fmt.Sprintf("/resources/SpartaShield%d.png", suffix)
}

func StampImage(reader io.Reader, logger *logrus.Logger) (io.ReadSeeker, error) {

	target, imageType, err := image.Decode(reader)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"Error": err,
		}).Info("Failed to decode image")
		return nil, err
	}

	// Pick the longer edge and a reasonably sized stamp
	maxEdge := math.Max(float64(target.Bounds().Max.X), float64(target.Bounds().Max.Y))
	edgeLog := int(math.Floor(math.Log2(maxEdge))) - 1
	logger.WithFields(logrus.Fields{
		"ImageType":    imageType,
		"MaxEdge":      maxEdge,
		"EdgeLog":      edgeLog,
		"TargetBounds": target.Bounds(),
	}).Info("Target Dimensions")
	watermarkSuffix := int(math.Max(32, math.Pow(2, math.Min(float64(8), float64(edgeLog)))))
	resourceName := watermarkName(watermarkSuffix)
	logger.Info("Watermark resource: ", resourceName)

	byteSource, err := assets.FSByte(false, resourceName)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"Error":        err,
			"Name":         resourceName,
			"TargetBounds": target.Bounds(),
		}).Warn("Failed to load computed watermark. Falling to default.")
		byteSource = assets.FSMustByte(false, watermarkName(16))
	}
	stampReader := bytes.NewReader(byteSource)
	stamp, _, err := image.Decode(stampReader)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"Error": err,
		}).Info("Failed to load stamp image")
		return nil, err
	}

	// Save it...
	compositedImage := image.NewRGBA(image.Rect(0, 0, target.Bounds().Max.X, target.Bounds().Max.Y))
	draw.Draw(compositedImage, compositedImage.Bounds(), target, image.Point{0, 0}, draw.Src)

	// Bottom right corner
	targetRect := target.Bounds()
	targetRect.Min.X = (targetRect.Max.X - stamp.Bounds().Max.X)
	targetRect.Min.Y = (targetRect.Max.Y - stamp.Bounds().Max.Y)

	logger.WithFields(logrus.Fields{
		"TargetBounds": target.Bounds(),
		"StampBounds":  stamp.Bounds(),
		"TargetRect":   targetRect,
	}).Info("Drawing")

	logger.Debug("Composing image")
	draw.Draw(compositedImage, targetRect, stamp, image.Point{0, 0}, draw.Over)
	buf := new(bytes.Buffer)
	err = png.Encode(buf, compositedImage)
	return bytes.NewReader(buf.Bytes()), nil
}
