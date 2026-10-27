package main

import (
	"fmt"
	"strings"
)

const privateStaffLabel = "Marvellous Motors team"

// Profile visibility is independent of authentication and owner permissions.
func privateStaff(s Staff) bool {
	return strings.EqualFold(strings.TrimSpace(s.Email), deploymentOwnerEmail)
}

func staffDisplayName(s Staff) string {
	if privateStaff(s) {
		return privateStaffLabel
	}
	return s.Name
}

// Only server-owned SQL identifiers may be passed here. Joining by account ID
// also protects historical records written before profile privacy was enabled.
func staffDisplayNameSQL(alias, fallback string) string {
	return fmt.Sprintf("CASE WHEN lower(%s.email)='%s' THEN '%s' ELSE %s END", alias, deploymentOwnerEmail, privateStaffLabel, fallback)
}
