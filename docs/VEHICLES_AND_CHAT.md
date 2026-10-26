# Vehicle collections and complaint attachments

The homepage links to Sports and Luxury collections. The catalogue combines collection and engine-configuration filters with existing make, price, condition and other filters. Both filters are preserved in shareable URLs and saved searches. Vehicle cards show the engine and collection; the detail page and comparison include engine configuration and collection.

The sample inventory distinguishes V6, inline-six, inline-four, flat-six boxer and electric motors. The G 500 sample retains an unconfirmed engine configuration because its exact market/production specification is not supplied. Vehicle equipment remains a model guide that should be confirmed for the actual stock vehicle.

Model references used for the key engine distinctions:

- [2021 Lexus RX brochure: 3.5-liter V6](https://www.lexus.com/content/dam/lexus/documents/brochures/models/2021/MY21-Lexus-RX-Brochure.pdf).
- [2024 Lexus RX: turbocharged four-cylinder](https://pressroom.lexus.com/whats-new-2024-lexus-rx/).
- [2024 BMW X5 xDrive40i: 3.0-liter inline-six](https://www.press.bmwgroup.com/usa/article/detail/T0408460EN_US/the-2024-bmw-x5-and-x6?language=en_US).
- [Porsche 911 Carrera: 3.0-liter six-cylinder boxer](https://pnr-prd2-pub1.newsroom.porsche.com/en/press-kits/911/Drive-technology--Porsche-911-Carrera.html).
- [2025 Toyota Camry specifications: four-cylinder hybrid](https://www.toyota.com/content/dam/toyota/brochures/pdf/2025/my25-camry-ebrochure.pdf).

## Sharing complaint evidence

1. Sign in using the regular login, open customer care and choose **Attach a picture or file**.
2. Choose one JPEG, PNG, PDF or TXT file up to 5 MB. A caption is optional. Press **Send**.
3. The customer and approved admins can view images directly in the conversation. Clicking an image opens it at full size; documents download with their original names.
4. Admins open **Customer chat** to review the complaint and can attach a picture or document in their reply.

Messages and attachment metadata are stored together transactionally in SQLite. File bytes are stored privately in `support/` below `MM_UPLOADS_DIR` (or the deployment's configured persistent uploads directory), using random storage names. The private attachment endpoint checks the current staff role/status or customer ownership on every request. Retries with the same send reference do not duplicate files or messages, and failed transactions remove any newly written file. Existing text-only chat history is preserved.

Production deployments must persist both the database and configured uploads directory, just as they do for payment receipts. Back up both together.

## Verification

Run `go test ./...` and `go vet ./...`. With the local server and Python Playwright available, run `.venv/bin/python tests/vehicle_chat_browser_check.py` for collection filtering, engine details, mobile layouts, customer image uploads, admin previews, document replies and attachment history.

The implementation passed the backend test suite, `go vet`, 22 browser checks and an additional mobile check for both embedded and floating chat. Backend coverage includes file ownership, disabled staff, invalid formats, upload limits, CSRF, retry deduplication and removing files when a database transaction fails. Local fictional complaint conversations demonstrate the image and document exchange.
