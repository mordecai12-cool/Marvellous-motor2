package main

import (
	"os"
	"strings"
)

const deploymentSecondOwnerEmail = "marvellousmordecaiotiti@gmail.com"

func ownerOnlyAccess() bool { return os.Getenv("MM_OWNER_ONLY") != "false" }
func allowedOwnerEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return email == deploymentOwnerEmail || email == deploymentSecondOwnerEmail
}
func authorizedStaff(s Staff) bool {
	if s.Status != "approved" {
		return false
	}
	if ownerOnlyAccess() {
		return s.Role == "admin" || (s.Role == "super_admin" && allowedOwnerEmail(s.Email))
	}
	return s.Role == "admin" || s.Role == "super_admin"
}
