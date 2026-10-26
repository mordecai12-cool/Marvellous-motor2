# Vehicle photography cleanup

The catalogue keeps all 50 illustrative vehicle listings. The Mazda MX-5 driver and people in other selected sample photographs were removed. Crowded secondary gallery photographs were removed entirely; three cover photographs were replaced with licensed, visually checked alternatives (Mazda RX-8, Mercedes-Benz 230 E W123 and Nissan 370Z NISMO).

Edited JPEG assets are saved under `images/` and `images/gallery/` using their existing filenames, so persisted inventory links remain valid locally and after deployment. `photo-retouches.json` lists every edited asset and its SHA-256. `vehicle-gallery.json` marks edited photographs, and the detail viewer discloses the retouch alongside the original photographer and licence. Replacement photo attribution is in that file and `image-credits.json` / `image-credits.html`.

## Tool and prompts

Mode: built-in imagegen, one supplied photograph per edit. Outputs were copied into this repository and encoded as web JPEGs. No CLI image generation was used. Original source photographs and their applicable licences remain attributed; these are representative model images, not evidence of actual dealership stock. Actual dealer-uploaded stock photographs were not retouched by this change.

Prompt set, with the supplied file as the edit target:

> Use case: precise-object-edit. Edit target: the supplied vehicle photograph for a professional car catalogue. Remove EVERY person, including tiny background pedestrians, occupants inside all cars, visible heads, and human reflections. Reconstruct the existing background naturally where people were. Preserve the featured vehicle exactly: same model, paint colour, body shape, trim, wheels, glass, lights, plates, camera angle, framing and current condition. Keep the original surroundings and other parked cars, but no people anywhere. Photorealistic clean result; no added text, watermark or invented accessories.

For the Supra, a second targeted pass requested an empty cabin with only factory seats, headrests, dashboard and steering wheel, and removal of human depictions from background posters.

Earlier photographs used the equivalent targeted instruction to remove background people, visible occupants and reflections while preserving the vehicle and setting. The generated files were visually inspected before installation. Downloaded replacement photographs were inspected independently; alternatives containing crowds were discarded.

## Presentation and validation

Vehicle cards retain the established sky-blue/navy palette with consistent spacing, wrapping specifications, aligned actions and contained images that show the full car. Gallery frames use neutral surfaces, full-size viewing, keyboard controls and clear source/retouch labels. Actual stock photographs keep their stock-specific captions.

Run `go test ./...`, `go vet ./...`, `.venv/bin/python tests/catalogue_recovery_browser_check.py` and, with the rebuilt preview running, `.venv/bin/python tests/photo_browser_check.py`. The latter visits all 50 vehicle detail pages, checks every displayed gallery image, exercises the viewer and verifies retouch disclosures and mobile width. Automated checks verify asset delivery and interfaces; the absence of people is checked visually.

Filter summaries now use readable, individually removable chips and accessible button names. Search and select fields use neutral surfaces while keeping the requested sky-blue page background.

Validation completed: full Go tests and vet; 40 catalogue/recovery browser checks; 35 business workflow browser checks; all 50 detail pages and 67 gallery photographs; all public navigation destinations checked without errors; JavaScript parsing and targeted filter, source-label, keyboard and mobile checks. All 22 retouched assets have verified manifest hashes.
