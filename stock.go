package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// Available counts unreserved units. Orders reserve one in the same transaction
// that saves the order; handover never deducts that unit a second time.
func refreshStockStatus(tx *sql.Tx, vehicle string) error {
	_, err := tx.Exec(`UPDATE fleet_vehicles SET status=CASE
 WHEN status='Draft' THEN 'Draft'
 WHEN (SELECT available FROM vehicle_stock WHERE vehicle_id=?)>0 THEN 'Available'
 WHEN EXISTS(SELECT 1 FROM orders o JOIN order_pricing p ON p.order_id=o.id WHERE p.vehicle_id=? AND o.status IN ('payment_pending','processing','out_for_delivery')) THEN 'Reserved'
 ELSE 'Sold' END, updated_at=CURRENT_TIMESTAMP WHERE id=?`, vehicle, vehicle, vehicle)
	return err
}

func updateVehicleStock(db *sql.DB, r *http.Request, staff Staff) error {
	quantity, err := integerField(r, "stock_quantity", 0, 1000000)
	if err != nil {
		return err
	}
	expected, err := integerField(r, "stock_version", 0, 1<<50)
	if err != nil {
		return errors.New("Refresh the stock page before updating its quantity.")
	}
	id := r.FormValue("id")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE vehicle_stock SET available=? WHERE vehicle_id=? AND available=?", quantity, id, expected)
	if err != nil {
		return errors.New("Could not update stock. Please try again.")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("Stock changed since you opened this page. Review the current quantity and try again.")
	}
	if err = refreshStockStatus(tx, id); err != nil {
		return err
	}
	if err = addActivity(tx, staff, "Stock quantity updated", fmt.Sprintf("%s: %d → %d available", id, expected, quantity)); err != nil {
		return err
	}
	return tx.Commit()
}
