package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVehicleGalleryFilesAndAttributions(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range vehicles {
		photos := v.Photos()
		if len(photos) == 0 {
			t.Fatal(v.ID, "has no photographs")
		}
		for _, p := range photos {
			if !strings.HasPrefix(p.Image, "/images/") || strings.Contains(p.Image, "..") {
				t.Fatal("Invalid image path", p.Image)
			}
			info, err := os.Stat(strings.TrimPrefix(p.Image, "/"))
			if err != nil || info.Size() == 0 {
				t.Fatal("Missing gallery asset", p.Image, err)
			}
			if p.Source == "" || p.Label == "" {
				t.Fatal("Missing attribution or view label", v.ID)
			}
			if strings.HasPrefix(p.Image, "/images/gallery/") && (p.License == "" || p.Artist == "" || !strings.HasPrefix(p.Source, "https://commons.wikimedia.org/")) {
				t.Fatal("Gallery photo needs its license and source", v.ID)
			}
			if seen[v.ID+":"+p.Image] {
				t.Fatal("Duplicate gallery photo", v.ID)
			}
			seen[v.ID+":"+p.Image] = true
		}
	}
}

// Independently compare the source filenames, so a mistaken photo_set tag alone
// cannot make a different vehicle's rear photograph pass this check.
func TestGalleryExteriorSourceFamiliesMatch(t *testing.T) {
	extension := regexp.MustCompile(`(?i)\.(jpg|jpeg|png|webp)$`)
	number := regexp.MustCompile(`\(\d+\)`)
	direction := regexp.MustCompile(`(?i)\b(front|rear|left|right|side|view)\b`)
	punctuation := regexp.MustCompile(`[^a-z0-9]`)
	family := func(title string) string {
		title = extension.ReplaceAllString(title, "")
		title = number.ReplaceAllString(title, "")
		title = direction.ReplaceAllString(title, "")
		return punctuation.ReplaceAllString(strings.ToLower(title), "")
	}
	for _, v := range vehicles {
		photos := v.Photos()
		if photos[0].Image != v.Image {
			t.Fatalf("%s listing and gallery show different front photos", v.ID)
		}
		set := ""
		for _, p := range photos {
			if p.Label == "Front view" {
				set = family(p.Caption)
			}
		}
		for _, p := range vehicleGalleries[v.ID] {
			if p.Reference || p.Label == "Exterior" {
				continue
			}
			if set == "" || family(p.Caption) != set || p.PhotoSet != set {
				t.Fatalf("%s mixes different exterior photo sets: %s", v.ID, p.Caption)
			}
		}
	}
}
func TestVehicleGalleryHidesUnverifiedRearPhotos(t *testing.T) {
	v := vehicles[0]
	old := vehicleGalleries[v.ID]
	defer func() { vehicleGalleries[v.ID] = old }()
	vehicleGalleries[v.ID] = []VehiclePhoto{{Image: v.Image, Label: "Front view", PhotoSet: "verified-car"}, {Image: "/images/wrong-car.jpg", Label: "Rear view", PhotoSet: "other-car"}}
	photos := v.Photos()
	if len(photos) != 1 || photos[0].Label != "Front view" {
		t.Fatal("A different car's rear photo was displayed")
	}
}
