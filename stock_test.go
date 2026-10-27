package main

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func TestStockStartsAtSixAndAdminChangesPersist(t *testing.T) {
	db := serviceFixture(t)
	list, err := fleetList(db, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list {
		if v.StockQuantity != 6 || !v.StockManaged {
			t.Fatal("Initial stock was not six", v.ID, v.StockQuantity)
		}
	}
	fields := url.Values{"action": {"stock"}, "id": {"toyota-camry"}, "stock_quantity": {"10"}, "stock_version": {"6"}}
	w := httptest.NewRecorder()
	manageInventory(db, w, adminPost(t, "/admin/inventory", fields, 0))
	if w.Code != 403 {
		t.Fatal("Anonymous stock update allowed", w.Code)
	}
	w = httptest.NewRecorder()
	manageInventory(db, w, servicePost(t, "/admin/inventory", fields, 1))
	if w.Code != 403 {
		t.Fatal("Customer stock update allowed", w.Code)
	}
	w = httptest.NewRecorder()
	manageInventory(db, w, adminPost(t, "/admin/inventory", fields, 2))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := initBusiness(db); err != nil {
		t.Fatal(err)
	}
	v, _ := fleetVehicle(db, "toyota-camry", false)
	if v.StockQuantity != 10 {
		t.Fatal("Restart reset admin quantity", v.StockQuantity)
	}
	if err := updateVehicleStock(db, adminPost(t, "/admin/inventory", fields, 2), Staff{ID: 2, Name: "Test Staff"}); err == nil {
		t.Fatal("Stale stock form was accepted")
	}
	fields.Set("stock_version", "10")
	fields.Set("stock_quantity", "-1")
	if err := updateVehicleStock(db, adminPost(t, "/admin/inventory", fields, 2), Staff{ID: 2}); err == nil {
		t.Fatal("Negative stock accepted")
	}
	fields.Set("stock_quantity", "1.5")
	if err := updateVehicleStock(db, adminPost(t, "/admin/inventory", fields, 2), Staff{ID: 2}); err == nil {
		t.Fatal("Fractional stock accepted")
	}
}

func TestSixOrdersReserveSixUnitsAndCancellationReturnsOne(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"vehicle-id": {"toyota-camry"}, "country": {"Nigeria"}, "state": {"Benue"}, "city": {"Otukpo"}, "address": {"1 Fictional Road"}}
	for i := 0; i < 6; i++ {
		w := httptest.NewRecorder()
		placeVehicleOrder(db, w, servicePost(t, "/order", fields, 1))
		if w.Code != 303 {
			t.Fatal("Could not order unit", i+1, w.Code, w.Body.String())
		}
		v, _ := fleetVehicle(db, "toyota-camry", false)
		if v.StockQuantity != 5-i {
			t.Fatal("Wrong stock after order", v.StockQuantity)
		}
		if i < 5 && !v.Orderable() {
			t.Fatal("One order hid the rest of the stock")
		}
	}
	v, _ := fleetVehicle(db, "toyota-camry", false)
	if v.Orderable() || v.StockStatus != "Reserved" {
		t.Fatal("Exhausted stock still orderable", v.StockStatus)
	}
	w := httptest.NewRecorder()
	placeVehicleOrder(db, w, servicePost(t, "/order", fields, 2))
	if w.Code != 422 && w.Code != 409 {
		t.Fatal("Seventh order oversold stock", w.Code)
	}
	var id int64
	db.QueryRow("SELECT MAX(order_id) FROM order_pricing WHERE vehicle_id='toyota-camry'").Scan(&id)
	cancel := url.Values{"order-id": {fmt.Sprint(id)}}
	w = httptest.NewRecorder()
	cancelVehicleOrder(db, w, adminPost(t, "/admin/orders/cancel", cancel, 2))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	v, _ = fleetVehicle(db, "toyota-camry", false)
	if v.StockQuantity != 1 || !v.Orderable() {
		t.Fatal("Cancellation did not return one unit", v.StockQuantity, v.StockStatus)
	}
	w = httptest.NewRecorder()
	cancelVehicleOrder(db, w, adminPost(t, "/admin/orders/cancel", cancel, 2))
	if w.Code != 409 {
		t.Fatal("Repeated cancellation was accepted", w.Code)
	}
	v, _ = fleetVehicle(db, "toyota-camry", false)
	if v.StockQuantity != 1 {
		t.Fatal("Repeated cancellation added stock")
	}
}

func TestConcurrentBuyersCannotReserveTheSameLastUnit(t *testing.T) {
	db := serviceFixture(t)
	db.Exec("UPDATE vehicle_stock SET available=1 WHERE vehicle_id='toyota-camry'")
	fields := url.Values{"vehicle-id": {"toyota-camry"}, "country": {"Nigeria"}, "state": {"Benue"}, "city": {"Otukpo"}, "address": {"1 Fictional Road"}}
	requests := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	// Construct cookies before concurrency; then release both buyer requests together.
	one, two := servicePost(t, "/order", fields, 1), servicePost(t, "/order", fields, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; placeVehicleOrder(db, requests[0], one) }()
	go func() { defer wg.Done(); <-start; placeVehicleOrder(db, requests[1], two) }()
	close(start)
	wg.Wait()
	successes := 0
	for _, w := range requests {
		if w.Code == 303 {
			successes++
		} else if w.Code != 409 && w.Code != 422 {
			t.Fatal("Unexpected failure", w.Code, w.Body.String())
		}
	}
	if successes != 1 {
		t.Fatal("Expected one reservation", successes)
	}
	var stock, orders int
	db.QueryRow("SELECT available FROM vehicle_stock WHERE vehicle_id='toyota-camry'").Scan(&stock)
	db.QueryRow("SELECT COUNT(*) FROM order_pricing WHERE vehicle_id='toyota-camry'").Scan(&orders)
	if stock != 0 || orders != 1 {
		t.Fatal("Oversold last unit", stock, orders)
	}
}

func TestHandoverDoesNotDeductOrHideOtherUnits(t *testing.T) {
	db := serviceFixture(t)
	fields := url.Values{"vehicle-id": {"toyota-camry"}, "country": {"Nigeria"}, "state": {"Benue"}, "city": {"Otukpo"}, "address": {"1 Fictional Road"}}
	w := httptest.NewRecorder()
	placeVehicleOrder(db, w, servicePost(t, "/order", fields, 1))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	var id int64
	db.QueryRow("SELECT MAX(order_id) FROM order_pricing").Scan(&id)
	db.Exec("UPDATE orders SET status='out_for_delivery' WHERE id=?", id)
	w = httptest.NewRecorder()
	transitionDelivery(db, w, adminPost(t, "/admin/orders/delivered", url.Values{"order-id": {fmt.Sprint(id)}}, 2), "delivered")
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	v, _ := fleetVehicle(db, "toyota-camry", false)
	if v.StockQuantity != 5 || !v.Orderable() {
		t.Fatal("Handover deducted stock twice or hid remaining vehicles", v.StockQuantity, v.StockStatus)
	}
}
