package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed vehicle-gallery.json
var galleryJSON []byte

type VehiclePhoto struct {
	Image      string `json:"image"`
	Label      string `json:"label"`
	Caption    string `json:"caption"`
	Source     string `json:"source"`
	Artist     string `json:"artist"`
	License    string `json:"license"`
	LicenseURL string `json:"license_url"`
	PhotoSet   string `json:"photo_set"`
	Reference  bool   `json:"reference,omitempty"`
}

var vehicleGalleries = func() map[string][]VehiclePhoto {
	var galleries map[string][]VehiclePhoto
	if err := json.Unmarshal(galleryJSON, &galleries); err != nil {
		panic(err)
	}
	return galleries
}()

func (v Vehicle) Photos() []VehiclePhoto {
	if len(v.PhotoItems) > 0 {
		return v.PhotoItems
	}
	source := vehicleGalleries[v.ID]
	if len(source) == 1 && source[0].Label == "Exterior" {
		return source
	}
	primarySet := ""
	for _, photo := range source {
		if photo.Label == "Front view" && photo.PhotoSet != "" {
			primarySet = photo.PhotoSet
			break
		}
	}
	photos := []VehiclePhoto{}
	exterior := false
	for _, photo := range source {
		if photo.Reference {
			photos = append(photos, photo)
			continue
		}
		// An unverified rear or side image must never be displayed as this car's view.
		if primarySet != "" && photo.PhotoSet == primarySet {
			photos = append(photos, photo)
			exterior = true
		}
	}
	if !exterior {
		return append([]VehiclePhoto{{Image: v.Image, Label: "Exterior", Caption: "Representative model photograph", Source: "/image-credits.html"}}, photos...)
	}
	return photos
}
