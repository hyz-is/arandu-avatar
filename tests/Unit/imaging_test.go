package unit_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyz-is/arandu-avatar/internal/imaging"
)

var sizes = []int{16, 32, 64}

var limits = imaging.Limits{MaxPixels: 4_000_000, JPEGQuality: 85}

// photo is a picture that is red on its left half and blue on its right, with a
// green square in the corner, which tells a crop and an orientation apart.
func photo(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch {
			case x < w/2:
				img.Set(x, y, color.RGBA{R: 220, A: 255})
			default:
				img.Set(x, y, color.RGBA{B: 220, A: 255})
			}
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withEXIF inserts an APP1 Exif segment after the SOI of a JPEG, holding the
// orientation, a GPS tag and a recognisable string.
func withEXIF(t *testing.T, jpegBytes []byte, orientation int) []byte {
	t.Helper()
	var tiff bytes.Buffer
	tiff.WriteString("MM")
	binary.Write(&tiff, binary.BigEndian, uint16(42))
	binary.Write(&tiff, binary.BigEndian, uint32(8))
	binary.Write(&tiff, binary.BigEndian, uint16(2)) // two entries
	// Orientation, SHORT, 1 value
	binary.Write(&tiff, binary.BigEndian, uint16(0x0112))
	binary.Write(&tiff, binary.BigEndian, uint16(3))
	binary.Write(&tiff, binary.BigEndian, uint32(1))
	binary.Write(&tiff, binary.BigEndian, uint16(orientation))
	binary.Write(&tiff, binary.BigEndian, uint16(0))
	// GPS IFD pointer
	binary.Write(&tiff, binary.BigEndian, uint16(0x8825))
	binary.Write(&tiff, binary.BigEndian, uint16(4))
	binary.Write(&tiff, binary.BigEndian, uint32(1))
	binary.Write(&tiff, binary.BigEndian, uint32(60))
	binary.Write(&tiff, binary.BigEndian, uint32(0))
	tiff.WriteString("GPSLatitude=-23.5505,GPSLongitude=-46.6333;SECRET-CAMERA-SERIAL")

	segment := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	length := uint16(len(segment) + 2)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(length >> 8), byte(length)}
	out = append(out, segment...)
	return append(out, jpegBytes[2:]...)
}

func decode(t *testing.T, body []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("the output does not decode: %v", err)
	}
	return img
}

func TestAJPEGBecomesOneSquareVariantPerSize(t *testing.T) {
	t.Parallel()

	res, err := imaging.Process(encodeJPEG(t, photo(200, 120)), imaging.Crop{}, sizes, limits)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != "image/jpeg" || res.Extension != ".jpg" || len(res.Variants) != len(sizes) {
		t.Fatalf("result = %s %s with %d variants", res.Type, res.Extension, len(res.Variants))
	}
	for i, v := range res.Variants {
		if v.Size != sizes[i] {
			t.Errorf("variant %d has size %d, want %d", i, v.Size, sizes[i])
		}
		if b := decode(t, v.Body).Bounds(); b.Dx() != v.Size || b.Dy() != v.Size {
			t.Errorf("variant %d is %dx%d, want a %d square", i, b.Dx(), b.Dy(), v.Size)
		}
	}
}

func TestEXIFAndGPSAreGoneFromEveryVariant(t *testing.T) {
	t.Parallel()

	source := withEXIF(t, encodeJPEG(t, photo(160, 160)), 1)
	if !bytes.Contains(source, []byte("GPSLatitude")) {
		t.Fatal("the fixture carries no GPS")
	}
	res, err := imaging.Process(source, imaging.Crop{}, sizes, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Variants {
		for _, marker := range []string{"Exif", "GPSLatitude", "GPSLongitude", "SECRET-CAMERA-SERIAL", "\xFF\xE1"} {
			if bytes.Contains(v.Body, []byte(marker)) {
				t.Errorf("variant %d still carries %q", v.Size, marker)
			}
		}
	}
}

func TestTheEXIFOrientationTurnsThePictureUpright(t *testing.T) {
	t.Parallel()

	// Stored red-left blue-right. Orientation 6 says the camera was turned: the
	// upright picture is the stored one rotated 90 degrees clockwise, so what
	// was the left (red) is at the top.
	res, err := imaging.Process(withEXIF(t, encodeJPEG(t, photo(160, 160)), 6), imaging.Crop{}, []int{32}, limits)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, res.Variants[0].Body)
	top, bottom := img.At(16, 4), img.At(16, 28)
	tr, _, tb, _ := top.RGBA()
	br, _, bb, _ := bottom.RGBA()
	if !(tr > tb && bb > br) {
		t.Errorf("orientation 6 did not put red on top and blue below: top=%v bottom=%v", top, bottom)
	}
}

