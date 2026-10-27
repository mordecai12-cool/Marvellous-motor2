package main

import (
	"fmt"
	"math"
	"time"
)

func (d DeliveryDetails) ArrivalInput() string {
	t, err := time.Parse(time.RFC3339, d.ArrivalAt)
	if err != nil {
		return ""
	}
	return t.In(dealershipTime()).Format("15:04")
}

func (o ManagedOrder) ArrivalLabel() string {
	if o.Status == "delivered" {
		return "Delivery completed"
	}
	if t, err := time.Parse(time.RFC3339, o.Delivery.ArrivalAt); err == nil {
		return t.In(dealershipTime()).Format("02 Jan 2006 · 3:04 PM") + " WAT"
	}
	if o.Delivery.ScheduledDate != "" {
		return o.ScheduledLabel() + " · Time to be confirmed"
	}
	return "Your delivery team will confirm arrival"
}

func (o ManagedOrder) TimeRemaining() string { return o.arrivalRemaining(time.Now()) }

func (o ManagedOrder) arrivalRemaining(now time.Time) string {
	if o.Status == "delivered" {
		return "Your vehicle has arrived"
	}
	arrival, err := time.Parse(time.RFC3339, o.Delivery.ArrivalAt)
	if err != nil {
		day, e := time.ParseInLocation("2006-01-02", o.Delivery.ScheduledDate, dealershipTime())
		if e != nil {
			return "Arrival estimate not set yet"
		}
		today := now.In(dealershipTime())
		start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, dealershipTime())
		days := int(day.Sub(start).Hours() / 24)
		if days < 0 {
			return "Awaiting an updated arrival estimate"
		}
		if days == 0 {
			return "Expected today"
		}
		if days == 1 {
			return "Expected tomorrow"
		}
		return fmt.Sprintf("Expected in %d days", days)
	}
	remaining := arrival.Sub(now)
	if remaining <= 0 {
		return "Awaiting an updated arrival estimate"
	}
	minutes := int(math.Ceil(remaining.Minutes()))
	days, hours, mins := minutes/(24*60), (minutes/60)%24, minutes%60
	quantity := func(value int, unit string) string {
		if value != 1 {
			unit += "s"
		}
		return fmt.Sprintf("%d %s", value, unit)
	}
	if days > 0 {
		return "About " + quantity(days, "day") + ", " + quantity(hours, "hour") + " remaining"
	}
	if hours > 0 {
		return "About " + quantity(hours, "hour") + ", " + quantity(mins, "minute") + " remaining"
	}
	return "About " + quantity(minutes, "minute") + " remaining"
}