func TestACropChoosesTheSquareAndIsClampedIntoThePicture(t *testing.T) {
	t.Parallel()

	src := encodeJPEG(t, photo(200, 100))
	// The left half of the picture is red, and a square 50 wide at x=0 is all red.
	res, err := imaging.Process(src, imaging.Crop{X: 0, Y: 0, Size: 0.5}, []int{32}, limits)
	if err != nil {
		t.Fatal(err)
	}
	r, _, b, _ := decode(t, res.Variants[0].Body).At(16, 16).RGBA()
	if r < b {
		t.Error("a crop at the left edge is not red")
	}
	// Nonsense is clamped instead of failing.
	for _, crop := range []imaging.Crop{{X: -4, Y: 9, Size: 7}, {X: 2, Y: 2, Size: 0.0001}, {X: 0.5, Y: 0.5, Size: -3}} {
		if _, err := imaging.Process(src, crop, []int{32}, limits); err != nil && !strings.Contains(err.Error(), "small") {
			t.Errorf("crop %+v failed: %v", crop, err)
		}
	}
}

func TestAnOpaquePNGBecomesAJPEGAndATransparentOneStaysPNG(t *testing.T) {
	t.Parallel()

	res, err := imaging.Process(encodePNG(t, photo(120, 120)), imaging.Crop{}, sizes, limits)
	if err != nil || res.Type != "image/jpeg" {
		t.Fatalf("opaque PNG: %v %s", err, res.Type)
	}
	clear := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	for y := 30; y < 90; y++ {
		for x := 30; x < 90; x++ {
			clear.Set(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	res, err = imaging.Process(encodePNG(t, clear), imaging.Crop{}, sizes, limits)
	if err != nil || res.Type != "image/png" || res.Extension != ".png" {
		t.Fatalf("transparent PNG: %v %s", err, res.Type)
	}
	img := decode(t, res.Variants[2].Body)
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Errorf("the transparent corner became opaque (alpha %d)", a)
	}
}

func TestWebPIsReadByItsBytes(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"yellow_rose.lossy.webp", "gopher-doc.with-alpha.lossless.webp"} {
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if kind, ok := imaging.Detect(body); !ok || kind != imaging.WebP {
			t.Fatalf("%s is not detected as WebP", name)
		}
		res, err := imaging.Process(body, imaging.Crop{}, sizes, limits)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(res.Variants) != len(sizes) {
			t.Errorf("%s: %d variants", name, len(res.Variants))
		}
		if strings.Contains(string(res.Variants[0].Body), "RIFF") {
			t.Errorf("%s: the output is still a WebP container", name)
		}
	}
}

func TestBytesThatAreNotAPictureAreRefusedWhateverTheyAreCalled(t *testing.T) {
	t.Parallel()

	var animated bytes.Buffer
	if err := gif.Encode(&animated, photo(40, 40), nil); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"text":        []byte("this is definitely a photo.jpg, trust me"),
		"html":        []byte("<html><script>alert(1)</script></html>"),
		"svg":         []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"gif":         animated.Bytes(),
		"empty":       nil,
		"two bytes":   {0xFF, 0xD8},
		"php in jpeg": append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("<?php system($_GET['c']); ?>")...),
	} {
		_, err := imaging.Process(body, imaging.Crop{}, sizes, limits)
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := imaging.Process([]byte("not a picture"), imaging.Crop{}, sizes, limits); err != imaging.ErrType {
		t.Errorf("fake bytes: err = %v, want ErrType", err)
	}
	if _, err := imaging.Process(append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, 1, 2, 3), imaging.Crop{}, sizes, limits); err == nil || err == imaging.ErrType {
		t.Errorf("a JPEG that cannot be read: err = %v, want ErrCorrupt", err)
	}
}

// bombPNG is a few dozen bytes that declare a picture of w by h pixels.
func bombPNG(w, h uint32) []byte {
	chunk := func(kind string, payload []byte) []byte {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(payload)))
		b.WriteString(kind)
		b.Write(payload)
		binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), payload...)))
		return b.Bytes()
	}
	ihdr := new(bytes.Buffer)
	binary.Write(ihdr, binary.BigEndian, w)
	binary.Write(ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 2, 0, 0, 0})
	out := []byte("\x89PNG\r\n\x1a\n")
	out = append(out, chunk("IHDR", ihdr.Bytes())...)
	return append(out, chunk("IEND", nil)...)
}

func TestADeclaredSizeIsHeldAgainstTheCeilingBeforeAnyPixelIsDecoded(t *testing.T) {
	t.Parallel()

	body := bombPNG(50000, 50000)
	if len(body) > 100 {
		t.Fatalf("the fixture is %d bytes", len(body))
	}
	if _, err := imaging.Process(body, imaging.Crop{}, sizes, limits); err != imaging.ErrTooLarge {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestAPictureSmallerThanTheSmallestSizeIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := imaging.Process(encodeJPEG(t, photo(10, 10)), imaging.Crop{}, sizes, limits); err != imaging.ErrTooSmall {
		t.Errorf("err = %v, want ErrTooSmall", err)
	}
}
